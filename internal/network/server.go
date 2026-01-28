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

// Server handles incoming connections from peers
type Server struct {
	port       int
	tlsConfig  *tls.Config
	listener   net.Listener
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup

	// Connection management
	connections map[string]*Connection
	connMu      sync.RWMutex

	// Handlers
	onConnect    func(*Connection)
	onDisconnect func(*Connection)
	onMessage    func(*Connection, *Message)
}

// Connection represents a peer connection
type Connection struct {
	ID         string
	DeviceName string
	DeviceID   string
	Conn       net.Conn
	Server     *Server
	Paired     bool
	LastSeen   time.Time

	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

// NewServer creates a new network server
func NewServer(port int, tlsConfig *tls.Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		port:        port,
		tlsConfig:   tlsConfig,
		ctx:         ctx,
		cancel:      cancel,
		connections: make(map[string]*Connection),
	}
}

// SetHandlers sets the connection handlers
func (s *Server) SetHandlers(onConnect, onDisconnect func(*Connection), onMessage func(*Connection, *Message)) {
	s.onConnect = onConnect
	s.onDisconnect = onDisconnect
	s.onMessage = onMessage
}

// Start starts the server
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.port)

	var err error
	if s.tlsConfig != nil {
		s.listener, err = tls.Listen("tcp", addr, s.tlsConfig)
	} else {
		s.listener, err = net.Listen("tcp", addr)
	}

	if err != nil {
		return fmt.Errorf("failed to start listener: %w", err)
	}

	log.Info().
		Int("port", s.port).
		Bool("tls", s.tlsConfig != nil).
		Msg("Server started")

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop stops the server
func (s *Server) Stop() {
	s.cancel()

	if s.listener != nil {
		s.listener.Close()
	}

	// Close all connections
	s.connMu.Lock()
	for _, conn := range s.connections {
		conn.Close()
	}
	s.connMu.Unlock()

	s.wg.Wait()
	log.Info().Msg("Server stopped")
}

// GetConnections returns all active connections
func (s *Server) GetConnections() []*Connection {
	s.connMu.RLock()
	defer s.connMu.RUnlock()

	conns := make([]*Connection, 0, len(s.connections))
	for _, c := range s.connections {
		conns = append(conns, c)
	}
	return conns
}

// GetConnection returns a specific connection by ID
func (s *Server) GetConnection(id string) *Connection {
	s.connMu.RLock()
	defer s.connMu.RUnlock()
	return s.connections[id]
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				log.Error().Err(err).Msg("Failed to accept connection")
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(netConn net.Conn) {
	defer s.wg.Done()

	ctx, cancel := context.WithCancel(s.ctx)
	conn := &Connection{
		ID:       netConn.RemoteAddr().String(),
		Conn:     netConn,
		Server:   s,
		LastSeen: time.Now(),
		ctx:      ctx,
		cancel:   cancel,
	}

	// Register connection
	s.connMu.Lock()
	s.connections[conn.ID] = conn
	s.connMu.Unlock()

	log.Info().Str("remote", conn.ID).Msg("New connection")

	if s.onConnect != nil {
		s.onConnect(conn)
	}

	// Handle messages
	conn.readLoop()

	// Cleanup
	s.connMu.Lock()
	delete(s.connections, conn.ID)
	s.connMu.Unlock()

	if s.onDisconnect != nil {
		s.onDisconnect(conn)
	}

	log.Info().Str("remote", conn.ID).Msg("Connection closed")
}

// Close closes the connection
func (c *Connection) Close() {
	c.cancel()
	c.Conn.Close()
}

// Send sends a message to the peer
func (c *Connection) Send(msg *Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	return WriteMessage(c.Conn, msg)
}

// SendPayload creates and sends a message with the given payload
func (c *Connection) SendPayload(msgType MessageType, payload interface{}) error {
	msg, err := NewMessage(msgType, payload)
	if err != nil {
		return err
	}
	return c.Send(msg)
}

func (c *Connection) readLoop() {
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		c.Conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		msg, err := ReadMessage(c.Conn)
		if err != nil {
			select {
			case <-c.ctx.Done():
			default:
				log.Debug().Err(err).Str("remote", c.ID).Msg("Read error")
			}
			return
		}

		c.LastSeen = time.Now()

		// Handle ping/pong internally
		if msg.Type == MsgPing {
			c.SendPayload(MsgPong, nil)
			continue
		}

		if c.Server.onMessage != nil {
			c.Server.onMessage(c, msg)
		}
	}
}

// Port returns the server port
func (s *Server) Port() int {
	return s.port
}

// Broadcast sends a message to all connected peers
func (s *Server) Broadcast(msg *Message) {
	s.connMu.RLock()
	defer s.connMu.RUnlock()

	for _, conn := range s.connections {
		if err := conn.Send(msg); err != nil {
			log.Error().Err(err).Str("remote", conn.ID).Msg("Broadcast failed")
		}
	}
}

// BroadcastPayload creates and broadcasts a message to all peers
func (s *Server) BroadcastPayload(msgType MessageType, payload interface{}) error {
	msg, err := NewMessage(msgType, payload)
	if err != nil {
		return err
	}
	s.Broadcast(msg)
	return nil
}
