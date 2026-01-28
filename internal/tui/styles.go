package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Colors
var (
	primaryColor   = lipgloss.Color("#7C3AED") // Purple
	secondaryColor = lipgloss.Color("#10B981") // Green
	warningColor   = lipgloss.Color("#F59E0B") // Amber
	errorColor     = lipgloss.Color("#EF4444") // Red
	mutedColorVal  = lipgloss.Color("#6B7280") // Gray
	textColor      = lipgloss.Color("#F9FAFB") // White
)

// Styles
var (
	// Muted style for text
	mutedStyle = lipgloss.NewStyle().Foreground(mutedColorVal)

	// Title styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(primaryColor).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(mutedColorVal).
			Italic(true)

	// Box styles
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryColor).
			Padding(1, 2)

	innerBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(mutedColorVal).
			Padding(0, 1)

	// Status styles
	connectedStyle = lipgloss.NewStyle().
			Foreground(secondaryColor).
			Bold(true)

	disconnectedStyle = lipgloss.NewStyle().
				Foreground(errorColor).
				Bold(true)

	// List item styles
	selectedItemStyle = lipgloss.NewStyle().
				Foreground(primaryColor).
				Bold(true)

	normalItemStyle = lipgloss.NewStyle().
			Foreground(textColor)

	disabledItemStyle = lipgloss.NewStyle().
				Foreground(mutedColorVal)

	// Activity styles
	sentStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	receivedStyle = lipgloss.NewStyle().
			Foreground(primaryColor)

	deletedStyle = lipgloss.NewStyle().
			Foreground(errorColor)

	// Help styles
	helpKeyStyle = lipgloss.NewStyle().
			Foreground(primaryColor).
			Bold(true)

	helpDescStyle = lipgloss.NewStyle().
			Foreground(mutedColorVal)

	// Tab styles
	activeTabStyle = lipgloss.NewStyle().
			Foreground(textColor).
			Background(primaryColor).
			Padding(0, 2).
			Bold(true)

	inactiveTabStyle = lipgloss.NewStyle().
				Foreground(mutedColorVal).
				Padding(0, 2)

	// Input styles
	inputStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(primaryColor).
			Padding(0, 1)

	// Error/Warning styles
	errorStyle = lipgloss.NewStyle().
			Foreground(errorColor).
			Bold(true)

	warningStyle = lipgloss.NewStyle().
			Foreground(warningColor)

	successStyle = lipgloss.NewStyle().
			Foreground(secondaryColor)

	// Conflict styles
	conflictBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(warningColor).
				Padding(1, 2)
)

// StatusIndicator returns a styled status indicator
func StatusIndicator(connected bool) string {
	if connected {
		return connectedStyle.Render("● Connected")
	}
	return disconnectedStyle.Render("○ Disconnected")
}

// FolderStatusIndicator returns a styled folder status
func FolderStatusIndicator(enabled bool) string {
	if enabled {
		return connectedStyle.Render("✓")
	}
	return disabledItemStyle.Render("○")
}

// ActivityIcon returns an icon for activity type
func ActivityIcon(activityType string) string {
	switch activityType {
	case "sent":
		return sentStyle.Render("→")
	case "received":
		return receivedStyle.Render("←")
	case "deleted":
		return deletedStyle.Render("×")
	default:
		return "•"
	}
}

// HelpItem formats a help key-description pair
func HelpItem(key, desc string) string {
	return helpKeyStyle.Render("["+key+"]") + " " + helpDescStyle.Render(desc)
}

// Tab renders a tab item
func Tab(label string, active bool) string {
	if active {
		return activeTabStyle.Render(label)
	}
	return inactiveTabStyle.Render(label)
}

// TabWithKey renders a tab item with a keyboard shortcut indicator
func TabWithKey(label, key string, active bool) string {
	keyIndicator := helpKeyStyle.Render(key)
	if active {
		return keyIndicator + " " + activeTabStyle.Render(label)
	}
	return keyIndicator + " " + inactiveTabStyle.Render(label)
}
