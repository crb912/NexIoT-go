// Package protocol manages IoT protocol clients/servers.
// It handles connecting, disconnecting, and scheduling tasks.
// It reuses existing connections instead of creating new ones.
package protocol

import (
	"devices-iot-go/pkg/adapter/modbus"
	"devices-iot-go/pkg/adapter/snmp"
	"devices-iot-go/pkg/model"
	"errors"
	"fmt"
	"sync"
	"time"
)

// Polls manages protocol connpool connections.
// It caches ReadClient instances keyed by "<protocol>:<endpoint>".
type Polls struct {
	mu      sync.RWMutex
	timeout time.Duration
	maxSize int
	clients map[string]ReadClient
}

// Option is a functional option for configuring the Polls.
type Option func(*Polls)

// WithTimeout sets the default timeout for new protocol clients.
func WithTimeout(d time.Duration) Option {
	return func(p *Polls) {
		p.timeout = d
	}
}

// WithMaxCounts sets the maximum number of connections.
func WithMaxCounts(n int) Option {
	return func(p *Polls) {
		p.maxSize = n
	}
}

// NewPolls creates a new Polls with default settings and applies options.
func NewPolls(opts ...Option) *Polls {
	p := &Polls{
		clients: make(map[string]ReadClient),
		timeout: 5 * time.Second, // Default timeout
		maxSize: 400,             // Default max connections
	}

	for _, opt := range opts {
		opt(p)
	}
	return p
}

// GetHandler returns an existing connection or creates a new one.
func (p *Polls) GetHandler(pc *model.ProtocolConfig) (ReadClient, error) {
	protocol_, err := model.ValidateProtocol(pc.Name)
	if err != nil {
		return nil, err
	}
	endpoint := pc.GetEndpoint()

	protocolAdapter, exists := p.clients[endpoint]
	if exists {
		return protocolAdapter, nil
	}

	// Check connection limits before creating a new entry.
	if len(p.clients) >= p.maxSize {
		return nil, errors.New("max connections limit reached")
	}

	// Create and connect the new adapter.
	newAdapter, err := p.newClient(endpoint, protocol_, pc.GetTimeout(), pc.GetRawProtocolProperties())
	if err != nil {
		return nil, fmt.Errorf("failed to create protocol for endpoint %s: %w", endpoint, err)
	}

	if err := newAdapter.Connect(); err != nil {
		return nil, fmt.Errorf("failed to connect endpoint %s: %w", endpoint, err)
	}

	p.addClient(endpoint, newAdapter)
	return newAdapter, nil
}

// DisconnectAll disconnects all managed clients and clears the pool.
func (p *Polls) DisconnectAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for key, protocolAdapter := range p.clients {
		_ = protocolAdapter.Disconnect()
		delete(p.clients, key)
	}
}

// addClient stores a protocol under the given key.
func (p *Polls) addClient(key string, client ReadClient) {
	p.clients[key] = client
}

// removeClient deletes a protocol under the given key.
func (p *Polls) removeClient(key string) {
	delete(p.clients, key)
}

// newClient is a factory that returns the correct RWClient
// based on protocolName. For "modbus-tcp" and "modbus-rtu" it builds a
// ModbusClient from the provided args map; other protocols can be added here.
func (p *Polls) newClient(endpoint string, protocolName model.ProtocolType, timeout time.Duration, args map[string]string) (RWClient, error) {
	switch protocolName {
	case model.ModbusTCP:
		return modbus.NewModbusClient(endpoint, model.ModbusTCP, timeout, args)
	case model.ModbusRTU:
		return modbus.NewModbusClient(endpoint, model.ModbusRTU, timeout, args)
	case model.SNMP:
		return snmp.NewSnmpClient(endpoint, model.SNMP, timeout, args)
	default:
		return nil, fmt.Errorf("unsupported protocol: %s", protocolName)
	}
}
