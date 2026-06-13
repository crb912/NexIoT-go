// Package poller handles polling devices.
// pkg/protocol/poller/modbus.go
package poller

import (
	"fmt"
	"octopus-edge/pkg/protocol"
	"strings"
	"sync"
	"time"

	"github.com/simonvetter/modbus"
)

// Client declares methods that ModbusClient calls.
type Client interface {
	Open() error
	Close() error
	ReadRegisters(address, quantity uint16, registerType modbus.RegType) ([]uint16, error)
	ReadCoils(address, quantity uint16) ([]bool, error)
	ReadDiscreteInputs(address, quantity uint16) ([]bool, error)
}

// ModbusClient holds network settings for the connection.
type ModbusClient struct {
	EndPoint     string
	ProtocolType protocol.ProtocolType
	Timeout      time.Duration
	BaudRate     uint
	DataBits     uint
	StopBits     uint
	Parity       uint

	mu        sync.Mutex
	connected bool
	client    Client
}

// tableType represents a Modbus data table.
type tableType string

const (
	tableCoils            tableType = "COILS"
	tableDiscretes        tableType = "DISCRETES"
	tableHoldingRegisters tableType = "HOLDING_REGISTERS"
	tableInputRegisters   tableType = "INPUT_REGISTERS"
)

// newClient creates and configures a new modbus client.
func (m *ModbusClient) newClient() (*modbus.ModbusClient, error) {
	clientConfig := &modbus.ClientConfiguration{
		URL:     m.EndPoint,
		Timeout: m.Timeout,
	}

	if m.BaudRate > 0 {
		clientConfig.Speed = m.BaudRate
		clientConfig.DataBits = m.DataBits
		clientConfig.StopBits = m.StopBits
		clientConfig.Parity = m.Parity
	}

	underlyingClient, err := modbus.NewClient(clientConfig)
	if err != nil {
		m.connected = false
		return underlyingClient, fmt.Errorf("failed to create modbus client: %w", err)
	}
	m.connected = true
	return underlyingClient, nil
}

// Connect opens the physical connection.
func (m *ModbusClient) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client == nil {
		client, err := m.newClient()
		if err != nil {
			return err
		}
		m.client = client
	}

	err := m.client.Open()
	if err != nil {
		return fmt.Errorf("failed to connect to device: %w", err)
	}
	m.connected = true

	return nil
}

// Disconnect closes the physical connection.
func (m *ModbusClient) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client == nil {
		m.connected = false
		return nil
	}

	err := m.client.Close()
	m.connected = false
	if err != nil {
		return fmt.Errorf("failed to disconnect device: %w", err)
	}

	return nil
}

// GetProtocolType returns the protocol type.
func (m *ModbusClient) GetProtocolType() protocol.ProtocolType {
	return m.ProtocolType
}

// IsConnected checks if the client is still connected.
func (m *ModbusClient) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client == nil || !m.connected {
		return false
	}

	_, err := m.client.ReadRegisters(1, 1, modbus.HOLDING_REGISTER)
	if err != nil {
		errStr := err.Error()
		transportErrors := []string{
			"connection refused",
			"broken pipe",
			"closed network",
			"no such file or directory",
			"no such device",
		}
		for _, te := range transportErrors {
			if strings.Contains(errStr, te) {
				m.connected = false
				return false
			}
		}
	}

	return true
}

// ReadSingle reads one Modbus resource.
func (m *ModbusClient) ReadSingle(res *protocol.Resource) error {
	// Directly convert number to uint16 and subtract 1 for 0-based protocol address.
	addr := uint16(res.Address.(float64)) - 1

	tt := getTableType(res.Args)
	data, err := m.read(addr, res.Length, tt)
	if err != nil {
		return err
	}
	res.Value = extractValue(data, res.Type, res.Length)
	return nil
}

// ReadBatch reads data for a list of points in one request.
func (m *ModbusClient) ReadBatch(points []*protocol.Resource) error {
	minAddr, quantity := calculateBatchSpan(points)

	tt := getTableType(points[0].Args)
	dataAny, err := m.read(minAddr-1, quantity, tt)
	if err != nil {
		return fmt.Errorf("batch read failed for type %s: %w", tt, err)
	}

	switch data := dataAny.(type) {
	case []bool:
		assignResValues(points, minAddr, data)
	case []uint16:
		assignResValues(points, minAddr, data)
	default:
		return fmt.Errorf("unexpected data type returned from read: %T", dataAny)
	}
	return nil
}

// read reads data from the Modbus server.
func (m *ModbusClient) read(addressStart uint16, quantity uint16, tt tableType) (any, error) {
	switch tt {
	case tableCoils:
		return m.client.ReadCoils(addressStart, quantity)
	case tableDiscretes:
		return m.client.ReadDiscreteInputs(addressStart, quantity)
	case tableInputRegisters:
		return m.client.ReadRegisters(addressStart, quantity, modbus.INPUT_REGISTER)
	default:
		return m.client.ReadRegisters(addressStart, quantity, modbus.HOLDING_REGISTER)
	}
}

// calculateBatchSpan gets the start address and total length.
func calculateBatchSpan(points []*protocol.Resource) (minAddr uint16, quantity uint16) {
	minAddr = 0xFFFF
	var maxEnd uint16 = 0

	for _, p := range points {
		// No subtraction here. Keep it 1-based.
		addr := uint16(p.Address.(float64))
		length := p.Length

		if addr < minAddr {
			minAddr = addr
		}

		endAddr := addr + length
		if endAddr > maxEnd {
			maxEnd = endAddr
		}
	}

	quantity = maxEnd - minAddr
	return minAddr, quantity
}

// assignResValues sets the correct data slice for each resource.
func assignResValues[T bool | uint16](points []*protocol.Resource, minAddr uint16, data []T) {
	for _, p := range points {
		addr := uint16(p.Address.(float64))
		offset := addr - minAddr

		// Safety check to prevent index out of range panic.
		if int(offset)+int(p.Length) <= len(data) {
			p.Value = extractValue(data[offset:offset+p.Length], p.Type, p.Length)
		}
	}
}

// extractValue get data from raw Modbus data ([]bool, []unit16).
func extractValue(rawData any, resType string, length uint16) any {
	if length > 1 {
		return rawData
	}

	switch v := rawData.(type) {
	case []uint16:
		return v[0]
	case []bool:
		return v[0]
	default:
		return rawData
	}
}

// getTableType gets the primaryTable from args.
func getTableType(args map[string]any) tableType {
	if args == nil {
		return tableHoldingRegisters
	}

	raw, ok := args["primaryTable"]
	if !ok {
		return tableHoldingRegisters
	}
	s, ok := raw.(string)
	if !ok {
		return tableHoldingRegisters
	}

	t := tableType(s)
	switch t {
	case tableCoils, tableDiscretes, tableInputRegisters, tableHoldingRegisters:
		return t
	default:
		return tableHoldingRegisters
	}
}
