package discovery

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"
	"github.com/rs/zerolog/log"
)

const (
	serviceType   = "_mac-profile-sync._tcp"
	serviceDomain = "local."
)

// Peer represents a discovered peer
type Peer struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Host     string    `json:"host"`
	Port     int       `json:"port"`
	Addrs    []net.IP  `json:"addrs"`
	LastSeen time.Time `json:"last_seen"`
	Manual   bool      `json:"manual"`
}

// Address returns the best address to connect to
func (p *Peer) Address() string {
	if len(p.Addrs) > 0 {
		// Prefer IPv4
		for _, addr := range p.Addrs {
			if addr.To4() != nil {
				return fmt.Sprintf("%s:%d", addr, p.Port)
			}
		}
		return fmt.Sprintf("%s:%d", p.Addrs[0], p.Port)
	}
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}

// Discovery manages peer discovery via mDNS and manual configuration
type Discovery struct {
	deviceName   string
	port         int
	useDiscovery bool
	manualPeers  []string

	server   *zeroconf.Server
	peers    map[string]*Peer
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
	stopping bool
	stopMu   sync.RWMutex

	// Callbacks
	onPeerFound func(*Peer)
	onPeerLost  func(*Peer)
}

// NewDiscovery creates a new discovery service
func NewDiscovery(deviceName string, port int, useDiscovery bool, manualPeers []string) *Discovery {
	ctx, cancel := context.WithCancel(context.Background())
	return &Discovery{
		deviceName:   deviceName,
		port:         port,
		useDiscovery: useDiscovery,
		manualPeers:  manualPeers,
		peers:        make(map[string]*Peer),
		ctx:          ctx,
		cancel:       cancel,
	}
}

// SetCallbacks sets the callbacks for peer events
func (d *Discovery) SetCallbacks(onFound, onLost func(*Peer)) {
	d.onPeerFound = onFound
	d.onPeerLost = onLost
}

// Start begins the discovery service
func (d *Discovery) Start() error {
	// Register ourselves via mDNS if enabled
	if d.useDiscovery {
		if err := d.registerService(); err != nil {
			return fmt.Errorf("failed to register mDNS service: %w", err)
		}

		// Start browsing for peers
		go d.browse()
	}

	// Add manual peers
	for _, addr := range d.manualPeers {
		d.addManualPeer(addr)
	}

	// Start peer health check
	go d.healthCheck()

	return nil
}

// Stop stops the discovery service
func (d *Discovery) Stop() {
	d.stopMu.Lock()
	d.stopping = true
	d.stopMu.Unlock()

	d.cancel()

	// Give browse goroutines time to exit gracefully
	time.Sleep(100 * time.Millisecond)

	if d.server != nil {
		d.server.Shutdown()
	}
}

func (d *Discovery) isStopping() bool {
	d.stopMu.RLock()
	defer d.stopMu.RUnlock()
	return d.stopping
}

// GetPeers returns all known peers
func (d *Discovery) GetPeers() []*Peer {
	d.mu.RLock()
	defer d.mu.RUnlock()

	peers := make([]*Peer, 0, len(d.peers))
	for _, p := range d.peers {
		peers = append(peers, p)
	}
	return peers
}

// GetPeer returns a specific peer by ID
func (d *Discovery) GetPeer(id string) *Peer {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.peers[id]
}

func (d *Discovery) registerService() error {
	var err error
	d.server, err = zeroconf.Register(
		d.deviceName,        // Instance name
		serviceType,         // Service type
		serviceDomain,       // Domain
		d.port,              // Port
		[]string{"version=1"}, // TXT records
		nil,                 // Interfaces (nil = all)
	)
	if err != nil {
		return err
	}

	log.Info().
		Str("name", d.deviceName).
		Int("port", d.port).
		Msg("mDNS service registered")

	return nil
}

func (d *Discovery) browse() {
	// Browse continuously with a new resolver and channel each time
	for {
		if d.isStopping() {
			return
		}

		select {
		case <-d.ctx.Done():
			return
		default:
		}

		// Run a single browse cycle with panic recovery
		d.doBrowseCycle()

		// Check if we should stop
		if d.isStopping() {
			return
		}

		select {
		case <-d.ctx.Done():
			return
		default:
			time.Sleep(10 * time.Second)
		}
	}
}

func (d *Discovery) doBrowseCycle() {
	// Recover from panics in zeroconf (known issue with channel closing)
	defer func() {
		if r := recover(); r != nil {
			if !d.isStopping() {
				log.Debug().Interface("panic", r).Msg("Recovered from mDNS browse panic")
			}
		}
	}()

	// Create a new resolver for each browse cycle
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create mDNS resolver")
		return
	}

	// Create a new channel for each browse (zeroconf closes the channel when done)
	entries := make(chan *zeroconf.ServiceEntry, 10)

	// Create a context with timeout for this browse cycle
	browseCtx, browseCancel := context.WithTimeout(d.ctx, 5*time.Second)
	defer browseCancel()

	// Start Browse in background with panic recovery
	done := make(chan struct{})
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if !d.isStopping() {
					log.Debug().Interface("panic", r).Msg("Recovered from mDNS browse goroutine panic")
				}
			}
			close(done)
		}()
		resolver.Browse(browseCtx, serviceType, serviceDomain, entries)
	}()

	// Process entries until context done or browse completes
	for {
		select {
		case entry, ok := <-entries:
			if !ok {
				// Channel closed by zeroconf
				return
			}
			if entry == nil {
				continue
			}
			// Don't add ourselves
			if entry.Instance == d.deviceName {
				continue
			}
			d.handleServiceEntry(entry)
		case <-browseCtx.Done():
			return
		case <-d.ctx.Done():
			return
		case <-done:
			return
		}
	}
}

func (d *Discovery) handleServiceEntry(entry *zeroconf.ServiceEntry) {
	peer := &Peer{
		ID:       entry.Instance,
		Name:     entry.Instance,
		Host:     entry.HostName,
		Port:     entry.Port,
		Addrs:    append(entry.AddrIPv4, entry.AddrIPv6...),
		LastSeen: time.Now(),
		Manual:   false,
	}

	d.mu.Lock()
	existing, exists := d.peers[peer.ID]
	d.peers[peer.ID] = peer
	d.mu.Unlock()

	if !exists {
		log.Info().
			Str("peer", peer.Name).
			Str("addr", peer.Address()).
			Msg("Discovered new peer")

		if d.onPeerFound != nil {
			d.onPeerFound(peer)
		}
	} else {
		existing.LastSeen = time.Now()
	}
}

func (d *Discovery) addManualPeer(addr string) {
	host, portStr, err := net.SplitHostPort(addr)
	if err != nil {
		log.Error().Err(err).Str("addr", addr).Msg("Invalid manual peer address")
		return
	}

	var port int
	fmt.Sscanf(portStr, "%d", &port)

	peer := &Peer{
		ID:       fmt.Sprintf("manual-%s", addr),
		Name:     host,
		Host:     host,
		Port:     port,
		Addrs:    nil,
		LastSeen: time.Now(),
		Manual:   true,
	}

	// Try to resolve the hostname
	addrs, err := net.LookupIP(host)
	if err == nil {
		peer.Addrs = addrs
	}

	d.mu.Lock()
	d.peers[peer.ID] = peer
	d.mu.Unlock()

	log.Info().
		Str("peer", peer.Name).
		Str("addr", peer.Address()).
		Msg("Added manual peer")

	if d.onPeerFound != nil {
		d.onPeerFound(peer)
	}
}

func (d *Discovery) healthCheck() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.mu.Lock()
			now := time.Now()
			for id, peer := range d.peers {
				// Remove peers not seen in 2 minutes (unless manual)
				if !peer.Manual && now.Sub(peer.LastSeen) > 2*time.Minute {
					delete(d.peers, id)
					log.Info().Str("peer", peer.Name).Msg("Peer timed out")
					if d.onPeerLost != nil {
						d.onPeerLost(peer)
					}
				}
			}
			d.mu.Unlock()
		case <-d.ctx.Done():
			return
		}
	}
}

// AddManualPeer adds a manual peer at runtime
func (d *Discovery) AddManualPeer(addr string) {
	d.addManualPeer(addr)
}

// RemovePeer removes a peer by ID
func (d *Discovery) RemovePeer(id string) {
	d.mu.Lock()
	peer, exists := d.peers[id]
	if exists {
		delete(d.peers, id)
	}
	d.mu.Unlock()

	if exists && d.onPeerLost != nil {
		d.onPeerLost(peer)
	}
}
