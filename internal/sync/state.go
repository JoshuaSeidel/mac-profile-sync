package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/jseidel/mac-profile-sync/internal/config"
)

// FileState represents the known state of a file
type FileState struct {
	RelPath    string      `json:"rel_path"`
	Hash       string      `json:"hash"`
	Size       int64       `json:"size"`
	ModTime    time.Time   `json:"mod_time"`
	Permission os.FileMode `json:"permission"`
	SyncedAt   time.Time   `json:"synced_at"`
	SyncedFrom string      `json:"synced_from"` // Device name that last synced this file
}

// FolderState represents the state of all files in a folder
type FolderState struct {
	Path     string                `json:"path"`
	Files    map[string]*FileState `json:"files"`
	UpdatedAt time.Time            `json:"updated_at"`
}

// StateStore manages sync state persistence
type StateStore struct {
	mu       sync.RWMutex
	folders  map[string]*FolderState
	stateDir string
}

// NewStateStore creates a new state store
func NewStateStore() *StateStore {
	return &StateStore{
		folders:  make(map[string]*FolderState),
		stateDir: filepath.Join(config.ConfigDir(), "state"),
	}
}

// Load loads state from disk
func (s *StateStore) Load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Ensure state directory exists
	if err := os.MkdirAll(s.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	// Read all state files
	entries, err := os.ReadDir(s.stateDir)
	if err != nil {
		return fmt.Errorf("failed to read state directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(s.stateDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var fs FolderState
		if err := json.Unmarshal(data, &fs); err != nil {
			continue
		}

		s.folders[fs.Path] = &fs
	}

	return nil
}

// Save persists state to disk
func (s *StateStore) Save() error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Ensure state directory exists
	if err := os.MkdirAll(s.stateDir, 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	for _, fs := range s.folders {
		data, err := json.MarshalIndent(fs, "", "  ")
		if err != nil {
			continue
		}

		filename := fmt.Sprintf("%x.json", hashString(fs.Path))
		path := filepath.Join(s.stateDir, filename)
		if err := os.WriteFile(path, data, 0644); err != nil {
			return fmt.Errorf("failed to write state file: %w", err)
		}
	}

	return nil
}

// GetFolderState returns the state for a folder
func (s *StateStore) GetFolderState(folderPath string) *FolderState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fs, ok := s.folders[folderPath]
	if !ok {
		return nil
	}
	return fs
}

// GetFileState returns the state for a specific file
func (s *StateStore) GetFileState(folderPath, relPath string) *FileState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fs, ok := s.folders[folderPath]
	if !ok {
		return nil
	}

	return fs.Files[relPath]
}

// UpdateFileState updates the state for a file
func (s *StateStore) UpdateFileState(folderPath string, state *FileState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fs, ok := s.folders[folderPath]
	if !ok {
		fs = &FolderState{
			Path:  folderPath,
			Files: make(map[string]*FileState),
		}
		s.folders[folderPath] = fs
	}

	fs.Files[state.RelPath] = state
	fs.UpdatedAt = time.Now()
}

// RemoveFileState removes the state for a file
func (s *StateStore) RemoveFileState(folderPath, relPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	fs, ok := s.folders[folderPath]
	if !ok {
		return
	}

	delete(fs.Files, relPath)
	fs.UpdatedAt = time.Now()
}

// GetAllFiles returns all tracked files in a folder
func (s *StateStore) GetAllFiles(folderPath string) map[string]*FileState {
	s.mu.RLock()
	defer s.mu.RUnlock()

	fs, ok := s.folders[folderPath]
	if !ok {
		return nil
	}

	// Return a copy to avoid race conditions
	files := make(map[string]*FileState, len(fs.Files))
	for k, v := range fs.Files {
		files[k] = v
	}
	return files
}

// InitFolder initializes state tracking for a folder
func (s *StateStore) InitFolder(folderPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.folders[folderPath]; !ok {
		s.folders[folderPath] = &FolderState{
			Path:      folderPath,
			Files:     make(map[string]*FileState),
			UpdatedAt: time.Now(),
		}
	}
}

// ClearFolder removes all state for a folder
func (s *StateStore) ClearFolder(folderPath string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.folders, folderPath)

	// Also remove the state file
	filename := fmt.Sprintf("%x.json", hashString(folderPath))
	path := filepath.Join(s.stateDir, filename)
	os.Remove(path)
}

// hashString creates a simple hash of a string for filenames
func hashString(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
