package modbus

import (
	"devices-iot-go/pkg/conv"
	"devices-iot-go/pkg/model"
	"errors"
	"fmt"
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
	ProtocolType model.ProtocolType
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

// NewModbusClient constructs a ModbusClient from a generic args map.
func NewModbusClient(endpoint string, pt model.ProtocolType, defaultTimeout time.Duration, args map[string]string) (*ModbusClient, error) {
	c := &ModbusClient{
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

// ReadSingle reads one Modbus resource.
func (m *ModbusClient) ReadSingle(res *model.Resource) error {
	// Directly convert number to uint16 and subtract 1 for 0-based protocol address.
	addr := uint16(res.Address.(float64)) - 1

	tt := getTableType(res.Args)
	data, err := m.read(addr, res.Length, tt)
	if err != nil {
		return err
	}
	res.Value = extractRegisterValue(data, res.Type, res.Length)
	return nil
}

// ReadBatch reads data for a list of points in one request.
func (m *ModbusClient) ReadBatch(points []model.Resource) error {
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
func (m *ModbusClient) WriteSingle(res *model.Resource) error {
	addr := uint16(res.Address.(float64)) - 1
	tt := getTableType(res.Args)

	switch tt {
	case tableCoils:
		val, err := conv.ToBool(res.Value)
		if err != nil {
			return fmt.Errorf("WriteSingle coil: %w", err)
		}
		return m.client.WriteCoil(addr, val)
	case tableHoldingRegisters:
		values, err := conv.ToUint16Slice(res.Value)
		if err != nil {
			return fmt.Errorf("WriteSingle register: %w, resource=%s, type=%s", err, res.Name, res.Type)
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
func (m *ModbusClient) WriteBatch(points []model.Resource) error {
	if len(points) == 0 {
		return errors.New("WriteBatch: no points to write")
	}

	minAddr, _ := calculateBatchSpan(points)
	tt := getTableType(points[0].Args)

	// Prepare value arrays for modbus write (0-based protocol address).
	protocolAddr := minAddr - 1

	switch tt {
	case tableCoils:
		values, err := toBoolSliceBatch(points)
		if err != nil {
			return fmt.Errorf("WriteBatch coils: %w", err)
		}
		return m.client.WriteCoils(protocolAddr, values)
	case tableHoldingRegisters:
		values, err := toUint16SliceBatch(points)
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
		return underlyingClient, fmt.Errorf("failed to create modbus protocol: %w", err)
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
func calculateBatchSpan(points []model.Resource) (minAddr uint16, quantity uint16) {
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
func assignResValues[T bool | uint16](points []model.Resource, minAddr uint16, data []T) {
	for i := range points {
		addr := uint16(points[i].Address.(float64))
		offset := addr - minAddr

		// Safety check to prevent index out of range panic.
		if int(offset)+int(points[i].Length) <= len(data) {
			points[i].Value = extractRegisterValue(data[offset:offset+points[i].Length], points[i].Type, points[i].Length)
		}
	}
}

// extractRegisterValue get data from raw Modbus data ([]bool, []unit16).
func extractRegisterValue(rawData any, resType string, length uint16) any {
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

// ToBoolSliceBatch convert batch resource values to []bool
// each resource.Value must be bool type
func toBoolSliceBatch(points []model.Resource) ([]bool, error) {
	out := make([]bool, 0, len(points))
	for _, pt := range points {
		b, err := conv.ToBoolSlice(pt.Value)
		if err != nil {
			return nil, fmt.Errorf("resource= %s: %w", pt.Name, err)
		}
		out = append(out, b...)
	}
	return out, nil
}

// ToUint16SliceBatch convert batch resource values to []uint16
func toUint16SliceBatch(points []model.Resource) ([]uint16, error) {
	out := make([]uint16, 0, len(points))
	for _, pt := range points {
		u, err := conv.ToUint16Slice(pt.Value)
		if err != nil {
			return nil, fmt.Errorf("resource= %s: %w", pt.Name, err)
		}
		out = append(out, u...)
	}
	return out, nil
}
