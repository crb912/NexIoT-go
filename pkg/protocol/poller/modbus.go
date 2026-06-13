// Package poller
// pkg/protocol/poller/modbus.go
package poller

import (
	"fmt"
	"octopus-edge/pkg/protocol"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/simonvetter/modbus"
)

// Client declares only the methods that ModbusClient actually calls.
// The real *modbus.ModbusClient satisfies this interface automatically.
type Client interface {
	Open() error
	Close() error
	ReadRegisters(address, quantity uint16, registerType modbus.RegType) ([]uint16, error)
	ReadCoils(address, quantity uint16) ([]bool, error)
	ReadDiscreteInputs(address, quantity uint16) ([]bool, error)
}

// ModbusClient holds network settings for the modbus connpool.
type ModbusClient struct {
	// EndPoint is the target address (e.g., "tcp://192.168.1.100:502" or "rtu:///dev/ttyUSB0")
	EndPoint string
	// Timeout is the max time to wait for a reply
	ProtocolType protocol.ProtocolType
	Timeout      time.Duration
	// BaudRate is for RTU only (e.g., 9600, 115200)
	BaudRate uint
	// DataBits is for RTU only (usually 8)
	DataBits uint
	// StopBits is for RTU only (usually 1 or 2)
	StopBits uint
	// Parity is for RTU only (0: None, 1: Odd, 2: Even)
	Parity uint

	mu        sync.Mutex // Protects connection state changes
	connected bool       // Tracks whether the connection is currently established
	client    Client
}

// represents a Modbus data table.
type tableType string

const (
	tableCoils            tableType = "COILS"
	tableDiscretes        tableType = "DISCRETES"
	tableHoldingRegisters tableType = "HOLDING_REGISTERS"
	tableInputRegisters   tableType = "INPUT_REGISTERS"
)

type pointCache struct {
	res  *protocol.Resource
	addr uint16
	len  uint16
}

// newClient creates and configures a new modbus client instance.
// It sets up IP, port, and serial port settings internally.
func (m *ModbusClient) newClient() (*modbus.ModbusClient, error) {
	// 1. Set up the basic configuration using the user inputs
	clientConfig := &modbus.ClientConfiguration{
		URL:     m.EndPoint,
		Timeout: m.Timeout,
	}

	// 2. Add serial port (RTU) settings if a baud rate is provided
	if m.BaudRate > 0 {
		clientConfig.Speed = m.BaudRate
		clientConfig.DataBits = m.DataBits
		clientConfig.StopBits = m.StopBits
		clientConfig.Parity = m.Parity
	}

	// 3. Create the underlying third-party connpool
	underlyingClient, err := modbus.NewClient(clientConfig)
	if err != nil {
		m.connected = false
		return underlyingClient, fmt.Errorf("failed to create modbus connpool: %w", err)
	}
	m.connected = true
	return underlyingClient, nil
}

// Connect opens the physical connection to the device.
// It is thread-safe and safe to call multiple times.
func (m *ModbusClient) Connect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Only create a new client if one doesn't already exist (supports mock injection)
	if m.client == nil {
		client, err := m.newClient()
		if err != nil {
			return err
		}
		m.client = client
	}

	// The underlying library's Open() method establishes the TCP or RTU connection.
	err := m.client.Open()
	if err != nil {
		return fmt.Errorf("failed to connect to modbus device: %w", err)
	}
	m.connected = true

	return nil
}

// Disconnect gracefully closes the physical connection.
// It is thread-safe.
func (m *ModbusClient) Disconnect() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client == nil {
		m.connected = false
		return nil
	}

	// The underlying library's Close() method frees network/serial resources.
	err := m.client.Close()
	m.connected = false
	if err != nil {
		return fmt.Errorf("failed to disconnect modbus device: %w", err)
	}

	return nil
}

func (m *ModbusClient) GetProtocolType() protocol.ProtocolType {
	return m.ProtocolType
}

// IsConnected checks whether the modbus connpool is still connected to the device.
// It verifies the connection by attempting a lightweight read of a holding register.
// Returns true if the connection is alive, false otherwise.
func (m *ModbusClient) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.client == nil || !m.connected {
		return false
	}

	// Attempt a lightweight read to verify the connection is still alive.
	// We read holding register at address 1 — if the device responds (even with
	// a protocol-level error), the transport is alive.
	_, err := m.client.ReadRegisters(1, 1, modbus.HOLDING_REGISTER)
	if err != nil {
		// Check if the error indicates a transport-level failure.
		errStr := err.Error()
		// Common transport-level error indicators
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
		// Other errors (e.g., modbus protocol errors) mean the transport is alive
		// but the device rejected the request for protocol reasons.
	}

	return true
}

// ReadSingle reads a single Modbus resource based on its primaryTable.
// The result is stored in point.Value.
func (m *ModbusClient) ReadSingle(res *protocol.Resource) error {
	addr, err := convProtocolAddr(res.Address)
	if err != nil {
		return fmt.Errorf("modbus ReadSingle: %w", err)
	}

	tt := getTableType(res.Args)
	data, err := m.read(addr, res.Length, tt)
	if err != nil {
		return err
	}
	res.Value = alignSingleValue(data, res.Type, res.Length)
	return nil
}

// read data from the Modbus server using a specific function code.
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

// ReadBatch performs a single span-based read for a list of resources.
// IMPORTANT: This method assumes ALL resources in the slice share the SAME table type.
func (m *ModbusClient) ReadBatch(points []*protocol.Resource) error {
	// Calculate span and build cache
	minAddr, quantity, caches, err := calculateBatchSpan(points)
	if err != nil {
		return err
	}

	tt := getTableType(points[0].Args)
	// Call the unified read function
	dataAny, err := m.read(minAddr, quantity, tt)
	if err != nil {
		return fmt.Errorf("batch read failed for type %s: %w", tt, err)
	}

	// Type switch is necessary to unwrap 'any' into a concrete slice type
	// so the generic assignValues function can accept it.
	switch data := dataAny.(type) {
	case []bool:
		assignResValues(caches, minAddr, data)
	case []uint16:
		assignResValues(caches, minAddr, data)
	default:
		return fmt.Errorf("unexpected data type returned from read: %T", dataAny)
	}
	return nil
}

// calculateBatchSpan calculates the minimum start address, total span length, and returns the cache list
func calculateBatchSpan(points []*protocol.Resource) (minAddr uint16, quantity uint16, caches []pointCache, err error) {
	minAddr = 0xFFFF
	var maxEnd uint16 = 0

	// Pre-allocate slice capacity to improve performance
	caches = make([]pointCache, 0, len(points))

	for i, p := range points {
		addr, err := convProtocolAddr(p.Address)
		if err != nil {
			return 0, 0, nil, fmt.Errorf("point[%d] %s address error: %w", i, p.Name, err)
		}

		length := uint16(p.Length)
		caches = append(caches, pointCache{res: p, addr: addr, len: length})

		if addr < minAddr {
			minAddr = addr
		}

		endAddr := addr + length
		if endAddr > maxEnd {
			maxEnd = endAddr
		}
	}

	quantity = maxEnd - minAddr
	return minAddr, quantity, caches, nil
}

// assignResValues extracts the specific slice of data for each resource based on its offset.
// Using generics here [T bool | uint16] keeps the code extremely clean since []bool and []uint16 slice identically.
func assignResValues[T bool | uint16](caches []pointCache, minAddr uint16, data []T) {
	for _, c := range caches {
		// Calculate how far this specific resource is from the start of the read block
		offset := c.addr - minAddr

		// Safety check to prevent panic (index out of range)
		if int(offset)+int(c.len) <= len(data) {
			// Extract the subset and assign it to the resource
			c.res.Value = alignSingleValue(data[offset:offset+c.len], c.res.Type, c.res.Length)
		}
	}
}

// alignSingleValue aligns raw Modbus slices to the target business type
func alignSingleValue(rawData any, resType string, length uint16) any {
	// Return the raw slice if length is > 1
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

// parseAddress converts a string pointID (e.g., "40001" or "1") to a zero-based Modbus protocol address.
func parseAddress(id string) (uint16, error) {
	addrInt, err := strconv.ParseUint(id, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid address format: %s", id)
	}
	if addrInt == 0 {
		return 0, fmt.Errorf("address must be > 0")
	}
	// Subtract 1 to get the actual protocol address (e.g., 1 -> 0x0000)
	return uint16(addrInt - 1), nil
}

// convProtocolAddr converts a Resource.Address (any type) to a zero-based Modbus protocol address.
// Numeric values (float64, int) are used as-is (0-indexed protocol address).
// String values follow the 1-indexed convention (subtract 1).
func convProtocolAddr(addr any) (uint16, error) {
	switch v := addr.(type) {
	case float64:
		if v < 0 || v > 65535 {
			return 0, fmt.Errorf("address out of range: %v", v)
		}
		return uint16(v) - 1, nil
	case int:
		if v < 0 || v > 65535 {
			return 0, fmt.Errorf("address out of range: %v", v)
		}
		return uint16(v) - 1, nil
	case string:
		return parseAddress(v)
	default:
		return 0, fmt.Errorf("unsupported address type: %T", addr)
	}
}

// getTableType extracts the primaryTable from Resource.Args.
// Defaults to HOLDING_REGISTERS when not specified.
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
