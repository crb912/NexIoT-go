// Package adapter provides unit tests for the Modbus protocol adapter.
package poller

import (
	"errors"
	"fmt"
	"octopus-edge/pkg/adapter"
	"strings"
	"testing"
	"time"

	"github.com/simonvetter/modbus"
)

// ─── Mock implementation ─────────────────────────────────────────────────────

// mockModbusClient is a fully controllable stand-in for modbusClientIface.
// Each field controls the behaviour of the matching method.
type mockModbusClient struct {
	// Open/Close controls
	openErr  error
	closeErr error

	// ReadRegisters controls
	readRegs []uint16
	readErr  error

	// Call tracking
	openCalled  bool
	closeCalled bool

	readCalls []readCall // every ReadRegisters invocation is recorded here
}

type readCall struct {
	address      uint16
	quantity     uint16
	registerType modbus.RegType
}

func (m *mockModbusClient) Open() error {
	m.openCalled = true
	return m.openErr
}

func (m *mockModbusClient) Close() error {
	m.closeCalled = true
	return m.closeErr
}

func (m *mockModbusClient) ReadRegisters(address, quantity uint16, rt modbus.RegType) ([]uint16, error) {
	m.readCalls = append(m.readCalls, readCall{address, quantity, rt})
	return m.readRegs, m.readErr
}

// ─── Helper: build a ModbusClient wired to a mock ────────────────────────────

func newMockedModbusClient(mock modbusClientIface) *ModbusClient {
	mc := &ModbusClient{
		EndPoint:  "tcp://127.0.0.1:502",
		Timeout:   1 * time.Second,
		connected: true,
		client:    mock,
	}
	return mc
}

// ═══════════════════════════════════════════════════════════════════════════════
// 1. parseAddress
// ═══════════════════════════════════════════════════════════════════════════════

func TestParseAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    uint16
		wantErr bool
		errMsg  string
	}{
		// ── Happy path ────────────────────────────────────────────────────────
		{
			name:  "address_1_maps_to_protocol_0",
			input: "1",
			want:  0x0000,
		},
		{
			name:  "classic_modbus_40001_maps_to_40000",
			input: "40001",
			want:  40000,
		},
		{
			name:  "address_100",
			input: "100",
			want:  99,
		},
		{
			name:  "max_uint16_address",
			input: "65535",
			want:  65534,
		},

		// ── Error path ────────────────────────────────────────────────────────
		{
			name:    "zero_is_invalid",
			input:   "0",
			wantErr: true,
			errMsg:  "address must be > 0",
		},
		{
			name:    "negative_number_string",
			input:   "-1",
			wantErr: true,
			errMsg:  "invalid address format",
		},
		{
			name:    "alphabetic_string",
			input:   "abc",
			wantErr: true,
			errMsg:  "invalid address format",
		},
		{
			name:    "empty_string",
			input:   "",
			wantErr: true,
			errMsg:  "invalid address format",
		},
		{
			name:    "float_string",
			input:   "1.5",
			wantErr: true,
			errMsg:  "invalid address format",
		},
		{
			name:    "hex_string_without_prefix",
			input:   "FF",
			wantErr: true,
			errMsg:  "invalid address format",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseAddress(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseAddress(%q) expected error, got nil", tt.input)
				}
				if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("parseAddress(%q) error = %q, want it to contain %q",
						tt.input, err.Error(), tt.errMsg)
				}
				return
			}

			if err != nil {
				t.Fatalf("parseAddress(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("parseAddress(%q) = %d (0x%04X), want %d (0x%04X)",
					tt.input, got, got, tt.want, tt.want)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 2. parseAndCalculateSpan
// ═══════════════════════════════════════════════════════════════════════════════

func TestParseAndCalculateSpan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		pointIDs     []string
		wantMinAddr  uint16
		wantCount    uint16
		wantValidLen int
		wantErrIDs   []string
	}{
		{
			name:         "single_valid_address",
			pointIDs:     []string{"1"},
			wantMinAddr:  0,
			wantCount:    1,
			wantValidLen: 1,
		},
		{
			name:         "contiguous_addresses_1_to_3",
			pointIDs:     []string{"1", "2", "3"},
			wantMinAddr:  0,
			wantCount:    3,
			wantValidLen: 3,
		},
		{
			name:         "sparse_addresses_span_is_calculated_from_extremes",
			pointIDs:     []string{"1", "5"},
			wantMinAddr:  0, // address "1" → protocol addr 0
			wantCount:    5, // max(4) - min(0) + 1 = 5
			wantValidLen: 2,
		},
		{
			name:         "addresses_40001_to_40003",
			pointIDs:     []string{"40001", "40002", "40003"},
			wantMinAddr:  40000,
			wantCount:    3,
			wantValidLen: 3,
		},
		{
			name:         "all_invalid_addresses",
			pointIDs:     []string{"abc", "0", "xyz"},
			wantMinAddr:  0, // reset when no valid addrs
			wantCount:    0,
			wantValidLen: 0,
			wantErrIDs:   []string{"abc", "0", "xyz"},
		},
		{
			name:         "mixed_valid_and_invalid",
			pointIDs:     []string{"1", "bad", "3"},
			wantMinAddr:  0,
			wantCount:    3, // max(2) - min(0) + 1
			wantValidLen: 2,
			wantErrIDs:   []string{"bad"},
		},
		{
			name:         "empty_slice",
			pointIDs:     []string{},
			wantMinAddr:  0,
			wantCount:    0,
			wantValidLen: 0,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			minAddr, count, validAddrs, parseErrs := parseAndCalculateSpan(tt.pointIDs)

			if minAddr != tt.wantMinAddr {
				t.Errorf("minAddr = %d, want %d", minAddr, tt.wantMinAddr)
			}
			if count != tt.wantCount {
				t.Errorf("count = %d, want %d", count, tt.wantCount)
			}
			if len(validAddrs) != tt.wantValidLen {
				t.Errorf("validAddrs len = %d, want %d", len(validAddrs), tt.wantValidLen)
			}
			for _, errID := range tt.wantErrIDs {
				if _, ok := parseErrs[errID]; !ok {
					t.Errorf("expected parse error for ID %q, but got none", errID)
				}
			}
		})
	}
}

// ─── Span offset arithmetic ───────────────────────────────────────────────────

// TestParseAndCalculateSpan_OffsetMapping verifies that offset maths used by
// ReadBatch (addr - minAddr) correctly indexes into the register slice.
func TestParseAndCalculateSpan_OffsetMapping(t *testing.T) {
	t.Parallel()

	pointIDs := []string{"3", "5", "7"} // protocol addrs: 2, 4, 6
	minAddr, count, validAddrs, _ := parseAndCalculateSpan(pointIDs)

	if minAddr != 2 {
		t.Fatalf("minAddr = %d, want 2", minAddr)
	}
	if count != 5 { // 6 - 2 + 1
		t.Fatalf("count = %d, want 5", count)
	}

	// Verify each address maps to the correct register-slice offset.
	cases := []struct {
		id     string
		offset uint16
	}{
		{"3", 0}, // proto addr 2 – minAddr 2 = 0
		{"5", 2}, // proto addr 4 – minAddr 2 = 2
		{"7", 4}, // proto addr 6 – minAddr 2 = 4
	}
	for _, c := range cases {
		addr := validAddrs[c.id]
		if addr-minAddr != c.offset {
			t.Errorf("ID %q: offset = %d, want %d", c.id, addr-minAddr, c.offset)
		}
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 3. GetProtocolType
// ═══════════════════════════════════════════════════════════════════════════════

func TestGetProtocolType(t *testing.T) {
	mc := &ModbusClient{}
	if got := mc.GetProtocolType(); got != adapter.ProtocolModbusTCP {
		t.Errorf("GetProtocolType() = %q, want %q", got, adapter.ProtocolModbusTCP)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 4. Connect
// ═══════════════════════════════════════════════════════════════════════════════

func TestConnect_OpenError(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{openErr: errors.New("connection refused")}
	mc := newMockedModbusClient(mock)
	mc.connected = false // start disconnected

	err := mc.Connect()

	if err == nil {
		t.Fatal("expected error from Connect(), got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %q, want it to contain 'connection refused'", err.Error())
	}
}

func TestConnect_SetsConnectedTrue(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{openErr: nil}
	mc := newMockedModbusClient(mock)
	mc.connected = false

	if err := mc.Connect(); err != nil {
		t.Fatalf("Connect() unexpected error: %v", err)
	}
	if !mc.connected {
		t.Error("connected should be true after successful Connect()")
	}
	if !mock.openCalled {
		t.Error("Open() was never called on the underlying connpool")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 5. Disconnect
// ═══════════════════════════════════════════════════════════════════════════════

func TestDisconnect_NilClient(t *testing.T) {
	t.Parallel()

	mc := &ModbusClient{connected: false}
	if err := mc.Disconnect(); err != nil {
		t.Errorf("Disconnect() with nil connpool should return nil, got: %v", err)
	}
	if mc.connected {
		t.Error("connected should be false after Disconnect()")
	}
}

func TestDisconnect_CloseError(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{closeErr: errors.New("broken pipe")}
	mc := newMockedModbusClient(mock)

	err := mc.Disconnect()

	if err == nil {
		t.Fatal("expected error from Disconnect(), got nil")
	}
	if !strings.Contains(err.Error(), "broken pipe") {
		t.Errorf("error = %q, want it to contain 'broken pipe'", err.Error())
	}
}

func TestDisconnect_SetsConnectedFalse(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{closeErr: nil}
	mc := newMockedModbusClient(mock)

	if err := mc.Disconnect(); err != nil {
		t.Fatalf("Disconnect() unexpected error: %v", err)
	}
	if mc.connected {
		t.Error("connected should be false after Disconnect()")
	}
	if !mock.closeCalled {
		t.Error("Close() was never called on the underlying connpool")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 6. IsConnect
// ═══════════════════════════════════════════════════════════════════════════════

func TestIsConnect_NilClient(t *testing.T) {
	t.Parallel()

	mc := &ModbusClient{client: nil, connected: false}
	if mc.IsConnected() {
		t.Error("IsConnect() should be false when connpool is nil")
	}
}

func TestIsConnect_ConnectedFlagFalse(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)
	mc.connected = false

	if mc.IsConnected() {
		t.Error("IsConnect() should be false when connected flag is false")
	}
	// ReadRegisters should NOT be called if the flag is already false.
	if len(mock.readCalls) > 0 {
		t.Error("ReadRegisters should not be called when connected flag is false")
	}
}

func TestIsConnect_TransportAlive(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{
		readRegs: []uint16{0x1234},
		readErr:  nil,
	}
	mc := newMockedModbusClient(mock)

	if !mc.IsConnected() {
		t.Error("IsConnect() should be true when ReadRegisters succeeds")
	}
}

func TestIsConnect_ProtocolErrorKeepsConnectionAlive(t *testing.T) {
	t.Parallel()
	// A Modbus protocol-level error (e.g., illegal function) must NOT mark the
	// transport as dead — only transport-level errors should do that.
	mock := &mockModbusClient{
		readErr: errors.New("modbus: exception '2' (illegal data address)"),
	}
	mc := newMockedModbusClient(mock)

	if !mc.IsConnected() {
		t.Error("IsConnect() should remain true on a protocol-level error")
	}
	if !mc.connected {
		t.Error("connected flag should not be cleared on a protocol-level error")
	}
}

var transportErrors = []string{
	"connection refused",
	"broken pipe",
	"closed network connection",
	"no such file or directory",
	"no such device",
}

func TestIsConnect_TransportErrors_MarkDisconnected(t *testing.T) {
	t.Parallel()

	for _, errMsg := range transportErrors {
		errMsg := errMsg
		t.Run(errMsg, func(t *testing.T) {
			t.Parallel()

			mock := &mockModbusClient{readErr: fmt.Errorf("dial tcp: %s", errMsg)}
			mc := newMockedModbusClient(mock)

			if mc.IsConnected() {
				t.Errorf("IsConnect() should be false for transport error: %q", errMsg)
			}
			if mc.connected {
				t.Errorf("connected flag should be cleared for transport error: %q", errMsg)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 7. ReadSingle
// ═══════════════════════════════════════════════════════════════════════════════

func TestReadSingle_Success(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0xBEEF}}
	mc := newMockedModbusClient(mock)

	res, err := mc.ReadSingle("1")

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}
	if !res.IsValid {
		t.Error("Resource.IsValid should be true on success")
	}
	if res.RawValue != uint16(0xBEEF) {
		t.Errorf("RawValue = %v, want 0xBEEF", res.RawValue)
	}
	if res.Address != "1" {
		t.Errorf("Address = %q, want %q", res.Address, "1")
	}
	if res.Timestamp.IsZero() {
		t.Error("Timestamp should not be zero")
	}

	// Verify the correct protocol address was sent to the device.
	if len(mock.readCalls) != 1 {
		t.Fatalf("expected 1 ReadRegisters call, got %d", len(mock.readCalls))
	}
	if mock.readCalls[0].address != 0 { // "1" → protocol addr 0
		t.Errorf("ReadRegisters called with address %d, want 0", mock.readCalls[0].address)
	}
	if mock.readCalls[0].quantity != 1 {
		t.Errorf("ReadRegisters called with quantity %d, want 1", mock.readCalls[0].quantity)
	}
}

func TestReadSingle_InvalidAddressFormat(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	res, err := mc.ReadSingle("not-a-number")

	if err == nil {
		t.Fatal("ReadSingle() should return error for invalid address")
	}
	if res.IsValid {
		t.Error("Resource.IsValid should be false on parse error")
	}
	if res.Error == nil {
		t.Error("Resource.Error should be set on parse error")
	}
	if len(mock.readCalls) > 0 {
		t.Error("ReadRegisters should not be called when address is invalid")
	}
}

func TestReadSingle_ZeroAddressError(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	_, err := mc.ReadSingle("0")
	if err == nil {
		t.Fatal("ReadSingle(\"0\") should return an error")
	}
}

func TestReadSingle_DeviceError(t *testing.T) {
	t.Parallel()

	deviceErr := errors.New("timeout waiting for device")
	mock := &mockModbusClient{readErr: deviceErr}
	mc := newMockedModbusClient(mock)

	res, err := mc.ReadSingle("1")

	if err == nil {
		t.Fatal("ReadSingle() should propagate device errors")
	}
	if res.IsValid {
		t.Error("Resource.IsValid should be false on device error")
	}
	if !errors.Is(res.Error, deviceErr) {
		t.Errorf("Resource.Error = %v, want %v", res.Error, deviceErr)
	}
}

func TestReadSingle_EmptyRegistersReturned(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{}} // device returns nothing
	mc := newMockedModbusClient(mock)

	res, err := mc.ReadSingle("1")

	if err != nil {
		t.Fatalf("ReadSingle() unexpected transport error: %v", err)
	}
	if res.IsValid {
		t.Error("Resource.IsValid should be false when no registers returned")
	}
	if res.Error == nil {
		t.Error("Resource.Error should explain why IsValid is false")
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 8. ReadBatch
// ═══════════════════════════════════════════════════════════════════════════════

func TestReadBatch_EmptyInput(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	points, err := mc.ReadBatch([]string{})

	if err != nil {
		t.Fatalf("ReadBatch([]) unexpected error: %v", err)
	}
	if len(points) != 0 {
		t.Errorf("ReadBatch([]) returned %d points, want 0", len(points))
	}
	if len(mock.readCalls) > 0 {
		t.Error("ReadRegisters should not be called for empty input")
	}
}

func TestReadBatch_SingleAddress(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0x00FF}}
	mc := newMockedModbusClient(mock)

	points, err := mc.ReadBatch([]string{"1"})

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}
	if len(points) != 1 {
		t.Fatalf("want 1 point, got %d", len(points))
	}
	if !points[0].IsValid {
		t.Error("point should be valid")
	}
	if points[0].RawValue != uint16(0x00FF) {
		t.Errorf("RawValue = %v, want 0x00FF", points[0].RawValue)
	}
}

func TestReadBatch_ContiguousAddresses(t *testing.T) {
	t.Parallel()

	// Registers for addresses 1, 2, 3 (protocol 0, 1, 2)
	regs := []uint16{0x0001, 0x0002, 0x0003}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	points, err := mc.ReadBatch([]string{"1", "2", "3"})

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("want 3 points, got %d", len(points))
	}

	// One physical read must cover the whole range.
	if len(mock.readCalls) != 1 {
		t.Errorf("expected 1 batch ReadRegisters call, got %d", len(mock.readCalls))
	}
	if mock.readCalls[0].quantity != 3 {
		t.Errorf("ReadRegisters quantity = %d, want 3", mock.readCalls[0].quantity)
	}

	for i, p := range points {
		if !p.IsValid {
			t.Errorf("point[%d] should be valid", i)
		}
	}
}

func TestReadBatch_SparseAddresses_CorrectOffsetMapping(t *testing.T) {
	t.Parallel()

	// Addresses "1" and "3" → protocol 0 and 2.
	// ReadRegisters returns a 3-element slice covering [0,1,2].
	// Index 0 → "1", index 2 → "3".
	regs := []uint16{0xAAAA, 0x0000, 0xBBBB}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	points, err := mc.ReadBatch([]string{"1", "3"})
	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	byAddr := make(map[string]adapter.Resource)
	for _, p := range points {
		byAddr[p.Address] = p
	}

	if byAddr["1"].RawValue != uint16(0xAAAA) {
		t.Errorf("address '1' RawValue = %v, want 0xAAAA", byAddr["1"].RawValue)
	}
	if byAddr["3"].RawValue != uint16(0xBBBB) {
		t.Errorf("address '3' RawValue = %v, want 0xBBBB", byAddr["3"].RawValue)
	}
}

func TestReadBatch_SpanTooLarge(t *testing.T) {
	t.Parallel()

	// Addresses "1" and "127" → span = 127 − 0 + 1 = 127 > 125.
	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	_, err := mc.ReadBatch([]string{"1", "127"})
	if err == nil {
		t.Fatal("ReadBatch() should return error when span > 125")
	}
	if !strings.Contains(err.Error(), "too large") {
		t.Errorf("error = %q, want it to mention 'too large'", err.Error())
	}
}

func TestReadBatch_DeviceReadError(t *testing.T) {
	t.Parallel()

	readErr := errors.New("gateway path unavailable")
	mock := &mockModbusClient{readErr: readErr}
	mc := newMockedModbusClient(mock)

	points, err := mc.ReadBatch([]string{"1", "2"})

	if err == nil {
		t.Fatal("ReadBatch() should propagate device errors")
	}
	if len(points) != 2 {
		t.Fatalf("want 2 points (even on error), got %d", len(points))
	}
	for _, p := range points {
		if p.IsValid {
			t.Errorf("point %q should be invalid on device error", p.Address)
		}
		if p.Error == nil {
			t.Errorf("point %q should carry an error", p.Address)
		}
	}
}

func TestReadBatch_AllInvalidAddresses(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	points, _ := mc.ReadBatch([]string{"abc", "0", "xyz"})

	if len(points) != 3 {
		t.Fatalf("want 3 points, got %d", len(points))
	}
	for _, p := range points {
		if p.IsValid {
			t.Errorf("point %q should be invalid for bad address", p.Address)
		}
		if p.Error == nil {
			t.Errorf("point %q should carry a parse error", p.Address)
		}
	}
	// No physical read should happen when all addresses are invalid.
	if len(mock.readCalls) > 0 {
		t.Error("ReadRegisters should not be called when all addresses are invalid")
	}
}

func TestReadBatch_MixedValidAndInvalid(t *testing.T) {
	t.Parallel()

	regs := []uint16{0x1111, 0x2222}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	points, _ := mc.ReadBatch([]string{"1", "bad", "2"})

	if len(points) != 3 {
		t.Fatalf("want 3 points, got %d", len(points))
	}

	byAddr := make(map[string]adapter.Resource)
	for _, p := range points {
		byAddr[p.Address] = p
	}

	if !byAddr["1"].IsValid {
		t.Error("point '1' should be valid")
	}
	if !byAddr["2"].IsValid {
		t.Error("point '2' should be valid")
	}
	if byAddr["bad"].IsValid {
		t.Error("point 'bad' should be invalid")
	}
	if byAddr["bad"].Error == nil {
		t.Error("point 'bad' should carry a parse error")
	}
}

func TestReadBatch_TimestampsAreConsistent(t *testing.T) {
	t.Parallel()

	// All points in a single batch must share the same timestamp (set once, after
	// the physical read, so they are logically simultaneous).
	regs := []uint16{1, 2, 3}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	before := time.Now()
	points, err := mc.ReadBatch([]string{"1", "2", "3"})
	after := time.Now()

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	ts := points[0].Timestamp
	for i, p := range points {
		if !p.Timestamp.Equal(ts) {
			t.Errorf("point[%d] timestamp differs from point[0]: %v vs %v", i, p.Timestamp, ts)
		}
	}
	if ts.Before(before) || ts.After(after) {
		t.Errorf("timestamp %v is outside the expected range [%v, %v]", ts, before, after)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 9. Thread-safety (race detector)
// ═══════════════════════════════════════════════════════════════════════════════

// Run with: go test -race ./...
func TestIsConnect_ConcurrentAccess(t *testing.T) {
	mock := &mockModbusClient{readRegs: []uint16{0}}
	mc := newMockedModbusClient(mock)

	const workers = 10
	done := make(chan struct{})

	for i := 0; i < workers; i++ {
		go func() {
			mc.IsConnected()
			done <- struct{}{}
		}()
	}
	for i := 0; i < workers; i++ {
		<-done
	}
}

func TestDisconnect_ConcurrentAccess(t *testing.T) {
	const runs = 5
	for i := 0; i < runs; i++ {
		mock := &mockModbusClient{}
		mc := newMockedModbusClient(mock)
		done := make(chan struct{}, 2)

		go func() {
			err := mc.Disconnect()
			if err != nil {
				return
			}
			done <- struct{}{}
		}() //nolint:errcheck
		go func() { mc.IsConnected(); done <- struct{}{} }()

		<-done
		<-done
	}
}
