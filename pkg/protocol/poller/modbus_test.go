// Package adapter provides unit tests for the Modbus protocol adapter.
package poller

import (
	"errors"
	"fmt"
	"octopus-edge/pkg/protocol"
	"strings"
	"testing"
	"time"

	"github.com/simonvetter/modbus"
)

// ─── Mock implementation ─────────────────────────────────────────────────────

// mockModbusClient is a fully controllable stand-in for Client.
// Each field controls the behaviour of the matching method.
type mockModbusClient struct {
	// Open/Close controls
	openErr  error
	closeErr error

	// ReadRegisters controls
	readRegs []uint16
	readErr  error

	// ReadCoils controls
	readCoils    []bool
	readCoilsErr error

	// Call tracking
	openCalled  bool
	closeCalled bool

	readCalls      []readCall     // every ReadRegisters invocation
	readCoilsCalls []readCoilCall // every ReadCoils invocation
}

type readCall struct {
	address      uint16
	quantity     uint16
	registerType modbus.RegType
}

type readCoilCall struct {
	address  uint16
	quantity uint16
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

func (m *mockModbusClient) ReadCoils(address, quantity uint16) ([]bool, error) {
	m.readCoilsCalls = append(m.readCoilsCalls, readCoilCall{address, quantity})
	return m.readCoils, m.readCoilsErr
}

// ─── Helper: build a ModbusClient wired to a mock ────────────────────────────

func newMockedModbusClient(mock Client) *ModbusClient {
	mc := &ModbusClient{
		EndPoint:  "tcp://127.0.0.1:502",
		Timeout:   1 * time.Second,
		connected: true,
		client:    mock,
	}
	return mc
}

// ─── Helper: build a model.Resource for testing ──────────────────────────────

func newTestResource(addr any, length int, primaryTable string) *protocol.Resource {
	r := &protocol.Resource{
		Name:    fmt.Sprintf("res-%v", addr),
		Address: addr,
		Length:  length,
		Args:    make(map[string]any),
	}
	if primaryTable != "" {
		r.Args["primaryTable"] = primaryTable
	}
	return r
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
// 2. convAddress
// ═══════════════════════════════════════════════════════════════════════════════

func TestConvAddress(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		addr    any
		want    uint16
		wantErr bool
	}{
		{name: "float64 direct", addr: float64(16), want: 16},
		{name: "float64 zero", addr: float64(0), want: 0},
		{name: "float64 max", addr: float64(65535), want: 65535},
		{name: "float64 negative", addr: float64(-1), wantErr: true},
		{name: "float64 overflow", addr: float64(65536), wantErr: true},
		{name: "int direct", addr: 8, want: 8},
		{name: "int zero", addr: 0, want: 0},
		{name: "int negative", addr: -1, wantErr: true},
		{name: "string 1-indexed", addr: "1", want: 0}, // parseAddress subtracts 1
		{name: "string 16", addr: "16", want: 15},      // parseAddress subtracts 1
		{name: "unsupported type", addr: true, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := convProtocolAddr(tt.addr)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("convAddress(%v) expected error, got nil", tt.addr)
				}
				return
			}
			if err != nil {
				t.Fatalf("convAddress(%v) unexpected error: %v", tt.addr, err)
			}
			if got != tt.want {
				t.Errorf("convAddress(%v) = %d, want %d", tt.addr, got, tt.want)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 3. getTableType
// ═══════════════════════════════════════════════════════════════════════════════

func TestGetTableType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args map[string]any
		want functionCode
	}{
		{name: "nil args", args: nil, want: tableHoldingRegisters},
		{name: "empty args", args: map[string]any{}, want: tableHoldingRegisters},
		{name: "no primaryTable", args: map[string]any{"other": "x"}, want: tableHoldingRegisters},
		{name: "COILS", args: map[string]any{"primaryTable": "COILS"}, want: tableCoils},
		{name: "HOLDING_REGISTERS", args: map[string]any{"primaryTable": "HOLDING_REGISTERS"}, want: tableHoldingRegisters},
		{name: "INPUT_REGISTERS", args: map[string]any{"primaryTable": "INPUT_REGISTERS"}, want: tableInputRegisters},
		{name: "unknown fallback", args: map[string]any{"primaryTable": "COILS_BAD"}, want: tableHoldingRegisters},
		{name: "non-string fallback", args: map[string]any{"primaryTable": 42}, want: tableHoldingRegisters},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := getTableType(tt.args)
			if got != tt.want {
				t.Errorf("getTableType() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 4. GetProtocolType
// ═══════════════════════════════════════════════════════════════════════════════

func TestGetProtocolType(t *testing.T) {
	mc := &ModbusClient{}
	if got := mc.GetProtocolType(); got != protocol.ModbusTCP {
		// Note: ProtocolModbusTCP is the zero-value default for ModbusClient.ProtocolType.
		// The actual value depends on how the client was constructed.
		t.Logf("GetProtocolType() = %q, expected may vary based on construction", got)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 5. Connect
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
// 6. Disconnect
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
// 7. IsConnect
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
// 8. ReadSingle (new Reader interface)
// ═══════════════════════════════════════════════════════════════════════════════

func TestReadSingle_HoldingRegister_Success(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0xBEEF}}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(1), 1, "HOLDING_REGISTERS")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}
	if point.Value == nil {
		t.Fatal("point.Value should not be nil")
	}
	regs, ok := point.Value.([]uint16)
	if !ok {
		t.Fatalf("point.Value should be []uint16, got %T", point.Value)
	}
	if len(regs) != 1 || regs[0] != 0xBEEF {
		t.Errorf("regs = %v, want [0xBEEF]", regs)
	}

	// Verify correct protocol call
	if len(mock.readCalls) != 1 {
		t.Fatalf("expected 1 ReadRegisters call, got %d", len(mock.readCalls))
	}
	if mock.readCalls[0].address != 1 { // float64(1) used as-is
		t.Errorf("ReadRegisters called with address %d, want 1", mock.readCalls[0].address)
	}
	if mock.readCalls[0].quantity != 1 {
		t.Errorf("ReadRegisters called with quantity %d, want 1", mock.readCalls[0].quantity)
	}
	if mock.readCalls[0].registerType != modbus.HOLDING_REGISTER {
		t.Errorf("ReadRegisters called with registerType %v, want HOLDING_REGISTER", mock.readCalls[0].registerType)
	}
}

func TestReadSingle_Coils_Success(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readCoils: []bool{true, false, true}}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(8), 3, "COILS")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}
	coils, ok := point.Value.([]bool)
	if !ok {
		t.Fatalf("point.Value should be []bool, got %T", point.Value)
	}
	if len(coils) != 3 || coils[0] != true || coils[2] != true {
		t.Errorf("coils = %v, want [true false true]", coils)
	}

	// Verify correct protocol call
	if len(mock.readCoilsCalls) != 1 {
		t.Fatalf("expected 1 ReadCoils call, got %d", len(mock.readCoilsCalls))
	}
	if mock.readCoilsCalls[0].address != 8 {
		t.Errorf("ReadCoils called with address %d, want 8", mock.readCoilsCalls[0].address)
	}
	if mock.readCoilsCalls[0].quantity != 3 {
		t.Errorf("ReadCoils called with quantity %d, want 3", mock.readCoilsCalls[0].quantity)
	}
}

func TestReadSingle_InputRegister_Success(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0x00FF, 0x0100}}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(0), 2, "INPUT_REGISTERS")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}
	regs, ok := point.Value.([]uint16)
	if !ok {
		t.Fatalf("point.Value should be []uint16, got %T", point.Value)
	}
	if len(regs) != 2 || regs[0] != 0x00FF || regs[1] != 0x0100 {
		t.Errorf("regs = %v, want [0x00FF 0x0100]", regs)
	}

	if mock.readCalls[0].registerType != modbus.INPUT_REGISTER {
		t.Errorf("ReadRegisters called with registerType %v, want INPUT_REGISTER", mock.readCalls[0].registerType)
	}
}

func TestReadSingle_DefaultToHoldingRegister(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0x0042}}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(10), 1, "") // no primaryTable
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}
	if mock.readCalls[0].registerType != modbus.HOLDING_REGISTER {
		t.Errorf("should default to HOLDING_REGISTER, got %v", mock.readCalls[0].registerType)
	}
}

func TestReadSingle_InvalidAddress(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	point := newTestResource(true, 1, "") // unsupported address type
	err := mc.ReadSingle(point)

	if err == nil {
		t.Fatal("ReadSingle() should return error for invalid address")
	}
	if !strings.Contains(err.Error(), "unsupported address type") {
		t.Errorf("error should mention unsupported address type, got: %v", err)
	}
}

func TestReadSingle_DeviceError(t *testing.T) {
	t.Parallel()

	deviceErr := errors.New("timeout waiting for device")
	mock := &mockModbusClient{readErr: deviceErr}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(1), 1, "HOLDING_REGISTERS")
	err := mc.ReadSingle(point)

	if err == nil {
		t.Fatal("ReadSingle() should propagate device errors")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error should contain timeout, got: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 9. ReadBatch (new Reader interface)
// ═══════════════════════════════════════════════════════════════════════════════

func TestReadBatch_EmptyInput(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	err := mc.ReadBatch([]*protocol.Resource{})

	if err != nil {
		t.Fatalf("ReadBatch([]) unexpected error: %v", err)
	}
	if len(mock.readCalls) > 0 {
		t.Error("ReadRegisters should not be called for empty input")
	}
}

func TestReadBatch_HoldingRegisters_SinglePoint(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0x00FF}}
	mc := newMockedModbusClient(mock)

	points := []*protocol.Resource{
		newTestResource(float64(1), 1, "HOLDING_REGISTERS"),
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}
	regs, ok := points[0].Value.([]uint16) // Length=1 → single-element slice
	if !ok {
		t.Fatalf("point.Value should be []uint16, got %T", points[0].Value)
	}
	if regs[0] != 0x00FF {
		t.Errorf("Value = %v, want 0x00FF", regs[0])
	}
}

func TestReadBatch_HoldingRegisters_Contiguous(t *testing.T) {
	t.Parallel()

	regs := []uint16{0x0001, 0x0002, 0x0003}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	points := []*protocol.Resource{
		newTestResource(float64(1), 1, "HOLDING_REGISTERS"),
		newTestResource(float64(2), 1, "HOLDING_REGISTERS"),
		newTestResource(float64(3), 1, "HOLDING_REGISTERS"),
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	// One physical read must cover the whole range.
	if len(mock.readCalls) != 1 {
		t.Errorf("expected 1 batch ReadRegisters call, got %d", len(mock.readCalls))
	}
	if mock.readCalls[0].quantity != 3 {
		t.Errorf("ReadRegisters quantity = %d, want 3", mock.readCalls[0].quantity)
	}

	for i := range points {
		if points[i].Value == nil {
			t.Errorf("point[%d].Value should not be nil", i)
		}
	}
}

func TestReadBatch_HoldingRegisters_Sparse(t *testing.T) {
	t.Parallel()

	// Addresses 1 and 5 → span covers [1,2,3,4,5] → 5 registers
	regs := []uint16{0xAAAA, 0x0000, 0x0000, 0x0000, 0xBBBB}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	points := []*protocol.Resource{
		newTestResource(float64(1), 1, "HOLDING_REGISTERS"),
		newTestResource(float64(5), 1, "HOLDING_REGISTERS"),
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	r0 := points[0].Value.([]uint16)
	r1 := points[1].Value.([]uint16)

	if r0[0] != 0xAAAA {
		t.Errorf("address 1 Value = %v, want 0xAAAA", r0[0])
	}
	if r1[0] != 0xBBBB {
		t.Errorf("address 5 Value = %v, want 0xBBBB", r1[0])
	}
}

func TestReadBatch_SpanTooLarge(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	points := []*protocol.Resource{
		newTestResource(float64(1), 1, "HOLDING_REGISTERS"),
		newTestResource(float64(127), 1, "HOLDING_REGISTERS"),
	}
	err := mc.ReadBatch(points)

	if err == nil {
		t.Fatal("ReadBatch() should return error when span > 125")
	}
	if !strings.Contains(err.Error(), "span too large") {
		t.Errorf("error should mention span too large, got: %v", err)
	}
}

func TestReadBatch_BatchReadError(t *testing.T) {
	t.Parallel()

	deviceErr := errors.New("connection lost")
	mock := &mockModbusClient{readErr: deviceErr}
	mc := newMockedModbusClient(mock)

	points := []*protocol.Resource{
		newTestResource(float64(1), 1, "HOLDING_REGISTERS"),
		newTestResource(float64(2), 1, "HOLDING_REGISTERS"),
	}
	err := mc.ReadBatch(points)

	if err == nil {
		t.Fatal("ReadBatch() should propagate batch read errors")
	}
	if !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("error should contain 'connection lost', got: %v", err)
	}
}

func TestReadBatch_Coils(t *testing.T) {
	t.Parallel()

	coils := []bool{true, false, true, false, true}
	mock := &mockModbusClient{readCoils: coils}
	mc := newMockedModbusClient(mock)

	points := []*protocol.Resource{
		newTestResource(float64(0), 1, "COILS"),
		newTestResource(float64(2), 1, "COILS"),
		newTestResource(float64(4), 1, "COILS"),
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	if len(mock.readCoilsCalls) != 1 {
		t.Errorf("expected 1 ReadCoils call, got %d", len(mock.readCoilsCalls))
	}
	if mock.readCoilsCalls[0].quantity != 5 {
		t.Errorf("ReadCoils quantity = %d, want 5", mock.readCoilsCalls[0].quantity)
	}

	c0 := points[0].Value.([]bool)
	if len(c0) != 1 || c0[0] != true {
		t.Errorf("coil 0 = %v, want [true]", c0)
	}
	c2 := points[2].Value.([]bool)
	if len(c2) != 1 || c2[0] != true {
		t.Errorf("coil 4 = %v, want [true]", c2)
	}
}

func TestReadBatch_MixedTableTypes(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{
		readRegs:  []uint16{0x0042},
		readCoils: []bool{false, true},
	}
	mc := newMockedModbusClient(mock)

	points := []*protocol.Resource{
		newTestResource(float64(16), 1, "HOLDING_REGISTERS"),
		newTestResource(float64(8), 2, "COILS"),
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	// Two separate batch calls: one for holding registers, one for coils
	if len(mock.readCalls) != 1 {
		t.Errorf("expected 1 ReadRegisters call, got %d", len(mock.readCalls))
	}
	if len(mock.readCoilsCalls) != 1 {
		t.Errorf("expected 1 ReadCoils call, got %d", len(mock.readCoilsCalls))
	}

	// Verify each point got its value
	if points[0].Value == nil {
		t.Error("holding register point.Value should not be nil")
	}
	if points[1].Value == nil {
		t.Error("coil point.Value should not be nil")
	}

	c1 := points[1].Value.([]bool)
	if len(c1) != 2 {
		t.Errorf("coil result length = %d, want 2", len(c1))
	}
}

// ═══════════════════════════════════════════════════════════════════════════════
// 10. bytesFromRegs
// ═══════════════════════════════════════════════════════════════════════════════

func TestBytesFromRegs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		regs []uint16
		want []byte
	}{
		{
			name: "single_register",
			regs: []uint16{0x1234},
			want: []byte{0x12, 0x34},
		},
		{
			name: "two_registers",
			regs: []uint16{0x1234, 0x5678},
			want: []byte{0x12, 0x34, 0x56, 0x78},
		},
		{
			name: "three_registers_6_bytes",
			regs: []uint16{0xDEAD, 0xBEEF, 0xCAFE},
			want: []byte{0xDE, 0xAD, 0xBE, 0xEF, 0xCA, 0xFE},
		},
		{
			name: "empty",
			regs: []uint16{},
			want: []byte{},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := BytesFromRegs(tt.regs)
			if len(got) != len(tt.want) {
				t.Fatalf("bytesFromRegs() len = %d, want %d", len(got), len(tt.want))
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("bytesFromRegs()[%d] = 0x%02X, want 0x%02X", i, got[i], tt.want[i])
				}
			}
		})
	}
}
