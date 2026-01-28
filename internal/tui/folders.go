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

type itemType int

const (
	itemSyncFolder itemType = iota
	itemExcludeDir
)

// FoldersModel represents the folder management view
type FoldersModel struct {
	cfg          *config.Config
	items        []folderItem
	selected     int
	width        int
	height       int
	addMode      bool
	addType      itemType // What type we're adding
	input        textinput.Model
	err          string
	success      string
}

type folderItem struct {
	path      string
	enabled   bool
	fileCount int
	itemType  itemType
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
					if m.addType == itemSyncFolder {
						if err := m.cfg.AddFolder(path); err != nil {
							m.err = err.Error()
						} else {
							m.success = fmt.Sprintf("Added sync folder: %s", path)
							m.refreshFolders()
						}
					} else {
						if err := m.addExcludeDir(path); err != nil {
							m.err = err.Error()
						} else {
							m.success = fmt.Sprintf("Added excluded directory: %s", path)
							m.refreshFolders()
						}
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
			if m.selected < len(m.items)-1 {
				m.selected++
			}
		case "a":
			// Add sync folder
			m.addMode = true
			m.addType = itemSyncFolder
			m.input.Placeholder = "~/path/to/sync"
			m.input.Focus()
			return m, textinput.Blink
		case "e":
			// Add exclude directory
			m.addMode = true
			m.addType = itemExcludeDir
			m.input.Placeholder = "~/path/to/exclude"
			m.input.Focus()
			return m, textinput.Blink
		case "enter", " ":
			if len(m.items) > 0 && m.selected < len(m.items) {
				item := m.items[m.selected]
				if item.itemType == itemSyncFolder {
					if err := m.cfg.ToggleFolder(item.path); err != nil {
						m.err = err.Error()
					} else {
						m.refreshFolders()
					}
				}
				// Exclude dirs can't be toggled
			}
		case "delete", "backspace", "x":
			if len(m.items) > 0 && m.selected < len(m.items) {
				item := m.items[m.selected]
				if item.itemType == itemSyncFolder {
					if err := m.cfg.RemoveFolder(item.path); err != nil {
						m.err = err.Error()
					} else {
						m.success = fmt.Sprintf("Removed sync folder: %s", item.path)
						m.refreshFolders()
					}
				} else {
					if err := m.removeExcludeDir(item.path); err != nil {
						m.err = err.Error()
					} else {
						m.success = fmt.Sprintf("Removed excluded directory: %s", item.path)
						m.refreshFolders()
					}
				}
				if m.selected >= len(m.items) && m.selected > 0 {
					m.selected--
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
		if m.addType == itemSyncFolder {
			b.WriteString("Add folder to sync:\n")
		} else {
			b.WriteString("Add directory to exclude:\n")
		}
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

	// Count items by type
	syncCount := 0
	excludeCount := 0
	for _, item := range m.items {
		if item.itemType == itemSyncFolder {
			syncCount++
		} else {
			excludeCount++
		}
	}

	// Synced Folders section
	b.WriteString(connectedStyle.Render("Synced Folders"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n")

	if syncCount == 0 {
		b.WriteString(subtitleStyle.Render("  No folders configured - press [a] to add"))
		b.WriteString("\n")
	} else {
		for i, item := range m.items {
			if item.itemType != itemSyncFolder {
				continue
			}

			icon := FolderStatusIndicator(item.enabled)
			shortPath := shortenPath(item.path, 35)

			var status string
			if item.enabled {
				status = connectedStyle.Render("syncing")
			} else {
				status = disabledItemStyle.Render("paused")
			}

			fileCount := fmt.Sprintf("%d files", item.fileCount)

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
	}

	b.WriteString("\n")

	// Excluded Directories section
	b.WriteString(errorStyle.Render("Excluded Directories"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 60))
	b.WriteString("\n")

	if excludeCount == 0 {
		b.WriteString(subtitleStyle.Render("  No exclusions - press [e] to exclude a directory"))
		b.WriteString("\n")
	} else {
		for i, item := range m.items {
			if item.itemType != itemExcludeDir {
				continue
			}

			shortPath := shortenPath(item.path, 45)

			cursor := "  "
			if i == m.selected {
				cursor = selectedItemStyle.Render("> ")
			}

			line := fmt.Sprintf("%s✗ %-45s %s",
				cursor, shortPath, disabledItemStyle.Render("excluded"))

			if i == m.selected {
				line = lipgloss.NewStyle().Bold(true).Render(line)
			}

			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return innerBoxStyle.Render(b.String())
}

func (m *FoldersModel) renderHelpBar() string {
	items := []string{
		HelpItem("a", "dd sync"),
		HelpItem("e", "xclude"),
		HelpItem("enter", "toggle"),
		HelpItem("x", "remove"),
		HelpItem("↑↓", "navigate"),
	}
	return strings.Join(items, " ")
}

func (m *FoldersModel) refreshFolders() {
	m.items = make([]folderItem, 0)

	// Add sync folders
	for _, f := range m.cfg.Folders {
		count, _ := fileutil.CountFilesRecursive(f.Path)
		m.items = append(m.items, folderItem{
			path:      f.Path,
			enabled:   f.Enabled,
			fileCount: count,
			itemType:  itemSyncFolder,
		})
	}

	// Add excluded directories
	for _, dir := range m.cfg.Sync.ExcludeDirs {
		m.items = append(m.items, folderItem{
			path:     dir,
			enabled:  false,
			itemType: itemExcludeDir,
		})
	}
}

func (m *FoldersModel) addExcludeDir(path string) error {
	// Check if already exists
	for _, dir := range m.cfg.Sync.ExcludeDirs {
		if dir == path {
			return fmt.Errorf("directory already excluded: %s", path)
		}
	}

	m.cfg.Sync.ExcludeDirs = append(m.cfg.Sync.ExcludeDirs, path)
	return config.Save(m.cfg)
}

func (m *FoldersModel) removeExcludeDir(path string) error {
	newDirs := make([]string, 0)
	found := false
	for _, dir := range m.cfg.Sync.ExcludeDirs {
		if dir == path {
			found = true
			continue
		}
		newDirs = append(newDirs, dir)
	}

	if !found {
		return fmt.Errorf("directory not in exclusion list: %s", path)
	}

	m.cfg.Sync.ExcludeDirs = newDirs
	return config.Save(m.cfg)
}

// Refresh reloads folder data
func (m *FoldersModel) Refresh() {
	m.refreshFolders()
}
