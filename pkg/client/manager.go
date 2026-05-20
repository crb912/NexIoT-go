// Package client manages IoT protocol clients.
// It handles connecting, disconnecting, and scheduling tasks.
// It reuses existing connections instead of creating new ones.
package client

import (
	"better-iot-edge/pkg/adapter"
	"better-iot-edge/pkg/conv"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Pool manages protocol client connections.
// It caches ProtocolAdapter instances keyed by "<protocol>:<endpoint>".
type Pool struct {
	mu      sync.RWMutex
	timeout time.Duration
	count   int
	clients map[string]ProtocolAdapter
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
		p.count = n
	}
}

// NewPool creates a new Pool with default settings and applies options.
func NewPool(opts ...Option) *Pool {
	p := &Pool{
		clients: make(map[string]ProtocolAdapter),
		timeout: 5 * time.Second, // Default timeout
		count:   100,             // Default max connections
	}

	for _, opt := range opts {
		opt(p)
	}
	return p
}

// GetOrCreate returns an existing connection or creates a new one.
func (p *Pool) GetOrCreate(endpoint string, protocolName string, args map[string]interface{}) (ProtocolAdapter, error) {
	protocol, err := validateProtocol(protocolName)
	if err != nil {
		return nil, err
	}

	key := genUniqueKey(protocol, endpoint)

	// Fast path: return existing healthy connection without a write lock.
	p.mu.RLock()
	protocolAdapter, exists := p.clients[key]
	p.mu.RUnlock()

	if exists && protocolAdapter.IsConnect() {
		return protocolAdapter, nil
	}

	// Slow path: create a new connection under a write lock.
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring the write lock.
	protocolAdapter, exists = p.clients[key]
	if exists && protocolAdapter.IsConnect() {
		return protocolAdapter, nil
	}

	// Check connection limits before creating a new entry.
	if len(p.clients) >= p.count {
		return nil, errors.New("max connections limit reached")
	}

	// Create and connect the new adapter.
	newAdapter, err := p.createClient(endpoint, protocolName, args)
	if err != nil {
		return nil, fmt.Errorf("failed to create client for endpoint %s: %w", endpoint, err)
	}

	if err := newAdapter.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect endpoint %s: %w", endpoint, err)
	}

	p.register(key, newAdapter)
	return newAdapter, nil
}

// CloseAll disconnects all managed clients and clears the pool.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, protocolAdapter := range p.clients {
		_ = protocolAdapter.Disconnect()
		delete(p.clients, key)
	}
}

// register stores a client under the given key.
// Caller must hold p.mu write lock.
func (p *Pool) register(key string, client ProtocolAdapter) {
	p.clients[key] = client
}

// createClient is a factory that returns the correct ProtocolAdapter
// based on protocolName. For "modbus-tcp" and "modbus-rtu" it builds a
// ModbusClient from the provided args map; other protocols can be added here.
func (p *Pool) createClient(endpoint, protocolName string, args map[string]interface{}) (ProtocolAdapter, error) {
	switch protocolName {
	case "modbus-tcp":
		return newModbusClient(endpoint, adapter.ProtocolModbusTCP, p.timeout, args)
	case "modbus-rtu":
		return newModbusClient(endpoint, adapter.ProtocolModbusRTU, p.timeout, args)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocolName)
	}
}

// genUniqueKey builds the cache key from protocol type and endpoint.
func genUniqueKey(pro adapter.ProtocolType, endpoint string) string {
	return fmt.Sprintf("%s:%s", pro, endpoint)
}

func validateProtocol(protocolName string) (adapter.ProtocolType, error) {
	switch protocolName {
	case "modbus-tcp":
		return adapter.ProtocolModbusTCP, nil
	case "modbus-rtu":
		return adapter.ProtocolModbusRTU, nil
	default:
		return adapter.ProtocolUnknown, errors.New("not support protocol type")
	}
}

// newModbusClient constructs a ModbusClient from a generic args map.
// Keys recognized in args:
//
//	"baud_rate"  uint  – serial baud rate (RTU only)
//	"data_bits"  uint  – data bits        (RTU only, default 8)
//	"stop_bits"  uint  – stop bits        (RTU only, default 1)
//	"parity"     uint  – 0=None 1=Odd 2=Even (RTU only, default 0)
//	"timeout"    time.Duration – overrides the pool-level timeout
func newModbusClient(endpoint string, pt adapter.ProtocolType, defaultTimeout time.Duration, args map[string]interface{}) (*adapter.ModbusClient, error) {
	c := &adapter.ModbusClient{
		EndPoint:     endpoint,
		ProtocolType: pt,
		Timeout:      defaultTimeout,
		// RTU defaults
		DataBits: 8,
		StopBits: 1,
		Parity:   0,
	}

	if args == nil {
		return c, nil
	}

	if v, ok := args["timeout"]; ok {
		if d, ok := v.(time.Duration); ok {
			c.Timeout = d
		}
	}
	if v, ok := args["baud_rate"]; ok {
		if u, ok := conv.ToUint(v); ok {
			c.BaudRate = u
		}
	}
	if v, ok := args["data_bits"]; ok {
		if u, ok := conv.ToUint(v); ok {
			c.DataBits = u
		}
	}
	if v, ok := args["stop_bits"]; ok {
		if u, ok := conv.ToUint(v); ok {
			c.StopBits = u
		}
	}
	if v, ok := args["parity"]; ok {
		if u, ok := conv.ToUint(v); ok {
			c.Parity = u
		}
	}

	return c, nil
}
