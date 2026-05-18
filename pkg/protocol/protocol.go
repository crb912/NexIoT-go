// Package protocol define the IoT protocol layer interfaces and the data structures for the returned resources.
package protocol

import (
	"fmt"
	"sync"
	"time"
)

// Resource represents a single piece of data read from a device.
type Resource struct {
	// Address is the unique identifier for the register or tag.
	// Examples: "40001" (Modbus), "ns=2;i=123" (OPC UA), "/api/v1/temp" (HTTP).
	Address string

	// RawValue holds the raw data from the protocol.
	// For Modbus/HTTP, this is usually []byte.
	// For OPC UA, this could be native Go types (e.g., float64, int32).
	RawValue any

	// Timestamp records the exact time the data was received.
	Timestamp time.Time

	// IsValid indicates if the read operation for this specific point was successful.
	// This is critical for batch reads where some points might fail while others succeed.
	IsValid bool

	// Error holds the specific failure reason if IsValid is false.
	Error error
}

type ProType string

// Config represents the unified configuration for any protocol.
// This is usually parsed from config.toml or JSON.
type Config struct {
	protocol ProType       // e.g., "modbus-tcp", "modbus-rtu", "opcua", "http"
	Endpoint string        // e.g., "tcp://127.0.0.1:502" or "/dev/ttyUSB0"
	Timeout  time.Duration // Connection and read timeout
	// You can add more protocol-specific settings here if needed
}

// Reader defines the standard read interface for all protocol plugins.
type Reader interface {
	ReadSingle(pointID string) (Resource, error)
	ReadBatch(pointIDs []string) ([]Resource, error)
}

// ReaderClient extends the Reader interface with lifecycle management.
// The main program will ONLY interact with this interface.
type ReaderClient interface {
	Connect() error
	Disconnect() error
	GetProtocolType() ProType
	Reader
}

// ProtocolAdapter defines the interface that all protocol adapters must implement
// to be managed by the Server connection pool.
type ProtocolAdapter interface {
	Connect() error
	Disconnect() error
	GetProtocolType() ProType
	GetEndpoint() string
	IsConnect() bool
	Reader
}

type WriterClient interface {
	Connect() error
	Disconnect() error
	GetProtocolType() ProType
	Writer
}

// Writer defines the standard write interface for all protocol plugins.
type Writer interface {
	WriteSingle(addr string, value interface{}) error
	WriteBatch(points []Resource) error // 连续写 n 个点
}

type SingleReader interface {
	ReadSingle(pointID string) (Resource, error)
}

type BatchReader interface {
	ReadBatch(pointIDs []string) ([]Resource, error)
}

type RWClient interface {
	Reader
	Writer
	GetProtocolType() ProType
}

// --------------------------------------------------------------------------
// Server — connection pool with functional options
// --------------------------------------------------------------------------

// serverOptions holds the configuration for the Server.
type serverOptions struct {
	timeout   time.Duration
	maxCounts int
	reuseConn bool
}

// ServerOption is a functional option for configuring the Server.
type ServerOption func(*serverOptions)

// WithTimeout sets the default connection/read timeout for new protocol clients.
func WithTimeout(d time.Duration) ServerOption {
	return func(o *serverOptions) {
		o.timeout = d
	}
}

// WithMaxCounts sets the maximum number of connections the pool can hold.
// When the limit is reached, GetOrCreate returns an error.
func WithMaxCounts(n int) ServerOption {
	return func(o *serverOptions) {
		o.maxCounts = n
	}
}

// WithReuseConn enables or disables connection reuse.
// When false, every GetOrCreate call creates a brand-new connection.
func WithReuseConn(b bool) ServerOption {
	return func(o *serverOptions) {
		o.reuseConn = b
	}
}

// ClientFactory is a function that creates a new ProtocolAdapter for the given
// protocol type and endpoint.
type ClientFactory func(pro ProType, endpoint string) (ProtocolAdapter, error)

// Server manages protocol client connections with pooling support.
// It caches ProtocolAdapter instances keyed by "<protocol>:<endpoint>",
// reusing existing connections when IsConnect() returns true.
type Server struct {
	mu      sync.RWMutex
	clients map[string]ProtocolAdapter
	opts    serverOptions
	factory ClientFactory
}

// NewServer creates a new Server (connection pool) with the given functional options.
//
// Example:
//
//	srv := protocol.NewServer(
//	    protocol.WithTimeout(5 * time.Second),
//	    protocol.WithMaxCounts(20),
//	    protocol.WithReuseConn(true),
//	)
func NewServer(opts ...ServerOption) *Server {
	o := serverOptions{
		timeout:   5 * time.Second,
		maxCounts: 10,
		reuseConn: true,
	}
	for _, opt := range opts {
		opt(&o)
	}
	return &Server{
		clients: make(map[string]ProtocolAdapter),
		opts:    o,
	}
}

// SetFactory registers a custom ClientFactory. This is required before calling
// GetOrCreate if the pool needs to create new connections on demand.
// If no factory is set, GetOrCreate will return an error when a new connection
// is needed.
func (s *Server) SetFactory(f ClientFactory) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.factory = f
}

// poolKey builds the cache key from protocol type and endpoint.
func poolKey(pro ProType, endpoint string) string {
	return fmt.Sprintf("%s:%s", pro, endpoint)
}

// GetOrCreate returns an existing connected ProtocolAdapter for the given
// protocol and endpoint, or creates a new one via the registered factory.
//
// Reuse logic:
//   - If reuse is enabled and a cached client exists with IsConnect() == true,
//     it is returned immediately.
//   - If a cached client exists but IsConnect() == false, it is removed and
//     a new one is created.
//   - If no cached client exists, a new one is created via the factory.
//   - If reuse is disabled, a new connection is always created.
func (s *Server) GetOrCreate(pro ProType, endpoint string) (ProtocolAdapter, error) {
	key := poolKey(pro, endpoint)

	s.mu.Lock()
	defer s.mu.Unlock()

	// If reuse is enabled, try to find an existing live connection.
	if s.opts.reuseConn {
		if existing, ok := s.clients[key]; ok {
			if existing.IsConnect() {
				return existing, nil
			}
			// Stale connection — disconnect and remove.
			_ = existing.Disconnect()
			delete(s.clients, key)
		}
	}

	// Check max connections limit.
	if s.opts.maxCounts > 0 && len(s.clients) >= s.opts.maxCounts {
		return nil, fmt.Errorf("connection pool full: %d/%d connections", len(s.clients), s.opts.maxCounts)
	}

	// Create a new connection via the factory.
	if s.factory == nil {
		return nil, fmt.Errorf("no client factory registered: call SetFactory() first")
	}

	client, err := s.factory(pro, endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for %s/%s: %w", pro, endpoint, err)
	}

	// Connect the new client.
	if err := client.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect client for %s/%s: %w", pro, endpoint, err)
	}

	// Store in pool.
	s.clients[key] = client

	return client, nil
}

// DisconnectAll disconnects all cached clients and clears the pool.
func (s *Server) DisconnectAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for key, client := range s.clients {
		_ = client.Disconnect()
		delete(s.clients, key)
	}
}

// Count returns the number of clients currently in the pool.
func (s *Server) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.clients)
}

// Options returns a copy of the current server options.
func (s *Server) Options() serverOptions {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.opts
}
