package sync

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jseidel/mac-profile-sync/internal/config"
	"github.com/jseidel/mac-profile-sync/internal/network"
	"github.com/jseidel/mac-profile-sync/pkg/fileutil"
	"github.com/rs/zerolog/log"
)

// SyncActivity represents a sync operation
type SyncActivity struct {
	Type       string    `json:"type"` // "sent", "received", "deleted"
	FileName   string    `json:"file_name"`
	FolderPath string    `json:"folder_path"`
	RelPath    string    `json:"rel_path"`
	PeerName   string    `json:"peer_name"`
	Timestamp  time.Time `json:"timestamp"`
}

// getFolderName extracts the base folder name from a path (e.g., "Desktop" from "/Users/josh/Desktop")
func getFolderName(folderPath string) string {
	return filepath.Base(folderPath)
}

// findLocalFolderByName finds the local folder path that matches the given folder name
func (e *Engine) findLocalFolderByName(folderName string) string {
	for _, folder := range e.cfg.Folders {
		if folder.Enabled && filepath.Base(folder.Path) == folderName {
			return folder.Path
		}
	}
	return ""
}

// Engine orchestrates the sync process
type Engine struct {
	cfg      *config.Config
	watcher  *Watcher
	state    *StateStore
	conflict *ConflictDetector
	server   *network.Server
	client   *network.Client

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Callbacks
	onActivity func(*SyncActivity)
	onConflict func(*Conflict)
	onError    func(error)

	// Activity log
	activities   []*SyncActivity
	activityMu   sync.RWMutex
	maxActivities int
}

// NewEngine creates a new sync engine
func NewEngine(cfg *config.Config, server *network.Server, client *network.Client) (*Engine, error) {
	watcher, err := NewWatcher(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	state := NewStateStore()
	conflict := NewConflictDetector(cfg, state)

	ctx, cancel := context.WithCancel(context.Background())

	return &Engine{
		cfg:           cfg,
		watcher:       watcher,
		state:         state,
		conflict:      conflict,
		server:        server,
		client:        client,
		ctx:           ctx,
		cancel:        cancel,
		activities:    make([]*SyncActivity, 0),
		maxActivities: 100,
	}, nil
}

// SetCallbacks sets the event callbacks
func (e *Engine) SetCallbacks(onActivity func(*SyncActivity), onConflict func(*Conflict), onError func(error)) {
	e.onActivity = onActivity
	e.onConflict = onConflict
	e.onError = onError
	e.conflict.SetCallback(onConflict)
}

// Start starts the sync engine
func (e *Engine) Start() error {
	// Load saved state
	if err := e.state.Load(); err != nil {
		log.Warn().Err(err).Msg("Failed to load state, starting fresh")
	}

	// Initialize folder states
	for _, folder := range e.cfg.Folders {
		if folder.Enabled {
			e.state.InitFolder(folder.Path)
		}
	}

	// Set up network message handlers
	e.server.SetHandlers(e.onClientConnect, e.onClientDisconnect, e.onServerMessage)
	e.client.SetHandlers(e.onServerConnect, e.onServerDisconnect, e.onClientMessage)

	// Start file watcher
	if err := e.watcher.Start(); err != nil {
		return fmt.Errorf("failed to start watcher: %w", err)
	}

	// Start processing events
	e.wg.Add(1)
	go e.processFileEvents()

	log.Info().Msg("Sync engine started")
	return nil
}

// Stop stops the sync engine
func (e *Engine) Stop() {
	e.cancel()
	e.watcher.Stop()
	e.wg.Wait()

	// Save state
	if err := e.state.Save(); err != nil {
		log.Error().Err(err).Msg("Failed to save state")
	}

	log.Info().Msg("Sync engine stopped")
}

// SyncFolder performs a full sync of a folder with all connected peers
func (e *Engine) SyncFolder(folderPath string) error {
	log.Info().Str("folder", folderPath).Msg("Starting folder sync")

	// Scan folder and build file list
	files, err := e.scanFolder(folderPath)
	if err != nil {
		return fmt.Errorf("failed to scan folder: %w", err)
	}

	// Convert to network format
	netFiles := make([]network.FileInfo, len(files))
	for i, f := range files {
		netFiles[i] = network.FileInfo{
			RelPath:    f.RelPath,
			Size:       f.Size,
			ModTime:    f.ModTime,
			Hash:       f.Hash,
			IsDir:      f.IsDir,
			Permission: uint32(f.Permission),
			FolderPath: folderPath,
		}
	}

	// Send file list to all connected peers
	msg := network.FileListMessage{
		FolderPath: folderPath,
		FolderName: getFolderName(folderPath),
		Files:      netFiles,
	}

	if err := e.server.BroadcastPayload(network.MsgFileList, msg); err != nil {
		return fmt.Errorf("failed to broadcast file list: %w", err)
	}

	// Also send to outgoing connections
	for _, conn := range e.client.GetConnections() {
		if err := conn.SendPayload(network.MsgFileList, msg); err != nil {
			log.Error().Err(err).Str("peer", conn.Address).Msg("Failed to send file list")
		}
	}

	return nil
}

func (e *Engine) scanFolder(folderPath string) ([]*fileutil.FileInfo, error) {
	var files []*fileutil.FileInfo

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip ignored files
		if e.cfg.ShouldIgnore(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip the root folder itself
		if path == folderPath {
			return nil
		}

		fi, err := fileutil.GetFileInfo(path, folderPath)
		if err != nil {
			log.Warn().Err(err).Str("path", path).Msg("Failed to get file info")
			return nil
		}

		files = append(files, fi)
		return nil
	})

	return files, err
}

func (e *Engine) processFileEvents() {
	defer e.wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return

		case event := <-e.watcher.Events():
			e.handleFileEvent(event)
		}
	}
}

func (e *Engine) handleFileEvent(event FileEvent) {
	log.Debug().
		Str("type", event.Type.String()).
		Str("path", event.Path).
		Str("folder", event.FolderPath).
		Msg("File event")

	switch event.Type {
	case EventCreate, EventModify:
		e.handleFileChange(event)
	case EventDelete:
		e.handleFileDelete(event)
	case EventRename:
		// Treat rename as delete + create
		e.handleFileDelete(event)
	}
}

func (e *Engine) handleFileChange(event FileEvent) {
	// Check if we're allowed to send files
	if !e.cfg.CanSend() {
		log.Debug().Str("path", event.Path).Msg("Skipping send (receive_only mode)")
		return
	}

	// Get file info
	fi, err := fileutil.GetFileInfo(event.Path, event.FolderPath)
	if err != nil {
		log.Error().Err(err).Str("path", event.Path).Msg("Failed to get file info")
		return
	}

	// Update state
	e.state.UpdateFileState(event.FolderPath, &FileState{
		RelPath:    fi.RelPath,
		Hash:       fi.Hash,
		Size:       fi.Size,
		ModTime:    fi.ModTime,
		Permission: fi.Permission,
		SyncedAt:   time.Now(),
		SyncedFrom: e.cfg.Device.Name,
	})

	// Prepare file data message
	data, err := os.ReadFile(event.Path)
	if err != nil {
		log.Error().Err(err).Str("path", event.Path).Msg("Failed to read file")
		return
	}

	msg := network.FileDataMessage{
		FolderPath: event.FolderPath,
		FolderName: getFolderName(event.FolderPath),
		RelPath:    fi.RelPath,
		Size:       fi.Size,
		ModTime:    fi.ModTime,
		Permission: uint32(fi.Permission),
		Hash:       fi.Hash,
		Data:       data,
	}

	// Send to all peers
	if err := e.server.BroadcastPayload(network.MsgFileData, msg); err != nil {
		log.Error().Err(err).Msg("Failed to broadcast file")
	}

	for _, conn := range e.client.GetConnections() {
		if err := conn.SendPayload(network.MsgFileData, msg); err != nil {
			log.Error().Err(err).Str("peer", conn.Address).Msg("Failed to send file")
		}
	}

	// Record activity
	e.addActivity(&SyncActivity{
		Type:       "sent",
		FileName:   filepath.Base(event.Path),
		FolderPath: event.FolderPath,
		RelPath:    fi.RelPath,
		PeerName:   "all",
		Timestamp:  time.Now(),
	})
}

func (e *Engine) handleFileDelete(event FileEvent) {
	// Update state
	e.state.RemoveFileState(event.FolderPath, event.RelPath)

	// Check if we're allowed to send
	if !e.cfg.CanSend() {
		log.Debug().Str("path", event.Path).Msg("Skipping delete broadcast (receive_only mode)")
		return
	}

	// Notify peers
	msg := network.FileDeleteMessage{
		FolderPath: event.FolderPath,
		FolderName: getFolderName(event.FolderPath),
		RelPath:    event.RelPath,
	}

	if err := e.server.BroadcastPayload(network.MsgFileDelete, msg); err != nil {
		log.Error().Err(err).Msg("Failed to broadcast delete")
	}

	for _, conn := range e.client.GetConnections() {
		if err := conn.SendPayload(network.MsgFileDelete, msg); err != nil {
			log.Error().Err(err).Str("peer", conn.Address).Msg("Failed to send delete")
		}
	}

	// Record activity
	e.addActivity(&SyncActivity{
		Type:       "deleted",
		FileName:   filepath.Base(event.Path),
		FolderPath: event.FolderPath,
		RelPath:    event.RelPath,
		PeerName:   "all",
		Timestamp:  time.Now(),
	})
}

// Network handlers
func (e *Engine) onClientConnect(conn *network.Connection) {
	log.Info().Str("remote", conn.ID).Msg("Peer connected (incoming)")

	// Send hello
	hello := network.HelloMessage{
		DeviceName: e.cfg.Device.Name,
		DeviceID:   e.cfg.Device.Name, // Use name as ID for now
		Version:    network.ProtocolVersion,
	}
	_ = conn.SendPayload(network.MsgHello, hello)
}

func (e *Engine) onClientDisconnect(conn *network.Connection) {
	log.Info().Str("remote", conn.ID).Msg("Peer disconnected (incoming)")
}

func (e *Engine) onServerConnect(conn *network.ClientConnection) {
	log.Info().Str("remote", conn.Address).Msg("Connected to peer (outgoing)")

	// Send hello
	hello := network.HelloMessage{
		DeviceName: e.cfg.Device.Name,
		DeviceID:   e.cfg.Device.Name,
		Version:    network.ProtocolVersion,
	}
	_ = conn.SendPayload(network.MsgHello, hello)
}

func (e *Engine) onServerDisconnect(conn *network.ClientConnection) {
	log.Info().Str("remote", conn.Address).Msg("Disconnected from peer (outgoing)")
}

func (e *Engine) onServerMessage(conn *network.Connection, msg *network.Message) {
	e.handleMessage(msg, conn.DeviceName, func(m *network.Message) error {
		return conn.Send(m)
	})
}

func (e *Engine) onClientMessage(conn *network.ClientConnection, msg *network.Message) {
	e.handleMessage(msg, conn.DeviceName, func(m *network.Message) error {
		return conn.Send(m)
	})
}

func (e *Engine) handleMessage(msg *network.Message, peerName string, send func(*network.Message) error) {
	switch msg.Type {
	case network.MsgHello:
		var hello network.HelloMessage
		if err := msg.DecodePayload(&hello); err != nil {
			log.Error().Err(err).Msg("Failed to decode hello")
			return
		}
		log.Info().Str("peer", hello.DeviceName).Msg("Received hello from peer")

		// Send hello ack
		ack := network.HelloAckMessage{
			DeviceName: e.cfg.Device.Name,
			DeviceID:   e.cfg.Device.Name,
			Accepted:   true,
		}
		ackMsg, _ := network.NewMessage(network.MsgHelloAck, ack)
		_ = send(ackMsg)

		// Trigger sync of all folders
		for _, folder := range e.cfg.Folders {
			if folder.Enabled {
				go func(path string) {
					_ = e.SyncFolder(path)
				}(folder.Path)
			}
		}

	case network.MsgHelloAck:
		var ack network.HelloAckMessage
		if err := msg.DecodePayload(&ack); err != nil {
			log.Error().Err(err).Msg("Failed to decode hello ack")
			return
		}
		log.Info().Str("peer", ack.DeviceName).Bool("accepted", ack.Accepted).Msg("Hello acknowledged")

	case network.MsgFileList:
		var fileList network.FileListMessage
		if err := msg.DecodePayload(&fileList); err != nil {
			log.Error().Err(err).Msg("Failed to decode file list")
			return
		}
		e.handleFileList(fileList, peerName, send)

	case network.MsgFileRequest:
		var req network.FileRequestMessage
		if err := msg.DecodePayload(&req); err != nil {
			log.Error().Err(err).Msg("Failed to decode file request")
			return
		}
		e.handleFileRequest(req, send)

	case network.MsgFileData:
		var fileData network.FileDataMessage
		if err := msg.DecodePayload(&fileData); err != nil {
			log.Error().Err(err).Msg("Failed to decode file data")
			return
		}
		e.handleFileData(fileData, peerName)

	case network.MsgFileDelete:
		var del network.FileDeleteMessage
		if err := msg.DecodePayload(&del); err != nil {
			log.Error().Err(err).Msg("Failed to decode file delete")
			return
		}
		e.handleRemoteDelete(del, peerName)
	}
}

func (e *Engine) handleFileList(fileList network.FileListMessage, peerName string, send func(*network.Message) error) {
	// Map remote folder to local folder by name
	localFolderPath := e.findLocalFolderByName(fileList.FolderName)
	if localFolderPath == "" {
		log.Debug().
			Str("folderName", fileList.FolderName).
			Msg("No matching local folder for received file list")
		return
	}

	log.Debug().
		Str("remoteFolder", fileList.FolderPath).
		Str("localFolder", localFolderPath).
		Int("files", len(fileList.Files)).
		Msg("Received file list")

	// If we can't receive, don't request any files
	if !e.cfg.CanReceive() {
		log.Debug().Msg("Ignoring file list (send_only mode)")
		return
	}

	// Check each file against our state
	for _, remoteFile := range fileList.Files {
		localPath := filepath.Join(localFolderPath, remoteFile.RelPath)

		// Check if local file exists
		localInfo, err := os.Stat(localPath)
		if err != nil {
			// File doesn't exist locally, request it
			req := network.FileRequestMessage{
				FolderPath: fileList.FolderPath,
				FolderName: fileList.FolderName,
				RelPath:    remoteFile.RelPath,
			}
			reqMsg, _ := network.NewMessage(network.MsgFileRequest, req)
			_ = send(reqMsg)
			continue
		}

		// File exists, check if we need to sync
		localHash, _ := fileutil.HashFile(localPath)

		if localHash != remoteFile.Hash {
			// Check for conflict
			conflict := e.conflict.DetectConflict(localFolderPath, remoteFile.RelPath, &ConflictFile{
				Size:       remoteFile.Size,
				ModTime:    remoteFile.ModTime,
				Hash:       remoteFile.Hash,
				DeviceName: peerName,
			})

			if conflict != nil {
				// Auto-resolve if not set to prompt
				resolution, err := e.conflict.AutoResolve(conflict)
				if err != nil {
					log.Error().Err(err).Msg("Failed to auto-resolve conflict")
					continue
				}

				if resolution == ResolutionKeepRemote || resolution == ResolutionKeepBoth {
					// Request the remote file
					req := network.FileRequestMessage{
						FolderPath: fileList.FolderPath,
						FolderName: fileList.FolderName,
						RelPath:    remoteFile.RelPath,
					}
					reqMsg, _ := network.NewMessage(network.MsgFileRequest, req)
					_ = send(reqMsg)
				}
			} else {
				// No conflict, check which is newer
				if remoteFile.ModTime.After(localInfo.ModTime()) {
					// Remote is newer, request it
					req := network.FileRequestMessage{
						FolderPath: fileList.FolderPath,
						FolderName: fileList.FolderName,
						RelPath:    remoteFile.RelPath,
					}
					reqMsg, _ := network.NewMessage(network.MsgFileRequest, req)
					_ = send(reqMsg)
				}
			}
		}
	}
}

func (e *Engine) handleFileRequest(req network.FileRequestMessage, send func(*network.Message) error) {
	fullPath := filepath.Join(req.FolderPath, req.RelPath)

	// Check if it's a directory (skip directories)
	info, err := os.Stat(fullPath)
	if err != nil {
		log.Error().Err(err).Str("path", fullPath).Msg("Failed to stat requested file")
		return
	}
	if info.IsDir() {
		log.Debug().Str("path", fullPath).Msg("Skipping directory in file request")
		return
	}

	// Check if path should be ignored
	if e.cfg.ShouldIgnore(fullPath) {
		log.Debug().Str("path", fullPath).Msg("Skipping ignored file in request")
		return
	}

	// Read file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		log.Error().Err(err).Str("path", fullPath).Msg("Failed to read requested file")
		return
	}

	fi, err := fileutil.GetFileInfo(fullPath, req.FolderPath)
	if err != nil {
		log.Error().Err(err).Str("path", fullPath).Msg("Failed to get file info")
		return
	}

	msg := network.FileDataMessage{
		FolderPath: req.FolderPath,
		FolderName: getFolderName(req.FolderPath),
		RelPath:    req.RelPath,
		Size:       fi.Size,
		ModTime:    fi.ModTime,
		Permission: uint32(fi.Permission),
		Hash:       fi.Hash,
		Data:       data,
	}

	dataMsg, _ := network.NewMessage(network.MsgFileData, msg)
	_ = send(dataMsg)
}

func (e *Engine) handleFileData(fileData network.FileDataMessage, peerName string) {
	// Check if we're allowed to receive files
	if !e.cfg.CanReceive() {
		log.Debug().Str("file", fileData.RelPath).Msg("Ignoring incoming file (send_only mode)")
		return
	}

	// Map remote folder to local folder by name
	localFolderPath := e.findLocalFolderByName(fileData.FolderName)
	if localFolderPath == "" {
		log.Debug().
			Str("folderName", fileData.FolderName).
			Msg("No matching local folder for received file")
		return
	}

	fullPath := filepath.Join(localFolderPath, fileData.RelPath)

	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Error().Err(err).Str("dir", dir).Msg("Failed to create directory")
		return
	}

	// Write file with permissions (file will be owned by current user automatically)
	if err := os.WriteFile(fullPath, fileData.Data, os.FileMode(fileData.Permission)); err != nil {
		log.Error().Err(err).Str("path", fullPath).Msg("Failed to write file")
		return
	}

	// Set modification time
	if err := os.Chtimes(fullPath, fileData.ModTime, fileData.ModTime); err != nil {
		log.Warn().Err(err).Str("path", fullPath).Msg("Failed to set mod time")
	}

	// Update state (use local folder path)
	e.state.UpdateFileState(localFolderPath, &FileState{
		RelPath:    fileData.RelPath,
		Hash:       fileData.Hash,
		Size:       fileData.Size,
		ModTime:    fileData.ModTime,
		Permission: os.FileMode(fileData.Permission),
		SyncedAt:   time.Now(),
		SyncedFrom: peerName,
	})

	// Record activity
	e.addActivity(&SyncActivity{
		Type:       "received",
		FileName:   filepath.Base(fileData.RelPath),
		FolderPath: localFolderPath,
		RelPath:    fileData.RelPath,
		PeerName:   peerName,
		Timestamp:  time.Now(),
	})

	log.Info().
		Str("file", fileData.RelPath).
		Str("folder", localFolderPath).
		Str("from", peerName).
		Msg("Received file")
}

func (e *Engine) handleRemoteDelete(del network.FileDeleteMessage, peerName string) {
	// Check if we're allowed to receive (and thus process deletions)
	if !e.cfg.CanReceive() {
		log.Debug().Str("file", del.RelPath).Msg("Ignoring remote delete (send_only mode)")
		return
	}

	// Map remote folder to local folder by name
	localFolderPath := e.findLocalFolderByName(del.FolderName)
	if localFolderPath == "" {
		log.Debug().
			Str("folderName", del.FolderName).
			Msg("No matching local folder for delete request")
		return
	}

	fullPath := filepath.Join(localFolderPath, del.RelPath)

	// Delete local file
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		log.Error().Err(err).Str("path", fullPath).Msg("Failed to delete file")
		return
	}

	// Update state
	e.state.RemoveFileState(localFolderPath, del.RelPath)

	// Record activity
	e.addActivity(&SyncActivity{
		Type:       "deleted",
		FileName:   filepath.Base(del.RelPath),
		FolderPath: localFolderPath,
		RelPath:    del.RelPath,
		PeerName:   peerName,
		Timestamp:  time.Now(),
	})

	log.Info().
		Str("file", del.RelPath).
		Str("folder", localFolderPath).
		Str("from", peerName).
		Msg("Deleted file (remote request)")
}

func (e *Engine) addActivity(activity *SyncActivity) {
	e.activityMu.Lock()
	defer e.activityMu.Unlock()

	e.activities = append([]*SyncActivity{activity}, e.activities...)

	// Trim to max
	if len(e.activities) > e.maxActivities {
		e.activities = e.activities[:e.maxActivities]
	}

	if e.onActivity != nil {
		e.onActivity(activity)
	}
}

// GetActivities returns recent sync activities
func (e *Engine) GetActivities(limit int) []*SyncActivity {
	e.activityMu.RLock()
	defer e.activityMu.RUnlock()

	if limit <= 0 || limit > len(e.activities) {
		limit = len(e.activities)
	}

	result := make([]*SyncActivity, limit)
	copy(result, e.activities[:limit])
	return result
}

// GetConflicts returns unresolved conflicts
func (e *Engine) GetConflicts() []*Conflict {
	return e.conflict.GetConflicts()
}

// ResolveConflict resolves a conflict
func (e *Engine) ResolveConflict(conflictID string, resolution ConflictResolution) error {
	conflict := e.conflict.GetConflict(conflictID)
	if conflict == nil {
		return fmt.Errorf("conflict not found: %s", conflictID)
	}
	return e.conflict.ResolveConflict(conflict, resolution)
}

// GetWatcher returns the file watcher
func (e *Engine) GetWatcher() *Watcher {
	return e.watcher
}

// GetState returns the state store
func (e *Engine) GetState() *StateStore {
	return e.state
}

// Ensure implements io.Closer
var _ io.Closer = (*Engine)(nil)

// Close implements io.Closer
func (e *Engine) Close() error {
	e.Stop()
	return nil
}
