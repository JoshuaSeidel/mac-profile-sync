package fileutil

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// FileInfo contains metadata about a file
type FileInfo struct {
	Path       string    `json:"path"`
	RelPath    string    `json:"rel_path"`
	Size       int64     `json:"size"`
	ModTime    time.Time `json:"mod_time"`
	Hash       string    `json:"hash"`
	IsDir      bool      `json:"is_dir"`
	Permission os.FileMode `json:"permission"`
}

// HashFile computes SHA256 hash of a file
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = f.Close() }()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}

	return hex.EncodeToString(h.Sum(nil)), nil
}

// GetFileInfo retrieves metadata for a file
func GetFileInfo(path string, basePath string) (*FileInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	relPath, err := filepath.Rel(basePath, path)
	if err != nil {
		relPath = path
	}

	fi := &FileInfo{
		Path:       path,
		RelPath:    relPath,
		Size:       info.Size(),
		ModTime:    info.ModTime(),
		IsDir:      info.IsDir(),
		Permission: info.Mode().Perm(),
	}

	// Only hash regular files
	if !info.IsDir() && info.Size() > 0 {
		hash, err := HashFile(path)
		if err != nil {
			return nil, err
		}
		fi.Hash = hash
	}

	return fi, nil
}

// CopyFile copies a file from src to dst, preserving permissions and mod time
func CopyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer func() { _ = srcFile.Close() }()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat source: %w", err)
	}

	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	dstFile, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, srcInfo.Mode().Perm())
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer func() { _ = dstFile.Close() }()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	// Preserve modification time
	if err := os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		return fmt.Errorf("failed to set mod time: %w", err)
	}

	return nil
}

// FormatSize returns a human-readable file size
func FormatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// FormatTime returns a human-readable relative time
func FormatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return fmt.Sprintf("%ds ago", int(diff.Seconds()))
	case diff < time.Hour:
		return fmt.Sprintf("%dm ago", int(diff.Minutes()))
	case diff < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(diff.Hours()))
	default:
		return t.Format("Jan 2, 2006 3:04 PM")
	}
}

// EnsureDir creates a directory if it doesn't exist
func EnsureDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// Exists checks if a path exists
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsDir checks if a path is a directory
func IsDir(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CountFiles returns the number of files in a directory (non-recursive)
func CountFiles(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, err
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			count++
		}
	}
	return count, nil
}

// CountFilesRecursive returns the total number of files in a directory tree
func CountFilesRecursive(dir string) (int, error) {
	count := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		if !info.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

// GenerateConflictName creates a conflict filename (e.g., "file_conflict_20060102_150405.txt")
func GenerateConflictName(originalPath string, deviceName string) string {
	dir := filepath.Dir(originalPath)
	ext := filepath.Ext(originalPath)
	base := filepath.Base(originalPath)
	name := base[:len(base)-len(ext)]

	timestamp := time.Now().Format("20060102_150405")
	newName := fmt.Sprintf("%s_%s_conflict_%s%s", name, deviceName, timestamp, ext)

	return filepath.Join(dir, newName)
}
