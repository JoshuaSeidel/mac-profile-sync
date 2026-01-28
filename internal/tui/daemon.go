package tui

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
)

// DaemonStatusMsg reports daemon running status
type DaemonStatusMsg struct {
	Running bool
}

// DaemonToggleMsg requests to start/stop daemon
type DaemonToggleMsg struct {
	Start bool
}

// checkDaemonStatus checks if the daemon is running by checking if the port is in use
func (a *ConfigApp) checkDaemonStatus() tea.Cmd {
	return func() tea.Msg {
		port := a.cfg.Network.Port
		conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err != nil {
			return DaemonStatusMsg{Running: false}
		}
		_ = conn.Close()
		return DaemonStatusMsg{Running: true}
	}
}

// startDaemon starts the sync daemon in the background
func (a *ConfigApp) startDaemon() tea.Cmd {
	return func() tea.Msg {
		// Get the path to our executable
		exePath, err := os.Executable()
		if err != nil {
			return DaemonStatusMsg{Running: false}
		}

		// Get log file path
		homeDir, _ := os.UserHomeDir()
		logFile := filepath.Join(homeDir, ".mac-profile-sync", "sync.log")

		// Open log file for appending
		logF, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return DaemonStatusMsg{Running: false}
		}

		// Start daemon process
		cmd := exec.Command(exePath, "-v")
		cmd.Stdout = logF
		cmd.Stderr = logF
		cmd.SysProcAttr = &syscall.SysProcAttr{
			Setpgid: true, // Create new process group so it doesn't die with TUI
		}

		if err := cmd.Start(); err != nil {
			_ = logF.Close()
			return DaemonStatusMsg{Running: false}
		}

		// Don't wait for it - let it run in background
		go func() {
			_ = cmd.Wait()
			_ = logF.Close()
		}()

		return DaemonStatusMsg{Running: true}
	}
}

// stopDaemon stops the sync daemon
func (a *ConfigApp) stopDaemon() tea.Cmd {
	return func() tea.Msg {
		// Find and kill the daemon process
		// Use pkill to find processes matching our name
		cmd := exec.Command("pkill", "-f", "mac-profile-sync")
		_ = cmd.Run() // Ignore errors - might not be running

		return DaemonStatusMsg{Running: false}
	}
}
