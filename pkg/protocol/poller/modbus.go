// Package poller
// pkg/protocol/poller/modbus.go
package poller

import (
	"encoding/binary"
	"fmt"
	"octopus-edge/pkg/model"
	"octopus-edge/pkg/protocol"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/simonvetter/modbus"
)

// modbusClientIface declares only the methods that ModbusClient actually calls.
// The real *modbus.ModbusClient satisfies this interface automatically.
type modbusClientIface interface {
	Open() error
	Close() error
	ReadRegisters(address, quantity uint16, registerType modbus.RegType) ([]uint16, error)
	ReadCoils(address, quantity uint16) ([]bool, error)
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
	client    modbusClientIface
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

// convAddress converts a Resource.Address (any type) to a zero-based Modbus protocol address.
// Numeric values (float64, int) are used as-is (0-indexed protocol address).
// String values follow the 1-indexed convention (subtract 1).
func convAddress(addr any) (uint16, error) {
	switch v := addr.(type) {
	case float64:
		if v < 0 || v > 65535 {
			return 0, fmt.Errorf("address out of range: %v", v)
		}
		return uint16(v), nil
	case int:
		if v < 0 || v > 65535 {
			return 0, fmt.Errorf("address out of range: %v", v)
		}
		return uint16(v), nil
	case string:
		return parseAddress(v)
	default:
		return 0, fmt.Errorf("unsupported address type: %T", addr)
	}
}

// tableType represents a Modbus data table.
type tableType string

const (
	tableCoils            tableType = "COILS"
	tableHoldingRegisters tableType = "HOLDING_REGISTERS"
	tableInputRegisters   tableType = "INPUT_REGISTERS"
)

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
	switch tableType(s) {
	case tableCoils:
		return tableCoils
	case tableInputRegisters:
		return tableInputRegisters
	default:
		return tableHoldingRegisters
	}
}

// ReadSingle reads a single Modbus resource based on its primaryTable.
// The result is stored in point.Value.
func (m *ModbusClient) ReadSingle(point *model.Resource) error {
	addr, err := convAddress(point.Address)
	if err != nil {
		return fmt.Errorf("modbus ReadSingle: %w", err)
	}

	tt := getTableType(point.Args)

	switch tt {
	case tableCoils:
		coils, err := m.client.ReadCoils(addr, uint16(point.Length))
		if err != nil {
			return fmt.Errorf("modbus ReadCoils addr=%d len=%d: %w", addr, point.Length, err)
		}
		point.Value = coils
	case tableInputRegisters:
		regs, err := m.client.ReadRegisters(addr, uint16(point.Length), modbus.INPUT_REGISTER)
		if err != nil {
			return fmt.Errorf("modbus ReadRegisters(input) addr=%d len=%d: %w", addr, point.Length, err)
		}
		point.Value = regs
	default: // tableHoldingRegisters
		regs, err := m.client.ReadRegisters(addr, uint16(point.Length), modbus.HOLDING_REGISTER)
		if err != nil {
			return fmt.Errorf("modbus ReadRegisters(holding) addr=%d len=%d: %w", addr, point.Length, err)
		}
		point.Value = regs
	}

	return nil
}

// ReadBatch reads multiple Modbus resources, grouped by primaryTable for efficiency.
// Each point's Value is set directly.
func (m *ModbusClient) ReadBatch(points []*model.Resource) error {
	if len(points) == 0 {
		return nil
	}

	// Group by table type
	groups := make(map[tableType][]*model.Resource)
	for _, p := range points {
		tt := getTableType(p.Args)
		groups[tt] = append(groups[tt], p)
	}

	for tt, group := range groups {
		if err := m.readBatchGroup(group, tt); err != nil {
			return err
		}
	}

	return nil
}

// readBatchGroup performs a span-based batch read for a group of points sharing the same table type.
func (m *ModbusClient) readBatchGroup(points []*model.Resource, tt tableType) error {
	// 1. Parse addresses and calculate span
	type idxAddr struct {
		idx  int
		addr uint16
		len  int
	}
	var valid []idxAddr

	minAddr := uint16(0xFFFF)
	var maxAddr uint16

	for i, p := range points {
		addr, err := convAddress(p.Address)
		if err != nil {
			return fmt.Errorf("point[%d] %s: %w", i, p.Name, err)
		}
		endAddr := addr + uint16(p.Length) - 1
		valid = append(valid, idxAddr{idx: i, addr: addr, len: p.Length})
		if addr < minAddr {
			minAddr = addr
		}
		if endAddr > maxAddr {
			maxAddr = endAddr
		}
	}

	count := uint16(0)
	if len(valid) > 0 {
		count = (maxAddr - minAddr) + 1
	}

	if count > 125 {
		return fmt.Errorf("address span too large (%d > 125), requires chunking", count)
	}

	// 2. Perform batch read based on table type
	switch tt {
	case tableCoils:
		coils, err := m.client.ReadCoils(minAddr, count)
		if err != nil {
			return fmt.Errorf("batch ReadCoils addr=%d count=%d: %w", minAddr, count, err)
		}
		for _, v := range valid {
			offset := v.addr - minAddr
			if int(offset)+v.len <= len(coils) {
				points[v.idx].Value = coils[offset : offset+uint16(v.len)]
			}
		}
	default: // holding / input registers
		regType := modbus.HOLDING_REGISTER
		if tt == tableInputRegisters {
			regType = modbus.INPUT_REGISTER
		}
		regs, err := m.client.ReadRegisters(minAddr, count, regType)
		if err != nil {
			return fmt.Errorf("batch ReadRegisters addr=%d count=%d: %w", minAddr, count, err)
		}
		for _, v := range valid {
			offset := v.addr - minAddr
			if int(offset)+v.len <= len(regs) {
				points[v.idx].Value = regs[offset : offset+uint16(v.len)]
			}
		}
	}

	return nil
}

// BytesFromRegs packs a slice of uint16 registers into big-endian []byte.
// Each register produces 2 bytes (MSB first).
func BytesFromRegs(regs []uint16) []byte {
	buf := make([]byte, len(regs)*2)
	for i, r := range regs {
		binary.BigEndian.PutUint16(buf[i*2:], r)
	}
	return buf
}
