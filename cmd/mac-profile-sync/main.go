package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/jseidel/mac-profile-sync/internal/config"
	"github.com/jseidel/mac-profile-sync/internal/discovery"
	"github.com/jseidel/mac-profile-sync/internal/network"
	"github.com/jseidel/mac-profile-sync/internal/sync"
	"github.com/jseidel/mac-profile-sync/internal/tui"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Set up logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	// Root command
	rootCmd := &cobra.Command{
		Use:   "mac-profile-sync",
		Short: "Sync folders between Macs in real-time",
		Long: `Mac Profile Sync keeps folders (Desktop, Documents, etc.) synchronized
between two Macs in real-time using filesystem watching, with Bonjour
auto-discovery and configurable conflict resolution.`,
		RunE: runApp,
	}

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("mac-profile-sync %s\n", version)
			fmt.Printf("  commit: %s\n", commit)
			fmt.Printf("  built:  %s\n", date)
		},
	}

	// Status command
	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show sync status",
		RunE:  runStatus,
	}

	// Add folder command
	addCmd := &cobra.Command{
		Use:   "add [path]",
		Short: "Add a folder to sync",
		Args:  cobra.ExactArgs(1),
		RunE:  runAdd,
	}

	// Remove folder command
	removeCmd := &cobra.Command{
		Use:   "remove [path]",
		Short: "Remove a folder from sync",
		Args:  cobra.ExactArgs(1),
		RunE:  runRemove,
	}

	// List peers command
	peersCmd := &cobra.Command{
		Use:   "peers",
		Short: "List discovered peers",
		RunE:  runPeers,
	}

	// Daemon command
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Run as background daemon (no TUI)",
		RunE:  runDaemon,
	}

	// Add commands
	rootCmd.AddCommand(versionCmd, statusCmd, addCmd, removeCmd, peersCmd, daemonCmd)

	// Flags
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose logging")
	rootCmd.PersistentFlags().StringP("config", "c", "", "Config file path")

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runApp(cmd *cobra.Command, args []string) error {
	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	log.Info().Str("device", cfg.Device.Name).Msg("Starting Mac Profile Sync")

	// Create network components
	server := network.NewServer(cfg.Network.Port, nil) // TLS can be added later
	client := network.NewClient(nil)

	// Create discovery service
	disc := discovery.NewDiscovery(
		cfg.Device.Name,
		cfg.Network.Port,
		cfg.Network.UseDiscovery,
		cfg.Network.ManualPeers,
	)

	// Create sync engine
	engine, err := sync.NewEngine(cfg, server, client)
	if err != nil {
		return fmt.Errorf("failed to create sync engine: %w", err)
	}

	// Set up discovery callbacks
	disc.SetCallbacks(
		func(peer *discovery.Peer) {
			log.Info().Str("peer", peer.Name).Msg("Peer found")
			// Connect to discovered peer
			go func() {
				if _, err := client.Connect(peer.Address()); err != nil {
					log.Error().Err(err).Str("peer", peer.Name).Msg("Failed to connect to peer")
				}
			}()
		},
		func(peer *discovery.Peer) {
			log.Info().Str("peer", peer.Name).Msg("Peer lost")
		},
	)

	// Start services
	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	defer server.Stop()

	if err := disc.Start(); err != nil {
		return fmt.Errorf("failed to start discovery: %w", err)
	}
	defer disc.Stop()

	if err := engine.Start(); err != nil {
		return fmt.Errorf("failed to start sync engine: %w", err)
	}
	defer engine.Stop()

	// Run TUI
	return tui.Run(cfg, disc, engine)
}

func runDaemon(cmd *cobra.Command, args []string) error {
	verbose, _ := cmd.Flags().GetBool("verbose")
	if verbose {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	log.Info().Str("device", cfg.Device.Name).Msg("Starting Mac Profile Sync daemon")

	// Create network components
	server := network.NewServer(cfg.Network.Port, nil)
	client := network.NewClient(nil)

	// Create discovery service
	disc := discovery.NewDiscovery(
		cfg.Device.Name,
		cfg.Network.Port,
		cfg.Network.UseDiscovery,
		cfg.Network.ManualPeers,
	)

	// Create sync engine
	engine, err := sync.NewEngine(cfg, server, client)
	if err != nil {
		return fmt.Errorf("failed to create sync engine: %w", err)
	}

	// Set up discovery callbacks
	disc.SetCallbacks(
		func(peer *discovery.Peer) {
			log.Info().Str("peer", peer.Name).Msg("Peer found")
			go func() {
				if _, err := client.Connect(peer.Address()); err != nil {
					log.Error().Err(err).Str("peer", peer.Name).Msg("Failed to connect to peer")
				}
			}()
		},
		func(peer *discovery.Peer) {
			log.Info().Str("peer", peer.Name).Msg("Peer lost")
		},
	)

	// Start services
	if err := server.Start(); err != nil {
		return fmt.Errorf("failed to start server: %w", err)
	}
	defer server.Stop()

	if err := disc.Start(); err != nil {
		return fmt.Errorf("failed to start discovery: %w", err)
	}
	defer disc.Stop()

	if err := engine.Start(); err != nil {
		return fmt.Errorf("failed to start sync engine: %w", err)
	}
	defer engine.Stop()

	log.Info().Msg("Daemon running. Press Ctrl+C to stop.")

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Info().Msg("Shutting down...")
	return nil
}

func runStatus(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Mac Profile Sync Status\n")
	fmt.Printf("=======================\n\n")
	fmt.Printf("Device: %s\n", cfg.Device.Name)
	fmt.Printf("Port: %d\n", cfg.Network.Port)
	fmt.Printf("Discovery: %v\n", cfg.Network.UseDiscovery)
	fmt.Printf("\nSynced Folders:\n")

	for _, folder := range cfg.Folders {
		status := "enabled"
		if !folder.Enabled {
			status = "disabled"
		}
		fmt.Printf("  %s (%s)\n", folder.Path, status)
	}

	fmt.Printf("\nConflict Resolution: %s\n", cfg.Sync.ConflictResolution)

	return nil
}

func runAdd(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	path := args[0]
	if err := cfg.AddFolder(path); err != nil {
		return err
	}

	fmt.Printf("Added folder: %s\n", path)
	return nil
}

func runRemove(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	path := args[0]
	if err := cfg.RemoveFolder(path); err != nil {
		return err
	}

	fmt.Printf("Removed folder: %s\n", path)
	return nil
}

func runPeers(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	fmt.Printf("Searching for peers...\n")

	// Create discovery service
	disc := discovery.NewDiscovery(
		cfg.Device.Name,
		cfg.Network.Port,
		cfg.Network.UseDiscovery,
		cfg.Network.ManualPeers,
	)

	disc.SetCallbacks(
		func(peer *discovery.Peer) {
			fmt.Printf("  Found: %s (%s)\n", peer.Name, peer.Address())
		},
		nil,
	)

	if err := disc.Start(); err != nil {
		return fmt.Errorf("failed to start discovery: %w", err)
	}
	defer disc.Stop()

	// Wait for discovery
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Press Ctrl+C to stop...")
	<-sigCh

	return nil
}
