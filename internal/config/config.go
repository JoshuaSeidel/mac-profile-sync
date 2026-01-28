package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Config holds all application configuration
type Config struct {
	Device   DeviceConfig   `mapstructure:"device"`
	Folders  []FolderConfig `mapstructure:"folders"`
	Sync     SyncConfig     `mapstructure:"sync"`
	Network  NetworkConfig  `mapstructure:"network"`
	Security SecurityConfig `mapstructure:"security"`
}

// DeviceConfig identifies this device
type DeviceConfig struct {
	Name string `mapstructure:"name"`
}

// FolderConfig defines a folder to sync
type FolderConfig struct {
	Path    string `mapstructure:"path"`
	Enabled bool   `mapstructure:"enabled"`
}

// SyncConfig defines sync behavior
type SyncConfig struct {
	ConflictResolution string   `mapstructure:"conflict_resolution"`
	IgnorePatterns     []string `mapstructure:"ignore_patterns"`
}

// NetworkConfig defines network settings
type NetworkConfig struct {
	Port         int      `mapstructure:"port"`
	UseDiscovery bool     `mapstructure:"use_discovery"`
	ManualPeers  []string `mapstructure:"manual_peers"`
}

// SecurityConfig defines security settings
type SecurityConfig struct {
	RequirePairing bool `mapstructure:"require_pairing"`
	Encryption     bool `mapstructure:"encryption"`
}

// ConflictStrategy represents how to handle conflicts
type ConflictStrategy string

const (
	ConflictNewestWins ConflictStrategy = "newest_wins"
	ConflictKeepBoth   ConflictStrategy = "keep_both"
	ConflictPrompt     ConflictStrategy = "prompt"
)

var (
	configDir  string
	configFile string
)

func init() {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	configDir = filepath.Join(home, ".mac-profile-sync")
	configFile = filepath.Join(configDir, "config.yaml")
}

// ConfigDir returns the configuration directory path
func ConfigDir() string {
	return configDir
}

// ConfigFile returns the configuration file path
func ConfigFile() string {
	return configFile
}

// Load reads configuration from file or creates default
func Load() (*Config, error) {
	// Ensure config directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create config directory: %w", err)
	}

	viper.SetConfigFile(configFile)
	viper.SetConfigType("yaml")

	// Set defaults
	setDefaults()

	// Try to read config file
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			// Config file not found, create default
			if err := createDefaultConfig(); err != nil {
				return nil, fmt.Errorf("failed to create default config: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Expand paths
	cfg.expandPaths()

	return &cfg, nil
}

// Save writes the current configuration to file
func Save(cfg *Config) error {
	viper.Set("device", cfg.Device)
	viper.Set("folders", cfg.Folders)
	viper.Set("sync", cfg.Sync)
	viper.Set("network", cfg.Network)
	viper.Set("security", cfg.Security)

	return viper.WriteConfig()
}

func setDefaults() {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "My-Mac"
	}

	viper.SetDefault("device.name", hostname)
	viper.SetDefault("folders", []FolderConfig{
		{Path: "~/Desktop", Enabled: true},
		{Path: "~/Documents", Enabled: true},
	})
	viper.SetDefault("sync.conflict_resolution", "newest_wins")
	viper.SetDefault("sync.ignore_patterns", []string{
		".DS_Store",
		"*.tmp",
		".git",
		"node_modules",
		".Trash",
		"*.swp",
		"*~",
	})
	viper.SetDefault("network.port", 9876)
	viper.SetDefault("network.use_discovery", true)
	viper.SetDefault("network.manual_peers", []string{})
	viper.SetDefault("security.require_pairing", true)
	viper.SetDefault("security.encryption", true)
}

func createDefaultConfig() error {
	setDefaults()

	// Ensure directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return err
	}

	return viper.SafeWriteConfig()
}

func (c *Config) expandPaths() {
	home, _ := os.UserHomeDir()
	for i := range c.Folders {
		c.Folders[i].Path = expandPath(c.Folders[i].Path, home)
	}
}

func expandPath(path, home string) string {
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	if path == "~" {
		return home
	}
	return path
}

// GetConflictStrategy returns the configured conflict resolution strategy
func (c *Config) GetConflictStrategy() ConflictStrategy {
	switch c.Sync.ConflictResolution {
	case "newest_wins":
		return ConflictNewestWins
	case "keep_both":
		return ConflictKeepBoth
	case "prompt":
		return ConflictPrompt
	default:
		return ConflictNewestWins
	}
}

// AddFolder adds a new folder to sync
func (c *Config) AddFolder(path string) error {
	home, _ := os.UserHomeDir()
	expandedPath := expandPath(path, home)

	// Check if folder already exists
	for _, f := range c.Folders {
		if f.Path == expandedPath {
			return fmt.Errorf("folder already configured: %s", path)
		}
	}

	// Verify path exists and is a directory
	info, err := os.Stat(expandedPath)
	if err != nil {
		return fmt.Errorf("cannot access path: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	c.Folders = append(c.Folders, FolderConfig{
		Path:    expandedPath,
		Enabled: true,
	})

	return Save(c)
}

// RemoveFolder removes a folder from sync
func (c *Config) RemoveFolder(path string) error {
	home, _ := os.UserHomeDir()
	expandedPath := expandPath(path, home)

	for i, f := range c.Folders {
		if f.Path == expandedPath {
			c.Folders = append(c.Folders[:i], c.Folders[i+1:]...)
			return Save(c)
		}
	}

	return fmt.Errorf("folder not found: %s", path)
}

// ToggleFolder enables or disables a folder
func (c *Config) ToggleFolder(path string) error {
	home, _ := os.UserHomeDir()
	expandedPath := expandPath(path, home)

	for i, f := range c.Folders {
		if f.Path == expandedPath {
			c.Folders[i].Enabled = !c.Folders[i].Enabled
			return Save(c)
		}
	}

	return fmt.Errorf("folder not found: %s", path)
}

// ShouldIgnore checks if a path matches any ignore pattern
func (c *Config) ShouldIgnore(path string) bool {
	base := filepath.Base(path)
	for _, pattern := range c.Sync.IgnorePatterns {
		matched, _ := filepath.Match(pattern, base)
		if matched {
			return true
		}
	}
	return false
}
