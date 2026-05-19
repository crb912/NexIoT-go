// Package client manages IoT protocol clients.
// It handles connecting, disconnecting, and scheduling tasks.
// It reuses existing connections instead of creating new ones.
package client

import (
	"better-iot-edge/pkg/adapter"
	"errors"
	"fmt"
	"sync"
	"time"
)

// DeviceClient holds the configuration for the ClientPool.
type DeviceClient struct {
	Endpoint      string
	protocol      ProType
	Timeout       time.Duration
	maxActiveConn int
	reuseConn     bool
	Args          map[string]interface{}
	pda           ProtocolAdapter
}

// AdapterFactory creates a new ProtocolAdapter.
type AdapterFactory func(endpoint string, timeout time.Duration) ProtocolAdapter

// Pool manages protocol client connections.
// It caches ProtocolAdapter instances keyed by "<protocol>:<endpoint>".
type Pool struct {
	mu        sync.RWMutex
	adapters  map[string]ProtocolAdapter
	factories map[adapter.ProtocolType]AdapterFactory

	timeout       time.Duration
	maxActiveConn int
	reuseConn     bool
}

// Option is a functional option for configuring the Pool.
type Option func(*Pool)

// WithTimeout sets the default timeout for new protocol clients.
func WithTimeout(d time.Duration) Option {
	return func(p *Pool) {
		p.timeout = d
	}
}

// WithMaxCounts sets the maximum number of connections.
func WithMaxCounts(n int) Option {
	return func(p *Pool) {
		p.maxActiveConn = n
	}
}

// WithReuseConn enables or disables connection reuse.
func WithReuseConn(b bool) Option {
	return func(p *Pool) {
		p.reuseConn = b
	}
}

// NewPool creates a new Pool with default settings and applies options.
func NewPool(opts ...Option) *Pool {
	p := &Pool{
		adapters:      make(map[string]ProtocolAdapter),
		factories:     make(map[adapter.ProtocolType]AdapterFactory),
		timeout:       5 * time.Second, // Default timeout
		maxActiveConn: 100,             // Default max connections
		reuseConn:     true,            // Default reuse to true
	}

	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Register adds a factory function for a specific protocol type.
func (p *Pool) Register(pro adapter.ProtocolType, factory AdapterFactory) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.factories[pro] = factory
}

// poolKey builds the cache key from protocol type and endpoint.
func poolKey(pro adapter.ProtocolType, endpoint string) string {
	return fmt.Sprintf("%s:%s", pro, endpoint)
}

// GetOrCreate returns an existing connection or creates a new one.
func (p *Pool) GetOrCreate(pro adapter.ProtocolType, endpoint string) (ProtocolAdapter, error) {
	key := poolKey(pro, endpoint)

	// Step 1: Fast path with read lock
	if p.reuseConn {
		p.mu.RLock()
		protocolAdapter, exists := p.adapters[key]
		p.mu.RUnlock()

		if exists && protocolAdapter.IsConnect() {
			return protocolAdapter, nil
		}
	}

	// Step 2: Write lock for creating a new connection
	p.mu.Lock()
	defer p.mu.Unlock()

	// Step 3: Double-check pattern
	if p.reuseConn {
		if protocolAdapter, exists := p.adapters[key]; exists && protocolAdapter.IsConnect() {
			return protocolAdapter, nil
		}
	}

	// Step 4: Check connection limits
	if len(p.adapters) >= p.maxActiveConn {
		return nil, errors.New("max connections limit reached")
	}

	// Step 5: Find factory and create adapter
	factory, ok := p.factories[pro]
	if !ok {
		return nil, fmt.Errorf("unsupported protocol: %s", pro)
	}

	newAdapter := factory(endpoint, p.timeout)

	// Step 6: Connect to the device
	if err := newAdapter.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect endpoint %s: %w", endpoint, err)
	}

	p.adapters[key] = newAdapter
	return newAdapter, nil
}

// CloseAll disconnects all managed clients and clears the pool.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, protocolAdapter := range p.adapters {
		_ = protocolAdapter.Disconnect()
		delete(p.adapters, key)
	}
}
