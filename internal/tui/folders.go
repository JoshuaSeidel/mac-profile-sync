package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/jseidel/mac-profile-sync/internal/config"
	"github.com/jseidel/mac-profile-sync/pkg/fileutil"
)

// FoldersModel represents the folder management view
type FoldersModel struct {
	cfg       *config.Config
	folders   []folderItem
	selected  int
	width     int
	height    int
	addMode   bool
	input     textinput.Model
	err       string
	success   string
}

type folderItem struct {
	path      string
	enabled   bool
	fileCount int
	size      string
}

// NewFoldersModel creates a new folders model
func NewFoldersModel(cfg *config.Config) *FoldersModel {
	ti := textinput.New()
	ti.Placeholder = "~/path/to/folder"
	ti.CharLimit = 256
	ti.Width = 40

	m := &FoldersModel{
		cfg:   cfg,
		input: ti,
	}
	m.refreshFolders()
	return m
}

// Init initializes the folders view
func (m *FoldersModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m *FoldersModel) Update(msg tea.Msg) (*FoldersModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		// Clear messages on any key
		m.err = ""
		m.success = ""

		if m.addMode {
			switch msg.String() {
			case "enter":
				path := m.input.Value()
				if path != "" {
					if err := m.cfg.AddFolder(path); err != nil {
						m.err = err.Error()
					} else {
						m.success = fmt.Sprintf("Added folder: %s", path)
						m.refreshFolders()
					}
				}
				m.addMode = false
				m.input.SetValue("")
				return m, nil

			case "esc":
				m.addMode = false
				m.input.SetValue("")
				return m, nil
			}

			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}

		switch msg.String() {
		case "up", "k":
			if m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.selected < len(m.folders)-1 {
				m.selected++
			}
		case "a":
			m.addMode = true
			m.input.Focus()
			return m, textinput.Blink
		case "enter", " ":
			if len(m.folders) > 0 {
				folder := m.folders[m.selected]
				if err := m.cfg.ToggleFolder(folder.path); err != nil {
					m.err = err.Error()
				} else {
					m.refreshFolders()
				}
			}
		case "delete", "backspace", "x":
			if len(m.folders) > 0 {
				folder := m.folders[m.selected]
				if err := m.cfg.RemoveFolder(folder.path); err != nil {
					m.err = err.Error()
				} else {
					m.success = fmt.Sprintf("Removed folder: %s", folder.path)
					m.refreshFolders()
					if m.selected >= len(m.folders) && m.selected > 0 {
						m.selected--
					}
				}
			}
		}
	}

	return m, nil
}

// View renders the folders view
func (m *FoldersModel) View() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render("Folder Management")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Add folder input
	if m.addMode {
		b.WriteString("Add new folder:\n")
		b.WriteString(inputStyle.Render(m.input.View()))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("Press Enter to add, Esc to cancel"))
		b.WriteString("\n\n")
	}

	// Error/Success messages
	if m.err != "" {
		b.WriteString(errorStyle.Render("Error: " + m.err))
		b.WriteString("\n\n")
	}
	if m.success != "" {
		b.WriteString(successStyle.Render(m.success))
		b.WriteString("\n\n")
	}

	// Folders list
	b.WriteString(m.renderFoldersList())
	b.WriteString("\n\n")

	// Help bar
	b.WriteString(m.renderHelpBar())

	// Wrap in main box
	maxWidth := m.width - 4
	if maxWidth < 50 {
		maxWidth = 50
	}

	return boxStyle.Width(maxWidth).Render(b.String())
}

func (m *FoldersModel) renderFoldersList() string {
	var b strings.Builder

	if len(m.folders) == 0 {
		b.WriteString(subtitleStyle.Render("No folders configured"))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("Press [a] to add a folder"))
		return b.String()
	}

	// Header
	header := fmt.Sprintf("%-3s %-35s %-12s %s", "", "Path", "Files", "Status")
	b.WriteString(mutedStyle.Render(header))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n")

	for i, folder := range m.folders {
		icon := FolderStatusIndicator(folder.enabled)

		// Shorten path
		shortPath := shortenPath(folder.path, 35)

		var status string
		if folder.enabled {
			status = connectedStyle.Render("enabled")
		} else {
			status = disabledItemStyle.Render("disabled")
		}

		fileCount := fmt.Sprintf("%d", folder.fileCount)

		// Build line
		cursor := "  "
		if i == m.selected {
			cursor = selectedItemStyle.Render("> ")
		}

		line := fmt.Sprintf("%s%s %-35s %-12s %s",
			cursor, icon, shortPath, fileCount, status)

		if i == m.selected {
			line = lipgloss.NewStyle().Bold(true).Render(line)
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	return innerBoxStyle.Render(b.String())
}

func (m *FoldersModel) renderHelpBar() string {
	items := []string{
		HelpItem("a", "dd"),
		HelpItem("enter", "toggle"),
		HelpItem("x", "remove"),
		HelpItem("↑↓", "navigate"),
	}
	return strings.Join(items, " ")
}

func (m *FoldersModel) refreshFolders() {
	m.folders = make([]folderItem, len(m.cfg.Folders))
	for i, f := range m.cfg.Folders {
		count, _ := fileutil.CountFilesRecursive(f.Path)
		m.folders[i] = folderItem{
			path:      f.Path,
			enabled:   f.Enabled,
			fileCount: count,
		}
	}
}

// Refresh reloads folder data
func (m *FoldersModel) Refresh() {
	m.refreshFolders()
}
