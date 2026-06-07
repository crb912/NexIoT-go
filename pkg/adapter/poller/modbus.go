// Package poller
// pkg/protocol/modbus.go
package poller

import (
	"fmt"
	"octopus-edge/pkg/adapter"
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
}

// ModbusClient holds network settings for the modbus connpool.
type ModbusClient struct {
	// EndPoint is the target address (e.g., "tcp://192.168.1.100:502" or "rtu:///dev/ttyUSB0")
	EndPoint string
	// Timeout is the max time to wait for a reply
	ProtocolType adapter.ProtocolType
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

	client, err := m.newClient()
	if err != nil {
		return err
	}
	m.client = client

	// The underlying library's Open() method establishes the TCP or RTU connection.
	err = m.client.Open()
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

func (m *ModbusClient) GetProtocolType() adapter.ProtocolType {
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

// ReadSingle reads a single holding register.
func (m *ModbusClient) ReadSingle(pointID string) (adapter.Resource, error) {
	p := adapter.Resource{
		Address:   pointID,
		Timestamp: time.Now(),
	}

	addr, err := parseAddress(pointID)
	if err != nil {
		p.IsValid = false
		p.Error = err
		return p, err
	}

	// Read 1 holding register and returns a []uint16.
	regs, err := m.client.ReadRegisters(addr, 1, modbus.HOLDING_REGISTER)
	if err != nil {
		p.IsValid = false
		p.Error = err
		return p, err // Return both the point (with error inside) and the interface error
	}

	if len(regs) > 0 {
		p.IsValid = true
		// Store the uint16 directly into the 'any' RawValue field.
		p.RawValue = regs[0]
	} else {
		p.IsValid = false
		p.Error = fmt.Errorf("no data returned from device")
	}

	return p, nil
}

// ReadBatch reads multiple holding registers efficiently by calculating the span.
// It accepts semantic points but optimizes physical reads.
func (m *ModbusClient) ReadBatch(pointIDs []string) ([]adapter.Resource, error) {
	var points []adapter.Resource
	if len(pointIDs) == 0 {
		return points, nil
	}

	// 1. Prepare batch context: parse IDs and calculate the memory span.
	minAddr, count, validAddresses, parseErrors := parseAndCalculateSpan(pointIDs)

	// If no valid addresses were found, we only instantiate failed points.
	// But we still delay instantiation to step 3 to keep the workflow unified.
	if count > 125 {
		return nil, fmt.Errorf("address span too large (%d > 125), requires chunking", count)
	}

	// 2. Perform a single batch read.
	var regs []uint16 // Assuming your connpool wrapper returns []uint16
	var readErr error

	if count > 0 {
		regs, readErr = m.client.ReadRegisters(minAddr, count, modbus.HOLDING_REGISTER)
	}

	// 3. Instantiate protocol.Resource objects.
	// We record the exact timestamp when the read operation finished.
	timestamp := time.Now()

	for _, id := range pointIDs {
		p := adapter.Resource{
			Address:   id,
			Timestamp: timestamp,
		}

		// Check if this specific point failed during the parsing stage.
		if parseErr, exists := parseErrors[id]; exists {
			p.IsValid = false
			p.Error = parseErr
			points = append(points, p)
			continue
		}

		// Check if the physical batch read failed (e.g., timeout).
		if readErr != nil {
			p.IsValid = false
			p.Error = fmt.Errorf("batch read failed: %w", readErr)
			points = append(points, p)
			continue
		}

		// Map the raw data back to the point.
		addr := validAddresses[id]
		offset := addr - minAddr

		if int(offset) < len(regs) {
			p.IsValid = true
			p.RawValue = regs[offset]
		} else {
			p.IsValid = false
			p.Error = fmt.Errorf("register index out of bounds")
		}

		points = append(points, p)
	}

	return points, readErr
}

// parseAndCalculateSpan converts string IDs to integers and calculates the batch span.
// It acts as an O(N) sorting mechanism to find the min and max bounds for Modbus.
func parseAndCalculateSpan(pointIDs []string) (minAddr uint16, count uint16, validAddrs map[string]uint16, parseErrs map[string]error) {
	validAddrs = make(map[string]uint16)
	parseErrs = make(map[string]error)

	minAddr = 0xFFFF
	var maxAddr uint16 = 0

	for _, id := range pointIDs {
		addr, err := parseAddress(id)
		if err != nil {
			parseErrs[id] = err
			continue
		}

		validAddrs[id] = addr
		if addr < minAddr {
			minAddr = addr
		}
		if addr > maxAddr {
			maxAddr = addr
		}
	}

	if len(validAddrs) > 0 {
		count = (maxAddr - minAddr) + 1
	} else {
		minAddr = 0 // Reset minAddr if no valid points exist
		count = 0
	}

	return minAddr, count, validAddrs, parseErrs
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
