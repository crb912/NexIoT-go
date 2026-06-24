// Package poller handles polling devices.
// pkg/protocol/poller/modbus.go
package poller

import (
	"errors"
	"fmt"
	"octopus-edge/pkg/parser"
	"octopus-edge/pkg/protocol"
	"strconv"
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
	WriteRegister(address, value uint16) error
	WriteRegisters(address uint16, values []uint16) error
	WriteCoil(address uint16, value bool) error
	WriteCoils(address uint16, values []bool) error
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
func (m *ModbusClient) ReadBatch(points []protocol.Resource) error {
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

// WriteSingle writes a single value to the Modbus device.
func (m *ModbusClient) WriteSingle(res *protocol.Resource) error {
	addr := uint16(res.Address.(float64)) - 1
	tt := getTableType(res.Args)

	switch tt {
	case tableCoils:
		val, err := toBool(res.Value)
		if err != nil {
			return fmt.Errorf("WriteSingle coil: %w", err)
		}
		return m.client.WriteCoil(addr, val)
	case tableHoldingRegisters:
		values, err := parser.EncodeRawData(res.Decoder, res.Type, res.Length, res.Value)
		if err != nil {
			return fmt.Errorf("WriteSingle register: %w", err)
		}
		if len(values) == 1 {
			return m.client.WriteRegister(addr, values[0])
		}
		return m.client.WriteRegisters(addr, values)
	case tableDiscretes:
		return errors.New("WriteSingle: discrete inputs are read-only")
	case tableInputRegisters:
		return errors.New("WriteSingle: input registers are read-only")
	default:
		return fmt.Errorf("WriteSingle: unknown table type %s", tt)
	}
}

// WriteBatch writes multiple values to the Modbus device in one request.
// All points must belong to the same primaryTable type.
func (m *ModbusClient) WriteBatch(points []protocol.Resource) error {
	if len(points) == 0 {
		return errors.New("WriteBatch: no points to write")
	}

	minAddr, quantity := calculateBatchSpan(points)
	tt := getTableType(points[0].Args)

	// Prepare value arrays for modbus write (0-based protocol address).
	protocolAddr := minAddr - 1

	switch tt {
	case tableCoils:
		values, err := buildBoolSlice(points, minAddr, quantity)
		if err != nil {
			return fmt.Errorf("WriteBatch coils: %w", err)
		}
		return m.client.WriteCoils(protocolAddr, values)
	case tableHoldingRegisters:
		values, err := buildUint16Slice(points, minAddr, quantity)
		if err != nil {
			return fmt.Errorf("WriteBatch registers: %w", err)
		}
		return m.client.WriteRegisters(protocolAddr, values)
	case tableDiscretes:
		return errors.New("WriteBatch: discrete inputs are read-only")
	case tableInputRegisters:
		return errors.New("WriteBatch: input registers are read-only")
	default:
		return fmt.Errorf("WriteBatch: unknown table type %s", tt)
	}
}

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
func calculateBatchSpan(points []protocol.Resource) (minAddr uint16, quantity uint16) {
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
func assignResValues[T bool | uint16](points []protocol.Resource, minAddr uint16, data []T) {
	for i := range points {
		addr := uint16(points[i].Address.(float64))
		offset := addr - minAddr

		// Safety check to prevent index out of range panic.
		if int(offset)+int(points[i].Length) <= len(data) {
			points[i].Value = extractValue(data[offset:offset+points[i].Length], points[i].Type, points[i].Length)
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

// ─── Write helpers ───────────────────────────────────────────────────────

// toUint16 converts an interface{} value to uint16 for modbus register writes.
// Handles common types received from EdgeX CommandValue params: string, int, float64, uint16.
func toUint16(v any) (uint16, error) {
	switch val := v.(type) {
	case uint16:
		return val, nil
	case uint8:
		return uint16(val), nil
	case int:
		if val < 0 || val > 65535 {
			return 0, fmt.Errorf("toUint16: value %d out of uint16 range", val)
		}
		return uint16(val), nil
	case int64:
		if val < 0 || val > 65535 {
			return 0, fmt.Errorf("toUint16: value %d out of uint16 range", val)
		}
		return uint16(val), nil
	case float64:
		if val < 0 || val > 65535 {
			return 0, fmt.Errorf("toUint16: value %v out of uint16 range", val)
		}
		return uint16(val), nil
	case string:
		u, err := strconv.ParseUint(val, 10, 16)
		if err != nil {
			return 0, fmt.Errorf("toUint16: cannot parse %q: %w", val, err)
		}
		return uint16(u), nil
	default:
		return 0, fmt.Errorf("toUint16: unsupported type %T", v)
	}
}

// toBool converts an interface{} value to bool for modbus coil writes.
func toBool(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case int:
		return val != 0, nil
	case float64:
		return val != 0, nil
	case string:
		switch val {
		case "true", "True", "TRUE", "1":
			return true, nil
		case "false", "False", "FALSE", "0":
			return false, nil
		default:
			return false, fmt.Errorf("toBool: cannot parse %q as bool", val)
		}
	default:
		return false, fmt.Errorf("toBool: unsupported type %T", v)
	}
}

// buildUint16Slice creates a []uint16 slice covering [minAddr, minAddr+quantity)
// by reading each point's Value and placing it at the correct offset.
func buildUint16Slice(points []protocol.Resource, minAddr, quantity uint16) ([]uint16, error) {
	values := make([]uint16, quantity)
	for i := range points {
		addr := uint16(points[i].Address.(float64))
		offset := addr - minAddr
		encoded, err := parser.EncodeRawData(points[i].Decoder, points[i].Type, points[i].Length, points[i].Value)
		if err != nil {
			return nil, fmt.Errorf("buildUint16Slice: resource %s: %w", points[i].Name, err)
		}
		if int(offset)+len(encoded) > len(values) {
			return nil, fmt.Errorf("buildUint16Slice: offset %d + len %d exceeds quantity %d for resource %s",
				offset, len(encoded), quantity, points[i].Name)
		}
		for j := uint16(0); j < uint16(len(encoded)); j++ {
			values[offset+j] = encoded[j]
		}
	}
	return values, nil
}

// buildBoolSlice creates a []bool slice covering [minAddr, minAddr+quantity)
// by reading each point's Value and placing it at the correct offset.
func buildBoolSlice(points []protocol.Resource, minAddr, quantity uint16) ([]bool, error) {
	values := make([]bool, quantity)
	for i := range points {
		addr := uint16(points[i].Address.(float64))
		offset := addr - minAddr
		if int(offset)+1 > len(values) {
			return nil, fmt.Errorf("buildBoolSlice: offset %d out of range for resource %s", offset, points[i].Name)
		}
		val, err := toBool(points[i].Value)
		if err != nil {
			return nil, fmt.Errorf("buildBoolSlice: resource %s: %w", points[i].Name, err)
		}
		values[offset] = val
	}
	return values, nil
}
