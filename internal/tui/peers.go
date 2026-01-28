package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jseidel/mac-profile-sync/internal/config"
	"github.com/jseidel/mac-profile-sync/internal/discovery"
)

// PeersModel represents the peers management view
type PeersModel struct {
	cfg             *config.Config
	discovery       *discovery.Discovery
	discoveredPeers []*discovery.Peer
	manualPeers     []string
	selected        int
	width           int
	height          int
	addMode         bool
	input           textinput.Model
	err             string
	success         string
}

// NewPeersModel creates a new peers model
func NewPeersModel(cfg *config.Config, disc *discovery.Discovery) *PeersModel {
	ti := textinput.New()
	ti.Placeholder = "192.168.1.100:9876"
	ti.CharLimit = 256
	ti.Width = 40

	return &PeersModel{
		cfg:         cfg,
		discovery:   disc,
		manualPeers: cfg.Network.ManualPeers,
		input:       ti,
	}
}

// Init initializes the peers view
func (m *PeersModel) Init() tea.Cmd {
	return nil
}

// Update handles messages
func (m *PeersModel) Update(msg tea.Msg) (*PeersModel, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		m.err = ""
		m.success = ""

		if m.addMode {
			switch msg.String() {
			case "enter":
				addr := m.input.Value()
				if addr != "" {
					if err := m.addPeer(addr); err != nil {
						m.err = err.Error()
					} else {
						m.success = fmt.Sprintf("Added peer: %s", addr)
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
			totalItems := len(m.discoveredPeers) + len(m.manualPeers)
			if m.selected < totalItems-1 {
				m.selected++
			}
		case "a":
			m.addMode = true
			m.input.Focus()
			return m, textinput.Blink
		case "delete", "backspace", "x":
			m.removePeer()
		case "enter", " ":
			// Connect to selected peer
			m.connectToPeer()
		}
	}

	return m, nil
}

// View renders the peers view
func (m *PeersModel) View() string {
	var b strings.Builder

	// Title
	title := titleStyle.Render("Peer Management")
	b.WriteString(title)
	b.WriteString("\n\n")

	// Add peer input
	if m.addMode {
		b.WriteString("Add peer address (host:port):\n")
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

	// Discovered Peers section
	b.WriteString(m.renderDiscoveredPeers())
	b.WriteString("\n")

	// Manual Peers section
	b.WriteString(m.renderManualPeers())
	b.WriteString("\n\n")

	// Help bar
	b.WriteString(m.renderHelpBar())

	maxWidth := m.width - 4
	if maxWidth < 50 {
		maxWidth = 50
	}

	return boxStyle.Width(maxWidth).Render(b.String())
}

func (m *PeersModel) renderDiscoveredPeers() string {
	var b strings.Builder

	b.WriteString(connectedStyle.Render("Auto-Discovered Peers"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n")

	if len(m.discoveredPeers) == 0 {
		b.WriteString(subtitleStyle.Render("  Searching for peers on local network..."))
		b.WriteString("\n")
	} else {
		for i, peer := range m.discoveredPeers {
			cursor := "  "
			if i == m.selected {
				cursor = selectedItemStyle.Render("> ")
			}

			status := connectedStyle.Render("●")
			line := fmt.Sprintf("%s%s %s (%s)", cursor, status, peer.Name, peer.Address())

			if i == m.selected {
				line = selectedItemStyle.Render(line)
			}

			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return b.String()
}

func (m *PeersModel) renderManualPeers() string {
	var b strings.Builder

	b.WriteString(mutedStyle.Render("Manual Peers"))
	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", 50))
	b.WriteString("\n")

	if len(m.manualPeers) == 0 {
		b.WriteString(subtitleStyle.Render("  No manual peers configured"))
		b.WriteString("\n")
		b.WriteString(subtitleStyle.Render("  Press [a] to add a peer"))
		b.WriteString("\n")
	} else {
		offset := len(m.discoveredPeers)
		for i, addr := range m.manualPeers {
			idx := offset + i
			cursor := "  "
			if idx == m.selected {
				cursor = selectedItemStyle.Render("> ")
			}

			line := fmt.Sprintf("%s○ %s", cursor, addr)

			if idx == m.selected {
				line = selectedItemStyle.Render(line)
			}

			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	return innerBoxStyle.Render(b.String())
}

func (m *PeersModel) renderHelpBar() string {
	items := []string{
		HelpItem("a", "dd peer"),
		HelpItem("x", " remove"),
		HelpItem("enter", "connect"),
		HelpItem("↑↓", "navigate"),
	}
	return strings.Join(items, " ")
}

func (m *PeersModel) addPeer(addr string) error {
	// Validate format (basic check)
	if !strings.Contains(addr, ":") {
		return fmt.Errorf("invalid format, use host:port (e.g., 192.168.1.100:9876)")
	}

	// Add to config
	m.cfg.Network.ManualPeers = append(m.cfg.Network.ManualPeers, addr)
	m.manualPeers = m.cfg.Network.ManualPeers

	// Save config
	if err := config.Save(m.cfg); err != nil {
		return err
	}

	// Add to discovery if available
	if m.discovery != nil {
		m.discovery.AddManualPeer(addr)
	}

	return nil
}

func (m *PeersModel) removePeer() {
	// Can only remove manual peers
	offset := len(m.discoveredPeers)
	manualIdx := m.selected - offset

	if manualIdx < 0 || manualIdx >= len(m.manualPeers) {
		m.err = "Can only remove manual peers"
		return
	}

	addr := m.manualPeers[manualIdx]

	// Remove from config
	newPeers := make([]string, 0, len(m.manualPeers)-1)
	for i, p := range m.manualPeers {
		if i != manualIdx {
			newPeers = append(newPeers, p)
		}
	}
	m.cfg.Network.ManualPeers = newPeers
	m.manualPeers = newPeers

	// Save config
	if err := config.Save(m.cfg); err != nil {
		m.err = err.Error()
		return
	}

	// Remove from discovery
	if m.discovery != nil {
		m.discovery.RemovePeer(fmt.Sprintf("manual-%s", addr))
	}

	m.success = fmt.Sprintf("Removed peer: %s", addr)

	// Adjust selection
	if m.selected >= len(m.discoveredPeers)+len(m.manualPeers) && m.selected > 0 {
		m.selected--
	}
}

func (m *PeersModel) connectToPeer() {
	// For discovered peers, they should already be connecting automatically
	// For manual peers, trigger a connection attempt
	if m.selected < len(m.discoveredPeers) {
		peer := m.discoveredPeers[m.selected]
		m.success = fmt.Sprintf("Connecting to %s...", peer.Name)
	} else {
		offset := len(m.discoveredPeers)
		manualIdx := m.selected - offset
		if manualIdx >= 0 && manualIdx < len(m.manualPeers) {
			addr := m.manualPeers[manualIdx]
			m.success = fmt.Sprintf("Connecting to %s...", addr)
		}
	}
}

// SetDiscoveredPeers updates the list of discovered peers
func (m *PeersModel) SetDiscoveredPeers(peers []*discovery.Peer) {
	m.discoveredPeers = peers
}

// Refresh reloads peer data
func (m *PeersModel) Refresh() {
	m.manualPeers = m.cfg.Network.ManualPeers
}
