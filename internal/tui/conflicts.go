package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/jseidel/mac-profile-sync/internal/sync"
	"github.com/jseidel/mac-profile-sync/pkg/fileutil"
)

// ConflictsModel represents the conflict resolution view
type ConflictsModel struct {
	conflicts []*sync.Conflict
	selected  int
	width     int
	height    int

	// Callback for resolving conflicts
	onResolve func(conflictID string, resolution sync.ConflictResolution) error
}

// NewConflictsModel creates a new conflicts model
func NewConflictsModel() *ConflictsModel {
	return &ConflictsModel{
		conflicts: make([]*sync.Conflict, 0),
	}
}

// Init initializes the conflicts view
func (m *ConflictsModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m *ConflictsModel) Update(msg tea.Msg) (*ConflictsModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if len(m.conflicts) == 0 {
			return m, nil
		}

		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.conflicts)-1 {
				m.selected++
			}
		case "l":
			// Keep local
			m.resolveSelected(sync.ResolutionKeepLocal)
		case "r":
			// Keep remote
			m.resolveSelected(sync.ResolutionKeepRemote)
		case "b":
			// Keep both
			m.resolveSelected(sync.ResolutionKeepBoth)
		case "s":
			// Skip
			m.resolveSelected(sync.ResolutionSkip)
		}
	}

	return m, nil
}

// View renders the conflicts view
func (m *ConflictsModel) View() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render("Conflict Resolution")
	b.WriteString(title)
	b.WriteString("\n\n")

	if len(m.conflicts) == 0 {
		b.WriteString(successStyle.Render("✓ No conflicts"))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("All files are in sync"))
		b.WriteString("\n\n")
		b.WriteString(m.renderHelpBar())

		maxWidth := m.width - 4
		if maxWidth < 50 {
			maxWidth = 50
		}
		return boxStyle.Width(maxWidth).Render(b.String())
	}

	// Conflict count
	countMsg := fmt.Sprintf("⚠ %d conflict(s) require attention", len(m.conflicts))
	b.WriteString(warningStyle.Render(countMsg))
	b.WriteString("\n\n")

	// Show selected conflict details
	if m.selected < len(m.conflicts) {
		conflict := m.conflicts[m.selected]
		b.WriteString(m.renderConflictDetail(conflict))
		b.WriteString("\n\n")
	}

	// Conflict list
	b.WriteString(m.renderConflictList())
	b.WriteString("\n\n")

	// Help bar
	b.WriteString(m.renderHelpBar())

	maxWidth := m.width - 4
	if maxWidth < 50 {
		maxWidth = 50
	}

	return boxStyle.Width(maxWidth).Render(b.String())
}

func (m *ConflictsModel) renderConflictDetail(conflict *sync.Conflict) string {
	var b strings.Builder

	// File path
	b.WriteString(warningStyle.Render("File: "))
	b.WriteString(conflict.RelPath)
	b.WriteString("\n\n")

	// Local version
	b.WriteString(normalItemStyle.Render("Local version:"))
	b.WriteString("\n")
	if conflict.LocalFile != nil {
		b.WriteString(fmt.Sprintf("  Modified: %s\n", conflict.LocalFile.ModTime.Format("Jan 2, 2006 3:04 PM")))
		b.WriteString(fmt.Sprintf("  Size: %s\n", fileutil.FormatSize(conflict.LocalFile.Size)))
	}
	b.WriteString("\n")

	// Remote version
	b.WriteString(normalItemStyle.Render("Remote version"))
	if conflict.RemoteFile != nil && conflict.RemoteFile.DeviceName != "" {
		b.WriteString(fmt.Sprintf(" (%s)", conflict.RemoteFile.DeviceName))
	}
	b.WriteString(":\n")
	if conflict.RemoteFile != nil {
		b.WriteString(fmt.Sprintf("  Modified: %s\n", conflict.RemoteFile.ModTime.Format("Jan 2, 2006 3:04 PM")))
		b.WriteString(fmt.Sprintf("  Size: %s\n", fileutil.FormatSize(conflict.RemoteFile.Size)))
	}

	return conflictBoxStyle.Render(b.String())
}

func (m *ConflictsModel) renderConflictList() string {
	var b strings.Builder

	b.WriteString("Conflicts:\n")
	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n")

	for i, conflict := range m.conflicts {
		cursor := "  "
		if i == m.selected {
			cursor = selectedItemStyle.Render("> ")
		}

		fileName := conflict.RelPath
		if len(fileName) > 40 {
			fileName = "..." + fileName[len(fileName)-37:]
		}

		line := fmt.Sprintf("%s%s", cursor, fileName)
		if i == m.selected {
			line = selectedItemStyle.Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return innerBoxStyle.Render(b.String())
}

func (m *ConflictsModel) renderHelpBar() string {
	if len(m.conflicts) == 0 {
		return HelpItem("←", "back")
	}

	items := []string{
		HelpItem("l", "keep local"),
		HelpItem("r", "keep remote"),
		HelpItem("b", "keep both"),
		HelpItem("s", "skip"),
		HelpItem("↑↓", "navigate"),
	}
	return strings.Join(items, " ")
}

func (m *ConflictsModel) resolveSelected(resolution sync.ConflictResolution) {
	if m.selected >= len(m.conflicts) {
		return
	}

	conflict := m.conflicts[m.selected]

	if m.onResolve != nil {
		if err := m.onResolve(conflict.ID, resolution); err != nil {
			return
		}
	}

	// Remove from list
	m.conflicts = append(m.conflicts[:m.selected], m.conflicts[m.selected+1:]...)

	// Adjust selection
	if m.selected >= len(m.conflicts) && m.selected > 0 {
		m.selected--
	}
}

// SetConflicts updates the conflict list
func (m *ConflictsModel) SetConflicts(conflicts []*sync.Conflict) {
	m.conflicts = conflicts
	if m.selected >= len(conflicts) {
		m.selected = 0
	}
}

// SetResolveCallback sets the resolve callback
func (m *ConflictsModel) SetResolveCallback(fn func(string, sync.ConflictResolution) error) {
	m.onResolve = fn
}

// HasConflicts returns whether there are conflicts
func (m *ConflictsModel) HasConflicts() bool {
	return len(m.conflicts) > 0
}
