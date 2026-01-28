package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jseidel/mac-profile-sync/internal/config"
	"github.com/jseidel/mac-profile-sync/internal/discovery"
	"github.com/jseidel/mac-profile-sync/internal/sync"
	"github.com/jseidel/mac-profile-sync/pkg/fileutil"
)

// SyncToggleMsg is sent when sync is toggled
type SyncToggleMsg struct {
	Enabled bool
}

// DashboardModel represents the dashboard view
type DashboardModel struct {
	cfg         *config.Config
	peers       []*discovery.Peer
	activities  []*sync.SyncActivity
	conflicts   []*sync.Conflict
	folders     []folderInfo
	width       int
	height      int
	selected    int
	syncRunning bool
}

type folderInfo struct {
	path      string
	enabled   bool
	fileCount int
}

// NewDashboardModel creates a new dashboard model
func NewDashboardModel(cfg *config.Config) *DashboardModel {
	folders := make([]folderInfo, len(cfg.Folders))
	for i, f := range cfg.Folders {
		count, _ := fileutil.CountFilesRecursive(f.Path)
		folders[i] = folderInfo{
			path:      f.Path,
			enabled:   f.Enabled,
			fileCount: count,
		}
	}

	return &DashboardModel{
		cfg:         cfg,
		folders:     folders,
		syncRunning: cfg.IsSyncEnabled(),
	}
}

// Init initializes the dashboard
func (m *DashboardModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m *DashboardModel) Update(msg tea.Msg) (*DashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.folders)-1 {
				m.selected++
			}
		case "s":
			// Toggle sync
			newState := !m.syncRunning
			m.syncRunning = newState
			m.cfg.Sync.Enabled = newState
			config.Save(m.cfg)
			return m, func() tea.Msg {
				return SyncToggleMsg{Enabled: newState}
			}
		}
	}

	return m, nil
}

// View renders the dashboard
func (m *DashboardModel) View() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render("Mac Profile Sync")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Sync status
	b.WriteString("Sync: ")
	if m.syncRunning {
		b.WriteString(connectedStyle.Render("● Running"))
	} else {
		b.WriteString(errorStyle.Render("○ Stopped"))
	}
	b.WriteString("  ")
	b.WriteString(subtitleStyle.Render("(press 's' to toggle)"))
	b.WriteString("\n")

	// Connection status
	connected := len(m.peers) > 0
	b.WriteString("Peer: ")
	if connected {
		peer := m.peers[0]
		b.WriteString(StatusIndicator(true))
		b.WriteString(fmt.Sprintf(" %s (%s)", peer.Name, peer.Address()))
	} else {
		b.WriteString(StatusIndicator(false))
		b.WriteString(subtitleStyle.Render(" Waiting for peers..."))
	}
	b.WriteString("\n")

	b.WriteString("\n")

	// Synced folders box
	foldersBox := m.renderFoldersBox()
	b.WriteString(foldersBox)
	b.WriteString("\n\n")

	// Recent activity
	activityBox := m.renderActivityBox()
	b.WriteString(activityBox)
	b.WriteString("\n\n")

	// Conflicts (if any)
	if len(m.conflicts) > 0 {
		conflictBox := m.renderConflictBox()
		b.WriteString(conflictBox)
		b.WriteString("\n\n")
	}

	// Help bar
	helpBar := m.renderHelpBar()
	b.WriteString(helpBar)

	// Wrap in main box
	content := b.String()
	maxWidth := m.width - 4
	if maxWidth < 50 {
		maxWidth = 50
	}

	return boxStyle.Width(maxWidth).Render(content)
}

func (m *DashboardModel) renderFoldersBox() string {
	var b strings.Builder

	header := "Synced Folders"
	addHint := helpKeyStyle.Render("[a]") + helpDescStyle.Render("dd")

	headerLine := lipgloss.JoinHorizontal(
		lipgloss.Top,
		normalItemStyle.Render(header),
		strings.Repeat(" ", 30-len(header)),
		addHint,
	)
	b.WriteString(headerLine)
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n")

	for i, folder := range m.folders {
		icon := FolderStatusIndicator(folder.enabled)

		// Shorten path
		shortPath := shortenPath(folder.path, 35)

		var countStr string
		if folder.enabled {
			countStr = fmt.Sprintf("%d files", folder.fileCount)
		} else {
			countStr = disabledItemStyle.Render("disabled")
		}

		// Highlight selected
		line := fmt.Sprintf("%s %s", icon, shortPath)
		padding := 45 - lipgloss.Width(line)
		if padding < 1 {
			padding = 1
		}

		if i == m.selected {
			line = selectedItemStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString(strings.Repeat(" ", padding))
		b.WriteString(disabledItemStyle.Render(countStr))
		b.WriteString("\n")
	}

	return innerBoxStyle.Render(b.String())
}

func (m *DashboardModel) renderActivityBox() string {
	var b strings.Builder

	b.WriteString("Recent Activity:\n")

	if len(m.activities) == 0 {
		b.WriteString(subtitleStyle.Render("  No recent activity"))
		return b.String()
	}

	maxShow := 5
	if len(m.activities) < maxShow {
		maxShow = len(m.activities)
	}

	for _, activity := range m.activities[:maxShow] {
		icon := ActivityIcon(activity.Type)
		timeStr := fileutil.FormatTime(activity.Timestamp)
		fileName := activity.FileName

		if len(fileName) > 30 {
			fileName = fileName[:27] + "..."
		}

		var action string
		switch activity.Type {
		case "sent":
			action = "Synced"
		case "received":
			action = "Received"
		case "deleted":
			action = "Deleted"
		}

		line := fmt.Sprintf("%s %s %s", icon, action, fileName)
		padding := 45 - lipgloss.Width(line)
		if padding < 1 {
			padding = 1
		}

		b.WriteString(line)
		b.WriteString(strings.Repeat(" ", padding))
		b.WriteString(mutedStyle.Render(timeStr))
		b.WriteString("\n")
	}

	return b.String()
}

func (m *DashboardModel) renderConflictBox() string {
	count := len(m.conflicts)
	msg := fmt.Sprintf("⚠ %d conflict(s) require attention", count)
	return warningStyle.Render(msg)
}

func (m *DashboardModel) renderHelpBar() string {
	var syncHint string
	if m.syncRunning {
		syncHint = HelpItem("s", "top sync")
	} else {
		syncHint = HelpItem("s", "tart sync")
	}

	items := []string{
		syncHint,
		HelpItem("↑↓", "navigate"),
		HelpItem("q", "uit"),
	}
	return strings.Join(items, " ")
}

// SetPeers updates the peer list
func (m *DashboardModel) SetPeers(peers []*discovery.Peer) {
	m.peers = peers
}

// SetActivities updates the activity list
func (m *DashboardModel) SetActivities(activities []*sync.SyncActivity) {
	m.activities = activities
}

// SetConflicts updates the conflict list
func (m *DashboardModel) SetConflicts(conflicts []*sync.Conflict) {
	m.conflicts = conflicts
}

// RefreshFolders updates folder info
func (m *DashboardModel) RefreshFolders() {
	m.folders = make([]folderInfo, len(m.cfg.Folders))
	for i, f := range m.cfg.Folders {
		count, _ := fileutil.CountFilesRecursive(f.Path)
		m.folders[i] = folderInfo{
			path:      f.Path,
			enabled:   f.Enabled,
			fileCount: count,
		}
	}
}

// SetSyncRunning updates the sync running state
func (m *DashboardModel) SetSyncRunning(running bool) {
	m.syncRunning = running
}

// IsSyncRunning returns whether sync is running
func (m *DashboardModel) IsSyncRunning() bool {
	return m.syncRunning
}

func shortenPath(path string, maxLen int) string {
	home, _ := filepath.Abs(filepath.Join("~"))

	// Replace home with ~
	if strings.HasPrefix(path, home) {
		path = "~" + path[len(home):]
	}

	// Check user home
	userHome := filepath.Join("/Users")
	if strings.HasPrefix(path, userHome) {
		parts := strings.SplitN(path[len(userHome)+1:], string(filepath.Separator), 2)
		if len(parts) == 2 {
			path = "~/" + parts[1]
		}
	}

	if len(path) <= maxLen {
		return path
	}

	// Shorten from the middle
	half := (maxLen - 3) / 2
	return path[:half] + "..." + path[len(path)-half:]
}
