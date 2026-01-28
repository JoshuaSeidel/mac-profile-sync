package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jseidel/mac-profile-sync/internal/config"
)

// SettingsModel represents the settings view
type SettingsModel struct {
	cfg       *config.Config
	settings  []settingItem
	selected  int
	width     int
	height    int
	editMode  bool
	input     textinput.Model
	err       string
	success   string
}

type settingItem struct {
	key         string
	label       string
	value       string
	editable    bool
	options     []string // For selection-based settings
	optionIndex int
}

// NewSettingsModel creates a new settings model
func NewSettingsModel(cfg *config.Config) *SettingsModel {
	ti := textinput.New()
	ti.CharLimit = 256
	ti.Width = 40

	m := &SettingsModel{
		cfg:   cfg,
		input: ti,
	}
	m.refreshSettings()
	return m
}

// Init initializes the settings view
func (m *SettingsModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m *SettingsModel) Update(msg tea.Msg) (*SettingsModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		m.err = ""
		m.success = ""

		if m.editMode {
			switch msg.String() {
			case "enter":
				m.applyEdit()
				m.editMode = false
				return m, nil
			case "esc":
				m.editMode = false
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
			if m.selected < len(m.settings)-1 {
				m.selected++
			}
		case "enter", " ":
			setting := m.settings[m.selected]
			if setting.editable {
				if len(setting.options) > 0 {
					// Cycle through options
					setting.optionIndex = (setting.optionIndex + 1) % len(setting.options)
					m.settings[m.selected].optionIndex = setting.optionIndex
					m.settings[m.selected].value = setting.options[setting.optionIndex]
					m.applySettingChange(setting.key, setting.options[setting.optionIndex])
				} else {
					// Text edit mode
					m.editMode = true
					m.input.SetValue(setting.value)
					m.input.Focus()
					return m, textinput.Blink
				}
			}
		case "left", "h":
			setting := m.settings[m.selected]
			if setting.editable && len(setting.options) > 0 {
				setting.optionIndex--
				if setting.optionIndex < 0 {
					setting.optionIndex = len(setting.options) - 1
				}
				m.settings[m.selected].optionIndex = setting.optionIndex
				m.settings[m.selected].value = setting.options[setting.optionIndex]
				m.applySettingChange(setting.key, setting.options[setting.optionIndex])
			}
		case "right", "l":
			setting := m.settings[m.selected]
			if setting.editable && len(setting.options) > 0 {
				setting.optionIndex = (setting.optionIndex + 1) % len(setting.options)
				m.settings[m.selected].optionIndex = setting.optionIndex
				m.settings[m.selected].value = setting.options[setting.optionIndex]
				m.applySettingChange(setting.key, setting.options[setting.optionIndex])
			}
		}
	}

	return m, nil
}

// View renders the settings view
func (m *SettingsModel) View() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render("Settings")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Edit mode input
	if m.editMode {
		setting := m.settings[m.selected]
		b.WriteString(fmt.Sprintf("Edit %s:\n", setting.label))
		b.WriteString(inputStyle.Render(m.input.View()))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("Press Enter to save, Esc to cancel"))
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

	// Settings list
	b.WriteString(m.renderSettingsList())
	b.WriteString("\n\n")

	// Help bar
	b.WriteString(m.renderHelpBar())

	maxWidth := m.width - 4
	if maxWidth < 50 {
		maxWidth = 50
	}

	return boxStyle.Width(maxWidth).Render(b.String())
}

func (m *SettingsModel) renderSettingsList() string {
	var b strings.Builder

	// Group settings by category
	categories := []struct {
		name     string
		settings []int
	}{
		{"Device", []int{0}},
		{"Sync", []int{1}},
		{"Network", []int{2, 3}},
		{"Security", []int{4, 5}},
	}

	for _, cat := range categories {
		b.WriteString(mutedStyle.Render(cat.name))
		b.WriteString("\n")
		b.WriteString(strings.Repeat("─", 50))
		b.WriteString("\n")

		for _, idx := range cat.settings {
			if idx >= len(m.settings) {
				continue
			}
			setting := m.settings[idx]

			cursor := "  "
			if idx == m.selected {
				cursor = selectedItemStyle.Render("> ")
			}

			label := setting.label
			value := setting.value

			if len(setting.options) > 0 {
				// Show options indicator
				value = fmt.Sprintf("< %s >", value)
			}

			line := fmt.Sprintf("%s%-25s %s", cursor, label, value)

			if idx == m.selected {
				line = selectedItemStyle.Render(line)
			} else if !setting.editable {
				line = disabledItemStyle.Render(line)
			}

			b.WriteString(line)
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	return innerBoxStyle.Render(b.String())
}

func (m *SettingsModel) renderHelpBar() string {
	if m.editMode {
		return HelpItem("enter", "save") + " " + HelpItem("esc", "cancel")
	}

	items := []string{
		HelpItem("enter", "edit"),
		HelpItem("←→", "change"),
		HelpItem("↑↓", "navigate"),
	}
	return strings.Join(items, " ")
}

func (m *SettingsModel) refreshSettings() {
	conflictOptions := []string{"newest_wins", "keep_both", "prompt"}
	conflictIndex := 0
	for i, opt := range conflictOptions {
		if opt == m.cfg.Sync.ConflictResolution {
			conflictIndex = i
			break
		}
	}

	m.settings = []settingItem{
		{
			key:      "device.name",
			label:    "Device Name",
			value:    m.cfg.Device.Name,
			editable: true,
		},
		{
			key:         "sync.conflict_resolution",
			label:       "Conflict Resolution",
			value:       m.cfg.Sync.ConflictResolution,
			editable:    true,
			options:     conflictOptions,
			optionIndex: conflictIndex,
		},
		{
			key:      "network.port",
			label:    "Network Port",
			value:    fmt.Sprintf("%d", m.cfg.Network.Port),
			editable: true,
		},
		{
			key:         "network.use_discovery",
			label:       "Auto Discovery",
			value:       boolToString(m.cfg.Network.UseDiscovery),
			editable:    true,
			options:     []string{"enabled", "disabled"},
			optionIndex: boolToIndex(m.cfg.Network.UseDiscovery),
		},
		{
			key:         "security.require_pairing",
			label:       "Require Pairing",
			value:       boolToString(m.cfg.Security.RequirePairing),
			editable:    true,
			options:     []string{"enabled", "disabled"},
			optionIndex: boolToIndex(m.cfg.Security.RequirePairing),
		},
		{
			key:         "security.encryption",
			label:       "Encryption",
			value:       boolToString(m.cfg.Security.Encryption),
			editable:    true,
			options:     []string{"enabled", "disabled"},
			optionIndex: boolToIndex(m.cfg.Security.Encryption),
		},
	}
}

func (m *SettingsModel) applyEdit() {
	setting := m.settings[m.selected]
	value := m.input.Value()

	m.applySettingChange(setting.key, value)
	m.settings[m.selected].value = value
}

func (m *SettingsModel) applySettingChange(key, value string) {
	switch key {
	case "device.name":
		m.cfg.Device.Name = value
	case "sync.conflict_resolution":
		m.cfg.Sync.ConflictResolution = value
	case "network.port":
		var port int
		fmt.Sscanf(value, "%d", &port)
		if port > 0 && port < 65536 {
			m.cfg.Network.Port = port
		}
	case "network.use_discovery":
		m.cfg.Network.UseDiscovery = (value == "enabled")
	case "security.require_pairing":
		m.cfg.Security.RequirePairing = (value == "enabled")
	case "security.encryption":
		m.cfg.Security.Encryption = (value == "enabled")
	}

	if err := config.Save(m.cfg); err != nil {
		m.err = err.Error()
	} else {
		m.success = "Settings saved"
	}
}

func boolToString(b bool) string {
	if b {
		return "enabled"
	}
	return "disabled"
}

func boolToIndex(b bool) int {
	if b {
		return 0
	}
	return 1
}

// Refresh reloads settings
func (m *SettingsModel) Refresh() {
	m.refreshSettings()
}
