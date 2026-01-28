package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/jseidel/mac-profile-sync/internal/config"
	"github.com/jseidel/mac-profile-sync/pkg/fileutil"
)

// Conflict represents a sync conflict
type Conflict struct {
	ID          string       `json:"id"`
	FolderPath  string       `json:"folder_path"`
	RelPath     string       `json:"rel_path"`
	LocalFile   *ConflictFile `json:"local_file"`
	RemoteFile  *ConflictFile `json:"remote_file"`
	DetectedAt  time.Time    `json:"detected_at"`
	Resolved    bool         `json:"resolved"`
	Resolution  string       `json:"resolution"`
}

// ConflictFile contains file info for conflict comparison
type ConflictFile struct {
	Size       int64     `json:"size"`
	ModTime    time.Time `json:"mod_time"`
	Hash       string    `json:"hash"`
	DeviceName string    `json:"device_name"`
}

// ConflictResolution defines how a conflict should be resolved
type ConflictResolution string

const (
	ResolutionKeepLocal  ConflictResolution = "keep_local"
	ResolutionKeepRemote ConflictResolution = "keep_remote"
	ResolutionKeepBoth   ConflictResolution = "keep_both"
	ResolutionSkip       ConflictResolution = "skip"
)

// ConflictDetector detects and manages conflicts
type ConflictDetector struct {
	cfg        *config.Config
	state      *StateStore
	conflicts  map[string]*Conflict
	onConflict func(*Conflict)
}

// NewConflictDetector creates a new conflict detector
func NewConflictDetector(cfg *config.Config, state *StateStore) *ConflictDetector {
	return &ConflictDetector{
		cfg:       cfg,
		state:     state,
		conflicts: make(map[string]*Conflict),
	}
}

// SetCallback sets the callback for new conflicts
func (cd *ConflictDetector) SetCallback(fn func(*Conflict)) {
	cd.onConflict = fn
}

// DetectConflict checks if there's a conflict between local and remote versions
func (cd *ConflictDetector) DetectConflict(folderPath, relPath string, remoteFile *ConflictFile) *Conflict {
	fullPath := filepath.Join(folderPath, relPath)

	// Check if local file exists
	localInfo, err := os.Stat(fullPath)
	if err != nil {
		// No local file, no conflict
		return nil
	}

	// Get local file hash
	localHash, err := fileutil.HashFile(fullPath)
	if err != nil {
		return nil
	}

	// If hashes match, no conflict
	if localHash == remoteFile.Hash {
		return nil
	}

	// Get known state
	knownState := cd.state.GetFileState(folderPath, relPath)

	// If we don't have a known state, check if files are identical
	if knownState == nil {
		// Files differ and we have no history - conflict
		conflict := &Conflict{
			ID:         fmt.Sprintf("%s:%s", folderPath, relPath),
			FolderPath: folderPath,
			RelPath:    relPath,
			LocalFile: &ConflictFile{
				Size:    localInfo.Size(),
				ModTime: localInfo.ModTime(),
				Hash:    localHash,
			},
			RemoteFile: remoteFile,
			DetectedAt: time.Now(),
		}

		cd.conflicts[conflict.ID] = conflict
		if cd.onConflict != nil {
			cd.onConflict(conflict)
		}
		return conflict
	}

	// Compare with known state
	// If local changed since last sync AND remote is different from what we synced
	localChanged := localHash != knownState.Hash
	remoteChanged := remoteFile.Hash != knownState.Hash

	if localChanged && remoteChanged {
		// Both sides changed - conflict
		conflict := &Conflict{
			ID:         fmt.Sprintf("%s:%s", folderPath, relPath),
			FolderPath: folderPath,
			RelPath:    relPath,
			LocalFile: &ConflictFile{
				Size:    localInfo.Size(),
				ModTime: localInfo.ModTime(),
				Hash:    localHash,
			},
			RemoteFile: remoteFile,
			DetectedAt: time.Now(),
		}

		cd.conflicts[conflict.ID] = conflict
		if cd.onConflict != nil {
			cd.onConflict(conflict)
		}
		return conflict
	}

	return nil
}

// ResolveConflict resolves a conflict according to the given resolution
func (cd *ConflictDetector) ResolveConflict(conflict *Conflict, resolution ConflictResolution) error {
	fullPath := filepath.Join(conflict.FolderPath, conflict.RelPath)

	switch resolution {
	case ResolutionKeepLocal:
		// Nothing to do, local file stays
		conflict.Resolution = "kept_local"

	case ResolutionKeepRemote:
		// Remote file will be applied by sync engine
		conflict.Resolution = "kept_remote"

	case ResolutionKeepBoth:
		// Rename local file to include device name
		localDevice := cd.cfg.Device.Name
		conflictPath := fileutil.GenerateConflictName(fullPath, localDevice)
		if err := os.Rename(fullPath, conflictPath); err != nil {
			return fmt.Errorf("failed to rename local file: %w", err)
		}
		conflict.Resolution = "kept_both"

	case ResolutionSkip:
		conflict.Resolution = "skipped"
	}

	conflict.Resolved = true
	delete(cd.conflicts, conflict.ID)

	return nil
}

// AutoResolve automatically resolves a conflict based on configuration
func (cd *ConflictDetector) AutoResolve(conflict *Conflict) (ConflictResolution, error) {
	strategy := cd.cfg.GetConflictStrategy()

	switch strategy {
	case config.ConflictNewestWins:
		if conflict.LocalFile.ModTime.After(conflict.RemoteFile.ModTime) {
			return ResolutionKeepLocal, cd.ResolveConflict(conflict, ResolutionKeepLocal)
		}
		return ResolutionKeepRemote, cd.ResolveConflict(conflict, ResolutionKeepRemote)

	case config.ConflictKeepBoth:
		return ResolutionKeepBoth, cd.ResolveConflict(conflict, ResolutionKeepBoth)

	case config.ConflictPrompt:
		// Don't auto-resolve, return skip for now
		return ResolutionSkip, nil

	default:
		return ResolutionKeepLocal, cd.ResolveConflict(conflict, ResolutionKeepLocal)
	}
}

// GetConflicts returns all unresolved conflicts
func (cd *ConflictDetector) GetConflicts() []*Conflict {
	conflicts := make([]*Conflict, 0, len(cd.conflicts))
	for _, c := range cd.conflicts {
		conflicts = append(conflicts, c)
	}
	return conflicts
}

// GetConflict returns a specific conflict by ID
func (cd *ConflictDetector) GetConflict(id string) *Conflict {
	return cd.conflicts[id]
}

// HasConflicts returns true if there are unresolved conflicts
func (cd *ConflictDetector) HasConflicts() bool {
	return len(cd.conflicts) > 0
}

// ClearConflicts removes all conflicts
func (cd *ConflictDetector) ClearConflicts() {
	cd.conflicts = make(map[string]*Conflict)
}
