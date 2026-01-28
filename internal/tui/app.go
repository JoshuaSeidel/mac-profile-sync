package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/jseidel/mac-profile-sync/internal/config"
	"github.com/jseidel/mac-profile-sync/internal/discovery"
	"github.com/jseidel/mac-profile-sync/internal/sync"
)

// View represents the current TUI view
type View int

const (
	ViewDashboard View = iota
	ViewFolders
	ViewPeers
	ViewSettings
)

const numViews = 4

// App is the main TUI application model
type App struct {
	cfg       *config.Config
	discovery *discovery.Discovery
	engine    *sync.Engine

	// Views
	dashboard *DashboardModel
	folders   *FoldersModel
	peers     *PeersModel
	settings  *SettingsModel

	// State
	currentView View
	width       int
	height      int
	spinner     spinner.Model
	quitting    bool

	// Update channels
	peerUpdates     chan []*discovery.Peer
	activityUpdates chan []*sync.SyncActivity
	conflictUpdates chan []*sync.Conflict
}

// NewApp creates a new TUI application
func NewApp(cfg *config.Config, disc *discovery.Discovery, engine *sync.Engine) *App {
	s := spinner.New()
	s.Spinner = spinner.Dot

	app := &App{
		cfg:             cfg,
		discovery:       disc,
		engine:          engine,
		dashboard:       NewDashboardModel(cfg),
		folders:         NewFoldersModel(cfg),
		peers:           NewPeersModel(cfg, disc),
		settings:        NewSettingsModel(cfg),
		currentView:     ViewDashboard,
		spinner:         s,
		peerUpdates:     make(chan []*discovery.Peer, 10),
		activityUpdates: make(chan []*sync.SyncActivity, 10),
		conflictUpdates: make(chan []*sync.Conflict, 10),
	}

	return app
}

// Init initializes the application
func (a *App) Init() tea.Cmd {
	return tea.Batch(
		a.spinner.Tick,
		a.tickCmd(),
		a.listenForUpdates(),
	)
}

// Update handles messages
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = msg.Width
		a.height = msg.Height

		// Forward to all views
		a.dashboard.width = msg.Width
		a.dashboard.height = msg.Height
		a.folders.width = msg.Width
		a.folders.height = msg.Height
		a.peers.width = msg.Width
		a.peers.height = msg.Height
		a.settings.width = msg.Width
		a.settings.height = msg.Height

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			a.quitting = true
			return a, tea.Quit

		case "tab", "shift+tab":
			// Tab switches between views
			if msg.String() == "tab" {
				a.currentView = View((int(a.currentView) + 1) % numViews)
			} else {
				a.currentView = View((int(a.currentView) - 1 + numViews) % numViews)
			}
			a.refreshCurrentView()

		case "1":
			a.currentView = ViewDashboard
			a.refreshCurrentView()
		case "2":
			a.currentView = ViewFolders
			a.refreshCurrentView()
		case "3":
			a.currentView = ViewPeers
			a.refreshCurrentView()
		case "4":
			a.currentView = ViewSettings
			a.refreshCurrentView()

		default:
			// Forward to current view
			cmds = append(cmds, a.updateCurrentView(msg))
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		a.spinner, cmd = a.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tickMsg:
		// Periodic refresh
		a.refreshData()
		cmds = append(cmds, a.tickCmd())

	case peerUpdateMsg:
		a.dashboard.SetPeers(msg.peers)

	case activityUpdateMsg:
		a.dashboard.SetActivities(msg.activities)

	case conflictUpdateMsg:
		a.dashboard.SetConflicts(msg.conflicts)

	case SyncToggleMsg:
		// Start or stop sync engine
		if msg.Enabled {
			if a.engine != nil {
				a.engine.Start()
			}
		} else {
			if a.engine != nil {
				a.engine.Stop()
			}
		}
	}

	return a, tea.Batch(cmds...)
}

// View renders the application
func (a *App) View() string {
	if a.quitting {
		return "Goodbye!\n"
	}

	// Render tabs
	tabs := a.renderTabs()

	// Render current view
	var content string
	switch a.currentView {
	case ViewDashboard:
		content = a.dashboard.View()
	case ViewFolders:
		content = a.folders.View()
	case ViewPeers:
		content = a.peers.View()
	case ViewSettings:
		content = a.settings.View()
	}

	return fmt.Sprintf("%s\n%s", tabs, content)
}

func (a *App) renderTabs() string {
	tabs := []struct {
		label string
		key   string
		view  View
	}{
		{"Dashboard", "1", ViewDashboard},
		{"Folders", "2", ViewFolders},
		{"Peers", "3", ViewPeers},
		{"Settings", "4", ViewSettings},
	}

	var rendered []string
	for _, t := range tabs {
		rendered = append(rendered, TabWithKey(t.label, t.key, a.currentView == t.view))
	}

	return lipglossJoinHorizontal(rendered...) + "  " + mutedStyle.Render("Tab: switch  q: quit")
}

func (a *App) refreshCurrentView() {
	switch a.currentView {
	case ViewDashboard:
		a.dashboard.RefreshFolders()
	case ViewFolders:
		a.folders.Refresh()
	case ViewPeers:
		a.peers.Refresh()
	case ViewSettings:
		a.settings.Refresh()
	}
}

func (a *App) updateCurrentView(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	switch a.currentView {
	case ViewDashboard:
		a.dashboard, cmd = a.dashboard.Update(msg)
	case ViewFolders:
		a.folders, cmd = a.folders.Update(msg)
	case ViewPeers:
		a.peers, cmd = a.peers.Update(msg)
	case ViewSettings:
		a.settings, cmd = a.settings.Update(msg)
	}
	return cmd
}

func (a *App) refreshData() {
	// Update peers
	if a.discovery != nil {
		peers := a.discovery.GetPeers()
		a.dashboard.SetPeers(peers)
		a.peers.SetDiscoveredPeers(peers)
	}

	// Update activities
	if a.engine != nil {
		activities := a.engine.GetActivities(10)
		a.dashboard.SetActivities(activities)
	}
}

// Message types
type tickMsg time.Time
type peerUpdateMsg struct{ peers []*discovery.Peer }
type activityUpdateMsg struct{ activities []*sync.SyncActivity }
type conflictUpdateMsg struct{ conflicts []*sync.Conflict }

func (a *App) tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (a *App) listenForUpdates() tea.Cmd {
	return func() tea.Msg {
		select {
		case peers := <-a.peerUpdates:
			return peerUpdateMsg{peers}
		case activities := <-a.activityUpdates:
			return activityUpdateMsg{activities}
		case conflicts := <-a.conflictUpdates:
			return conflictUpdateMsg{conflicts}
		}
	}
}

// NotifyPeerUpdate sends a peer update notification
func (a *App) NotifyPeerUpdate(peers []*discovery.Peer) {
	select {
	case a.peerUpdates <- peers:
	default:
	}
}

// NotifyActivityUpdate sends an activity update notification
func (a *App) NotifyActivityUpdate(activities []*sync.SyncActivity) {
	select {
	case a.activityUpdates <- activities:
	default:
	}
}

// NotifyConflictUpdate sends a conflict update notification
func (a *App) NotifyConflictUpdate(conflicts []*sync.Conflict) {
	select {
	case a.conflictUpdates <- conflicts:
	default:
	}
}

// lipglossJoinHorizontal joins strings horizontally with a space
func lipglossJoinHorizontal(strs ...string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += " "
		}
		result += s
	}
	return result
}

// Run starts the TUI application
func Run(cfg *config.Config, disc *discovery.Discovery, engine *sync.Engine) error {
	app := NewApp(cfg, disc, engine)
	p := tea.NewProgram(app, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
