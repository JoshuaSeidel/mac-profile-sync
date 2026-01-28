package network

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
)

// Client handles outgoing connections to peers
type Client struct {
	tlsConfig *tls.Config
	ctx       context.Context
	cancel    context.CancelFunc

	// Active connections
	connections map[string]*ClientConnection
	connMu      sync.RWMutex

	// Handlers
	onConnect    func(*ClientConnection)
	onDisconnect func(*ClientConnection)
	onMessage    func(*ClientConnection, *Message)
}

// ClientConnection represents an outgoing connection to a peer
type ClientConnection struct {
	ID         string
	Address    string
	DeviceName string
	DeviceID   string
	Conn       net.Conn
	Client     *Client
	Paired     bool
	LastSeen   time.Time

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

// NewClient creates a new network client
func NewClient(tlsConfig *tls.Config) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		tlsConfig:   tlsConfig,
		ctx:         ctx,
		cancel:      cancel,
		connections: make(map[string]*ClientConnection),
	}
}

// SetHandlers sets the connection handlers
func (c *Client) SetHandlers(onConnect, onDisconnect func(*ClientConnection), onMessage func(*ClientConnection, *Message)) {
	c.onConnect = onConnect
	c.onDisconnect = onDisconnect
	c.onMessage = onMessage
}

// Connect establishes a connection to a peer
func (c *Client) Connect(address string) (*ClientConnection, error) {
	// Check if already connected
	c.connMu.RLock()
	if existing, ok := c.connections[address]; ok {
		c.connMu.RUnlock()
		return existing, nil
	}
	c.connMu.RUnlock()

	var conn net.Conn
	var err error

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	if c.tlsConfig != nil {
		conn, err = tls.DialWithDialer(dialer, "tcp", address, c.tlsConfig)
	} else {
		conn, err = dialer.DialContext(c.ctx, "tcp", address)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}

	ctx, cancel := context.WithCancel(c.ctx)
	clientConn := &ClientConnection{
		ID:       address,
		Address:  address,
		Conn:     conn,
		Client:   c,
		LastSeen: time.Now(),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Register connection
	c.connMu.Lock()
	c.connections[address] = clientConn
	c.connMu.Unlock()

	log.Info().Str("address", address).Msg("Connected to peer")

	if c.onConnect != nil {
		c.onConnect(clientConn)
	}

	// Start read loop in background
	go clientConn.readLoop()

	return clientConn, nil
}

// Disconnect closes a connection to a peer
func (c *Client) Disconnect(address string) {
	c.connMu.Lock()
	conn, ok := c.connections[address]
	if ok {
		delete(c.connections, address)
	}
	c.connMu.Unlock()

	if ok {
		conn.Close()
	}
}

// Stop stops the client and closes all connections
func (c *Client) Stop() {
	c.cancel()

	c.connMu.Lock()
	for _, conn := range c.connections {
		conn.Close()
	}
	c.connections = make(map[string]*ClientConnection)
	c.connMu.Unlock()

	log.Info().Msg("Client stopped")
}

// GetConnections returns all active connections
func (c *Client) GetConnections() []*ClientConnection {
	c.connMu.RLock()
	defer c.connMu.RUnlock()

	conns := make([]*ClientConnection, 0, len(c.connections))
	for _, conn := range c.connections {
		conns = append(conns, conn)
	}
	return conns
}

// GetConnection returns a specific connection by address
func (c *Client) GetConnection(address string) *ClientConnection {
	c.connMu.RLock()
	defer c.connMu.RUnlock()
	return c.connections[address]
}

// Close closes the connection
func (cc *ClientConnection) Close() {
	cc.cancel()
	_ = cc.Conn.Close()

	if cc.Client.onDisconnect != nil {
		cc.Client.onDisconnect(cc)
	}

	log.Info().Str("address", cc.Address).Msg("Disconnected from peer")
}

// Send sends a message to the peer
func (cc *ClientConnection) Send(msg *Message) error {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	_ = cc.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return WriteMessage(cc.Conn, msg)
}

// SendPayload creates and sends a message with the given payload
func (cc *ClientConnection) SendPayload(msgType MessageType, payload interface{}) error {
	msg, err := NewMessage(msgType, payload)
	if err != nil {
		return err
	}
	return cc.Send(msg)
}

func (cc *ClientConnection) readLoop() {
	defer func() {
		cc.Client.connMu.Lock()
		delete(cc.Client.connections, cc.Address)
		cc.Client.connMu.Unlock()

		if cc.Client.onDisconnect != nil {
			cc.Client.onDisconnect(cc)
		}
	}()

	for {
		select {
		case <-cc.ctx.Done():
			return
		default:
		}

		_ = cc.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		msg, err := ReadMessage(cc.Conn)
		if err != nil {
			select {
			case <-cc.ctx.Done():
			default:
				log.Debug().Err(err).Str("address", cc.Address).Msg("Read error")
			}
			return
		}

		cc.LastSeen = time.Now()

		// Handle ping/pong internally
		if msg.Type == MsgPing {
			_ = cc.SendPayload(MsgPong, nil)
			continue
		}

		if cc.Client.onMessage != nil {
			cc.Client.onMessage(cc, msg)
		}
	}
}

// Ping sends a ping to the peer and waits for pong
func (cc *ClientConnection) Ping() error {
	return cc.SendPayload(MsgPing, nil)
}
