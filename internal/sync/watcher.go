package sync

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jseidel/mac-profile-sync/internal/config"
	"github.com/rs/zerolog/log"
)

// FileEvent represents a file system event
type FileEvent struct {
	Type       EventType
	Path       string
	RelPath    string
	FolderPath string
	Timestamp  time.Time
}

// EventType represents the type of file event
type EventType int

const (
	EventCreate EventType = iota
	EventModify
	EventDelete
	EventRename
)

func (e EventType) String() string {
	switch e {
	case EventCreate:
		return "create"
	case EventModify:
		return "modify"
	case EventDelete:
		return "delete"
	case EventRename:
		return "rename"
	default:
		return "unknown"
	}
}

// Watcher monitors folders for file changes
type Watcher struct {
	cfg     *config.Config
	watcher *fsnotify.Watcher
	events  chan FileEvent
	done    chan struct{}
	mu      sync.RWMutex
	folders map[string]bool // Active watched folders

	// Debouncing
	pendingEvents map[string]*FileEvent
	debounceTimer *time.Timer
	debounceMu    sync.Mutex
}

// NewWatcher creates a new file watcher
func NewWatcher(cfg *config.Config) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	return &Watcher{
		cfg:           cfg,
		watcher:       fsWatcher,
		events:        make(chan FileEvent, 100),
		done:          make(chan struct{}),
		folders:       make(map[string]bool),
		pendingEvents: make(map[string]*FileEvent),
	}, nil
}

// Events returns the channel of file events
func (w *Watcher) Events() <-chan FileEvent {
	return w.events
}

// Start begins watching configured folders
func (w *Watcher) Start() error {
	// Watch enabled folders
	for _, folder := range w.cfg.Folders {
		if folder.Enabled {
			if err := w.AddFolder(folder.Path); err != nil {
				log.Error().Err(err).Str("path", folder.Path).Msg("Failed to watch folder")
			}
		}
	}

	// Start event processing loop
	go w.processEvents()

	return nil
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	close(w.done)
	w.watcher.Close()
}

// AddFolder adds a folder to watch (recursively)
func (w *Watcher) AddFolder(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if already watching
	if w.folders[path] {
		return nil
	}

	// Walk the directory tree and add all directories
	err := filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip ignored paths
		if w.cfg.ShouldIgnore(walkPath) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Watch directories
		if info.IsDir() {
			if err := w.watcher.Add(walkPath); err != nil {
				log.Warn().Err(err).Str("path", walkPath).Msg("Failed to add watch")
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	w.folders[path] = true
	log.Info().Str("path", path).Msg("Watching folder")

	return nil
}

// RemoveFolder removes a folder from watching
func (w *Watcher) RemoveFolder(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.folders[path] {
		return nil
	}

	// Remove all watches under this path
	filepath.Walk(path, func(walkPath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			w.watcher.Remove(walkPath)
		}
		return nil
	})

	delete(w.folders, path)
	log.Info().Str("path", path).Msg("Stopped watching folder")

	return nil
}

func (w *Watcher) processEvents() {
	for {
		select {
		case <-w.done:
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleFsEvent(event)

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			log.Error().Err(err).Msg("Watcher error")
		}
	}
}

func (w *Watcher) handleFsEvent(event fsnotify.Event) {
	// Skip ignored files
	if w.cfg.ShouldIgnore(event.Name) {
		return
	}

	// Determine folder path and relative path
	folderPath, relPath := w.resolvePaths(event.Name)
	if folderPath == "" {
		return
	}

	var eventType EventType
	switch {
	case event.Op&fsnotify.Create == fsnotify.Create:
		eventType = EventCreate
		// If a new directory is created, add it to the watch
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			w.watcher.Add(event.Name)
		}
	case event.Op&fsnotify.Write == fsnotify.Write:
		eventType = EventModify
	case event.Op&fsnotify.Remove == fsnotify.Remove:
		eventType = EventDelete
	case event.Op&fsnotify.Rename == fsnotify.Rename:
		eventType = EventRename
	default:
		return
	}

	fileEvent := &FileEvent{
		Type:       eventType,
		Path:       event.Name,
		RelPath:    relPath,
		FolderPath: folderPath,
		Timestamp:  time.Now(),
	}

	// Debounce events
	w.debounceEvent(fileEvent)
}

func (w *Watcher) resolvePaths(path string) (folderPath, relPath string) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	for folder := range w.folders {
		if rel, err := filepath.Rel(folder, path); err == nil && len(rel) > 0 && rel[0] != '.' {
			return folder, rel
		}
	}

	return "", ""
}

func (w *Watcher) debounceEvent(event *FileEvent) {
	w.debounceMu.Lock()
	defer w.debounceMu.Unlock()

	// Store the event, newer events override older ones for the same path
	w.pendingEvents[event.Path] = event

	// Reset or start the debounce timer
	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}

	w.debounceTimer = time.AfterFunc(100*time.Millisecond, w.flushPendingEvents)
}

func (w *Watcher) flushPendingEvents() {
	w.debounceMu.Lock()
	events := w.pendingEvents
	w.pendingEvents = make(map[string]*FileEvent)
	w.debounceMu.Unlock()

	for _, event := range events {
		select {
		case w.events <- *event:
		case <-w.done:
			return
		default:
			log.Warn().Str("path", event.Path).Msg("Event channel full, dropping event")
		}
	}
}

// IsWatching returns whether a folder is being watched
func (w *Watcher) IsWatching(path string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.folders[path]
}

// WatchedFolders returns a list of watched folder paths
func (w *Watcher) WatchedFolders() []string {
	w.mu.RLock()
	defer w.mu.RUnlock()

	folders := make([]string, 0, len(w.folders))
	for path := range w.folders {
		folders = append(folders, path)
	}
	return folders
}
