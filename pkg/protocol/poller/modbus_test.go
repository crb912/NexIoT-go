// Package poller provides unit tests for the Modbus protocol poller.
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

// ─── Mock implementation ──────────────────────────────────────────────

// mockModbusClient implements the Client interface.
type mockModbusClient struct {
	openErr  error
	closeErr error

	readRegs    []uint16
	readRegsErr error

	readCoils    []bool
	readCoilsErr error

	readDiscretes    []bool
	readDiscretesErr error

	openCalled  bool
	closeCalled bool

	readCalls          []readCall
	readCoilsCalls     []readCoilCall
	readDiscretesCalls []readDiscreteCall
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

type readDiscreteCall struct {
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
	return m.readRegs, m.readRegsErr
}

func (m *mockModbusClient) ReadCoils(address, quantity uint16) ([]bool, error) {
	m.readCoilsCalls = append(m.readCoilsCalls, readCoilCall{address, quantity})
	return m.readCoils, m.readCoilsErr
}

func (m *mockModbusClient) ReadDiscreteInputs(address, quantity uint16) ([]bool, error) {
	m.readDiscretesCalls = append(m.readDiscretesCalls, readDiscreteCall{address, quantity})
	return m.readDiscretes, m.readDiscretesErr
}

// Compile-time check: mockModbusClient satisfies Client.
var _ Client = (*mockModbusClient)(nil)

// ─── Helpers ──────────────────────────────────────────────────────────

func newMockedModbusClient(mock Client) *ModbusClient {
	return &ModbusClient{
		EndPoint:     "tcp://127.0.0.1:502",
		ProtocolType: protocol.ModbusTCP,
		Timeout:      1 * time.Second,
		connected:    true,
		client:       mock,
	}
}

// newTestResource returns a *protocol.Resource for use in ReadSingle.
// For ReadBatch, callers should dereference: *newTestResource(...).
func newTestResource(addr float64, length uint16, primaryTable string) *protocol.Resource {
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

// ═══════════════════════════════════════════════════════════════════════
// getTableType
// ═══════════════════════════════════════════════════════════════════════

func TestGetTableType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args map[string]any
		want tableType
	}{
		{name: "nil args defaults to HOLDING_REGISTERS", args: nil, want: tableHoldingRegisters},
		{name: "empty args defaults to HOLDING_REGISTERS", args: map[string]any{}, want: tableHoldingRegisters},
		{name: "no primaryTable key defaults", args: map[string]any{"other": "x"}, want: tableHoldingRegisters},
		{name: "COILS", args: map[string]any{"primaryTable": "COILS"}, want: tableCoils},
		{name: "DISCRETES", args: map[string]any{"primaryTable": "DISCRETES"}, want: tableDiscretes},
		{name: "HOLDING_REGISTERS", args: map[string]any{"primaryTable": "HOLDING_REGISTERS"}, want: tableHoldingRegisters},
		{name: "INPUT_REGISTERS", args: map[string]any{"primaryTable": "INPUT_REGISTERS"}, want: tableInputRegisters},
		{name: "unknown value falls back", args: map[string]any{"primaryTable": "BAD_TABLE"}, want: tableHoldingRegisters},
		{name: "non-string value falls back", args: map[string]any{"primaryTable": 42}, want: tableHoldingRegisters},
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

// ═══════════════════════════════════════════════════════════════════════
// calculateBatchSpan
// ═══════════════════════════════════════════════════════════════════════

func TestCalculateBatchSpan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		points  []protocol.Resource
		wantMin uint16
		wantQty uint16
	}{
		{
			name: "single point",
			points: []protocol.Resource{
				{Address: float64(10), Length: 2},
			},
			wantMin: 10,
			wantQty: 2,
		},
		{
			name: "contiguous points",
			points: []protocol.Resource{
				{Address: float64(1), Length: 1},
				{Address: float64(2), Length: 1},
				{Address: float64(3), Length: 1},
			},
			wantMin: 1,
			wantQty: 3,
		},
		{
			name: "sparse points",
			points: []protocol.Resource{
				{Address: float64(5), Length: 1},
				{Address: float64(1), Length: 2},
				{Address: float64(9), Length: 1},
			},
			wantMin: 1,
			wantQty: 9, // endAddr max: 9+1=10, quantity: 10-1=9
		},
		{
			name: "multi-length resources",
			points: []protocol.Resource{
				{Address: float64(100), Length: 4},
				{Address: float64(104), Length: 2},
			},
			wantMin: 100,
			wantQty: 6, // endAddr max: 104+2=106, quantity: 106-100=6
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			minAddr, quantity := calculateBatchSpan(tt.points)
			if minAddr != tt.wantMin {
				t.Errorf("calculateBatchSpan() minAddr = %d, want %d", minAddr, tt.wantMin)
			}
			if quantity != tt.wantQty {
				t.Errorf("calculateBatchSpan() quantity = %d, want %d", quantity, tt.wantQty)
			}
		})
	}
}

func TestCalculateBatchSpan_Empty(t *testing.T) {
	t.Parallel()
	// Degenerate case: empty input. minAddr stays at 0xFFFF, quantity is maxEnd - minAddr = 0 - 0xFFFF = 1 (underflow).
	points := []protocol.Resource{}
	minAddr, quantity := calculateBatchSpan(points)
	// Document current behavior — caller should guard against empty input.
	t.Logf("calculateBatchSpan on empty: minAddr=%d (0x%04X), quantity=%d", minAddr, minAddr, quantity)
	_ = minAddr
	_ = quantity
}

// ═══════════════════════════════════════════════════════════════════════
// extractValue
// ═══════════════════════════════════════════════════════════════════════

func TestExtractValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rawData any
		resType string
		length  uint16
		want    any
	}{
		{
			name:    "uint16 length 1 returns bare value",
			rawData: []uint16{0xBEEF},
			length:  1,
			want:    uint16(0xBEEF),
		},
		{
			name:    "uint16 length > 1 returns slice",
			rawData: []uint16{0x0001, 0x0002},
			length:  2,
			want:    []uint16{0x0001, 0x0002},
		},
		{
			name:    "bool length 1 returns bare value",
			rawData: []bool{true},
			length:  1,
			want:    true,
		},
		{
			name:    "bool length > 1 returns slice",
			rawData: []bool{true, false, true},
			length:  3,
			want:    []bool{true, false, true},
		},
		{
			name:    "unexpected type length 1 returns rawData",
			rawData: "hello",
			length:  1,
			want:    "hello",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := extractValue(tt.rawData, tt.resType, tt.length)
			if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", tt.want) {
				t.Errorf("extractValue() = %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════
// assignResValues
// ═══════════════════════════════════════════════════════════════════════

func TestAssignResValues_Uint16(t *testing.T) {
	t.Parallel()

	points := []protocol.Resource{
		{Address: float64(10), Length: 1},
		{Address: float64(12), Length: 2},
	}
	data := []uint16{0xAAAA, 0xBBBB, 0xCCCC, 0xDDDD}

	assignResValues(points, 10, data)

	// Point at addr 10: offset=0, length=1 → extractValue → bare uint16
	if v, ok := points[0].Value.(uint16); !ok || v != 0xAAAA {
		t.Errorf("points[0].Value = %v (%T), want uint16(0xAAAA)", points[0].Value, points[0].Value)
	}
	// Point at addr 12: offset=2, length=2 → extractValue → []uint16{0xCCCC, 0xDDDD}
	slice, ok := points[1].Value.([]uint16)
	if !ok || len(slice) != 2 || slice[0] != 0xCCCC || slice[1] != 0xDDDD {
		t.Errorf("points[1].Value = %v (%T), want []uint16{0xCCCC, 0xDDDD}", points[1].Value, points[1].Value)
	}
}

func TestAssignResValues_Bool(t *testing.T) {
	t.Parallel()

	points := []protocol.Resource{
		{Address: float64(1), Length: 1},
		{Address: float64(3), Length: 1},
	}
	data := []bool{true, false, true, false}

	assignResValues(points, 1, data)

	if v, ok := points[0].Value.(bool); !ok || !v {
		t.Errorf("points[0].Value = %v (%T), want bool(true)", points[0].Value, points[0].Value)
	}
	if v, ok := points[1].Value.(bool); !ok || !v {
		t.Errorf("points[1].Value = %v (%T), want bool(true)", points[1].Value, points[1].Value)
	}
}

func TestAssignResValues_OutOfRange(t *testing.T) {
	t.Parallel()

	// Address 15 with offset 5 but data only has 3 elements → safety check skips.
	points := []protocol.Resource{
		{Address: float64(15), Length: 3},
	}
	data := []uint16{0x0001, 0x0002}

	// Should not panic.
	assignResValues(points, 10, data)

	if points[0].Value != nil {
		t.Errorf("points[0].Value should be nil (out of range), got %v", points[0].Value)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// GetProtocolType
// ═══════════════════════════════════════════════════════════════════════

func TestGetProtocolType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		pt   protocol.ProtocolType
	}{
		{name: "ModbusTCP", pt: protocol.ModbusTCP},
		{name: "ModbusRTU", pt: protocol.ModbusRTU},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mc := &ModbusClient{ProtocolType: tt.pt}
			got := mc.GetProtocolType()
			if got != tt.pt {
				t.Errorf("GetProtocolType() = %q, want %q", got, tt.pt)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Connect
// ═══════════════════════════════════════════════════════════════════════

func TestConnect_OpenError(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{openErr: errors.New("connection refused")}
	mc := newMockedModbusClient(mock)
	mc.connected = false

	err := mc.Connect()
	if err == nil {
		t.Fatal("expected error from Connect(), got nil")
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("error = %q, want it to contain 'connection refused'", err.Error())
	}
}

func TestConnect_Success(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)
	mc.connected = false

	if err := mc.Connect(); err != nil {
		t.Fatalf("Connect() unexpected error: %v", err)
	}
	if !mc.connected {
		t.Error("connected should be true after successful Connect()")
	}
	if !mock.openCalled {
		t.Error("Open() was never called on the underlying client")
	}
}

func TestConnect_ReusesExistingClient(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)
	// client already set and connected
	mc.connected = true

	if err := mc.Connect(); err != nil {
		t.Fatalf("Connect() unexpected error: %v", err)
	}
	// newClient() should not be called because m.client != nil.
	// But Open() will still be called.
	if !mock.openCalled {
		t.Error("Open() should still be called when client already exists")
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Disconnect
// ═══════════════════════════════════════════════════════════════════════

func TestDisconnect_NilClient(t *testing.T) {
	t.Parallel()

	mc := &ModbusClient{client: nil, connected: false}
	if err := mc.Disconnect(); err != nil {
		t.Errorf("Disconnect() with nil client should return nil, got: %v", err)
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
	if mc.connected {
		t.Error("connected should be false after Disconnect(), even on close error")
	}
}

func TestDisconnect_Success(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	if err := mc.Disconnect(); err != nil {
		t.Fatalf("Disconnect() unexpected error: %v", err)
	}
	if mc.connected {
		t.Error("connected should be false after Disconnect()")
	}
	if !mock.closeCalled {
		t.Error("Close() was never called on the underlying client")
	}
}

// ═══════════════════════════════════════════════════════════════════════
// IsConnected
// ═══════════════════════════════════════════════════════════════════════

func TestIsConnected_NilClient(t *testing.T) {
	t.Parallel()

	mc := &ModbusClient{client: nil, connected: false}
	if mc.IsConnected() {
		t.Error("IsConnected() should be false when client is nil")
	}
}

func TestIsConnected_ConnectedFlagFalse(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)
	mc.connected = false

	if mc.IsConnected() {
		t.Error("IsConnected() should be false when connected flag is false")
	}
	// ReadRegisters should NOT be called if the flag is already false.
	if len(mock.readCalls) > 0 {
		t.Error("ReadRegisters should not be called when connected flag is false")
	}
}

func TestIsConnected_Alive(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{
		readRegs: []uint16{0x1234},
	}
	mc := newMockedModbusClient(mock)

	if !mc.IsConnected() {
		t.Error("IsConnected() should be true when ReadRegisters succeeds")
	}
	// Should have called ReadRegisters(1, 1, HOLDING_REGISTER) — the probe.
	if len(mock.readCalls) != 1 {
		t.Fatalf("expected 1 ReadRegisters probe call, got %d", len(mock.readCalls))
	}
	if mock.readCalls[0].address != 1 || mock.readCalls[0].quantity != 1 {
		t.Errorf("probe call: address=%d quantity=%d, want address=1 quantity=1",
			mock.readCalls[0].address, mock.readCalls[0].quantity)
	}
}

func TestIsConnected_ProtocolErrorKeepsAlive(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{
		readRegsErr: errors.New("modbus: exception '2' (illegal data address)"),
	}
	mc := newMockedModbusClient(mock)

	if !mc.IsConnected() {
		t.Error("IsConnected() should remain true on a protocol-level error")
	}
	if !mc.connected {
		t.Error("connected flag should not be cleared on a protocol-level error")
	}
}

func TestIsConnected_TransportErrorsMarkDisconnected(t *testing.T) {
	t.Parallel()

	transportErrors := []string{
		"connection refused",
		"broken pipe",
		"closed network",
		"no such file or directory",
		"no such device",
	}

	for _, errMsg := range transportErrors {
		errMsg := errMsg
		t.Run(errMsg, func(t *testing.T) {
			t.Parallel()

			mock := &mockModbusClient{readRegsErr: fmt.Errorf("dial tcp: %s", errMsg)}
			mc := newMockedModbusClient(mock)

			if mc.IsConnected() {
				t.Errorf("IsConnected() should be false for transport error: %q", errMsg)
			}
			if mc.connected {
				t.Errorf("connected flag should be cleared for transport error: %q", errMsg)
			}
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════
// ReadSingle
// ═══════════════════════════════════════════════════════════════════════

func TestReadSingle_HoldingRegister_Length1(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0xBEEF}}
	mc := newMockedModbusClient(mock)

	// Address float64(1) → uint16(1)-1 = 0 (protocol address)
	point := newTestResource(float64(1), 1, "HOLDING_REGISTERS")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}

	// Length=1: extractValue returns bare uint16.
	v, ok := point.Value.(uint16)
	if !ok {
		t.Fatalf("point.Value should be uint16 (bare value for length=1), got %T", point.Value)
	}
	if v != 0xBEEF {
		t.Errorf("point.Value = 0x%04X, want 0xBEEF", v)
	}

	// Verify protocol call: address should be 0 (1→0 after -1).
	if len(mock.readCalls) != 1 {
		t.Fatalf("expected 1 ReadRegisters call, got %d", len(mock.readCalls))
	}
	if mock.readCalls[0].address != 0 {
		t.Errorf("ReadRegisters address = %d, want 0 (1-based 1 → 0-based 0)", mock.readCalls[0].address)
	}
	if mock.readCalls[0].quantity != 1 {
		t.Errorf("ReadRegisters quantity = %d, want 1", mock.readCalls[0].quantity)
	}
	if mock.readCalls[0].registerType != modbus.HOLDING_REGISTER {
		t.Errorf("ReadRegisters registerType = %v, want HOLDING_REGISTER", mock.readCalls[0].registerType)
	}
}

func TestReadSingle_HoldingRegister_LengthMulti(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0x00FF, 0x0100}}
	mc := newMockedModbusClient(mock)

	// Length > 1: extractValue returns the raw slice.
	point := newTestResource(float64(10), 2, "HOLDING_REGISTERS")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}

	regs, ok := point.Value.([]uint16)
	if !ok {
		t.Fatalf("point.Value should be []uint16 for length>1, got %T", point.Value)
	}
	if len(regs) != 2 || regs[0] != 0x00FF || regs[1] != 0x0100 {
		t.Errorf("point.Value = %v, want [0x00FF 0x0100]", regs)
	}

	// Address float64(10) → uint16(10)-1 = 9.
	if mock.readCalls[0].address != 9 {
		t.Errorf("ReadRegisters address = %d, want 9", mock.readCalls[0].address)
	}
}

func TestReadSingle_Coils_Length1(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readCoils: []bool{true}}
	mc := newMockedModbusClient(mock)

	// Address float64(8) → uint16(8)-1 = 7.
	point := newTestResource(float64(8), 1, "COILS")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}

	v, ok := point.Value.(bool)
	if !ok {
		t.Fatalf("point.Value should be bool (bare value for length=1), got %T", point.Value)
	}
	if !v {
		t.Error("point.Value should be true")
	}

	if len(mock.readCoilsCalls) != 1 {
		t.Fatalf("expected 1 ReadCoils call, got %d", len(mock.readCoilsCalls))
	}
	if mock.readCoilsCalls[0].address != 7 {
		t.Errorf("ReadCoils address = %d, want 7", mock.readCoilsCalls[0].address)
	}
	if mock.readCoilsCalls[0].quantity != 1 {
		t.Errorf("ReadCoils quantity = %d, want 1", mock.readCoilsCalls[0].quantity)
	}
}

func TestReadSingle_Coils_LengthMulti(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readCoils: []bool{true, false, true}}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(1), 3, "COILS")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}

	coils, ok := point.Value.([]bool)
	if !ok {
		t.Fatalf("point.Value should be []bool for length>1, got %T", point.Value)
	}
	if len(coils) != 3 || coils[0] != true || coils[1] != false || coils[2] != true {
		t.Errorf("point.Value = %v, want [true false true]", coils)
	}
}

func TestReadSingle_InputRegister(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0xABCD, 0xEF01}}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(1), 2, "INPUT_REGISTERS")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}

	regs, ok := point.Value.([]uint16)
	if !ok {
		t.Fatalf("point.Value should be []uint16, got %T", point.Value)
	}
	if len(regs) != 2 || regs[0] != 0xABCD || regs[1] != 0xEF01 {
		t.Errorf("point.Value = %v, want [0xABCD 0xEF01]", regs)
	}

	if mock.readCalls[0].registerType != modbus.INPUT_REGISTER {
		t.Errorf("registerType = %v, want INPUT_REGISTER", mock.readCalls[0].registerType)
	}
}

func TestReadSingle_DiscreteInputs(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readDiscretes: []bool{false, true, false}}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(5), 3, "DISCRETES")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}

	discs, ok := point.Value.([]bool)
	if !ok {
		t.Fatalf("point.Value should be []bool, got %T", point.Value)
	}
	if len(discs) != 3 || discs[1] != true {
		t.Errorf("point.Value = %v, want [false true false]", discs)
	}

	if len(mock.readDiscretesCalls) != 1 {
		t.Fatalf("expected 1 ReadDiscreteInputs call, got %d", len(mock.readDiscretesCalls))
	}
	// Address float64(5) → uint16(5)-1 = 4.
	if mock.readDiscretesCalls[0].address != 4 {
		t.Errorf("ReadDiscreteInputs address = %d, want 4", mock.readDiscretesCalls[0].address)
	}
}

func TestReadSingle_DefaultToHoldingRegister(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0x0042}}
	mc := newMockedModbusClient(mock)

	// No primaryTable → defaults to HOLDING_REGISTERS.
	point := newTestResource(float64(10), 1, "")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}
	if mock.readCalls[0].registerType != modbus.HOLDING_REGISTER {
		t.Errorf("should default to HOLDING_REGISTER, got %v", mock.readCalls[0].registerType)
	}
}

func TestReadSingle_InvalidAddressType_Panics(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	point := &protocol.Resource{
		Name:    "bad-addr",
		Address: true, // bool is not float64 — type assertion panics.
		Length:  1,
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("ReadSingle() should panic on non-float64 address type")
		}
	}()

	_ = mc.ReadSingle(point)
}

func TestReadSingle_DeviceError(t *testing.T) {
	t.Parallel()

	deviceErr := errors.New("timeout waiting for device")
	mock := &mockModbusClient{readRegsErr: deviceErr}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(1), 1, "HOLDING_REGISTERS")
	err := mc.ReadSingle(point)

	if err == nil {
		t.Fatal("ReadSingle() should propagate device errors")
	}
	if !strings.Contains(err.Error(), "timeout") {
		t.Errorf("error should contain 'timeout', got: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// ReadBatch
// ═══════════════════════════════════════════════════════════════════════

func TestReadBatch_EmptyInput(t *testing.T) {
	// NOT t.Parallel() — uses recover() which interacts with t.

	mock := &mockModbusClient{readRegs: []uint16{0x0000}}
	mc := newMockedModbusClient(mock)

	// The implementation accesses points[0].Args without checking len(points),
	// which panics on empty input. This test documents that behavior.
	points := []protocol.Resource{}

	defer func() {
		if r := recover(); r == nil {
			t.Error("ReadBatch([]) should panic because points[0].Args is accessed on empty slice")
		}
	}()

	_ = mc.ReadBatch(points)
}

func TestReadBatch_HoldingRegisters_SinglePoint(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0x00FF}}
	mc := newMockedModbusClient(mock)

	points := []protocol.Resource{
		*newTestResource(float64(1), 1, "HOLDING_REGISTERS"),
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	// Length=1 → bare uint16.
	v, ok := points[0].Value.(uint16)
	if !ok {
		t.Fatalf("point[0].Value should be uint16, got %T", points[0].Value)
	}
	if v != 0x00FF {
		t.Errorf("point[0].Value = 0x%04X, want 0x00FF", v)
	}

	// minAddr=1 → read(0, 1, HOLDING_REGISTERS)
	if mock.readCalls[0].address != 0 {
		t.Errorf("ReadRegisters address = %d, want 0", mock.readCalls[0].address)
	}
	if mock.readCalls[0].quantity != 1 {
		t.Errorf("ReadRegisters quantity = %d, want 1", mock.readCalls[0].quantity)
	}
}

func TestReadBatch_HoldingRegisters_Contiguous(t *testing.T) {
	t.Parallel()

	regs := []uint16{0x0001, 0x0002, 0x0003}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	points := []protocol.Resource{
		*newTestResource(float64(1), 1, "HOLDING_REGISTERS"),
		*newTestResource(float64(2), 1, "HOLDING_REGISTERS"),
		*newTestResource(float64(3), 1, "HOLDING_REGISTERS"),
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	// One physical read covers the entire range [1,2,3] → read(0, 3, HOLDING_REGISTERS).
	if len(mock.readCalls) != 1 {
		t.Errorf("expected 1 batch ReadRegisters call, got %d", len(mock.readCalls))
	}
	if mock.readCalls[0].address != 0 {
		t.Errorf("ReadRegisters address = %d, want 0", mock.readCalls[0].address)
	}
	if mock.readCalls[0].quantity != 3 {
		t.Errorf("ReadRegisters quantity = %d, want 3", mock.readCalls[0].quantity)
	}

	// Each point gets its bare uint16 value (length=1).
	wantVals := []uint16{0x0001, 0x0002, 0x0003}
	for i, want := range wantVals {
		v, ok := points[i].Value.(uint16)
		if !ok {
			t.Errorf("points[%d].Value should be uint16, got %T", i, points[i].Value)
			continue
		}
		if v != want {
			t.Errorf("points[%d].Value = %d, want %d", i, v, want)
		}
	}
}

func TestReadBatch_HoldingRegisters_Sparse(t *testing.T) {
	t.Parallel()

	// Addresses 1 and 5 → span covers [1,2,3,4,5] → 5 registers.
	regs := []uint16{0xAAAA, 0x0000, 0x0000, 0x0000, 0xBBBB}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	points := []protocol.Resource{
		*newTestResource(float64(1), 1, "HOLDING_REGISTERS"),
		*newTestResource(float64(5), 1, "HOLDING_REGISTERS"),
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	// Read span: minAddr=1, maxEnd=6, quantity=5 → read(0, 5, ...)
	if mock.readCalls[0].quantity != 5 {
		t.Errorf("ReadRegisters quantity = %d, want 5", mock.readCalls[0].quantity)
	}

	v0, _ := points[0].Value.(uint16)
	v1, _ := points[1].Value.(uint16)
	if v0 != 0xAAAA {
		t.Errorf("address 1 value = 0x%04X, want 0xAAAA", v0)
	}
	if v1 != 0xBBBB {
		t.Errorf("address 5 value = 0x%04X, want 0xBBBB", v1)
	}
}

func TestReadBatch_HoldingRegisters_MultiLength(t *testing.T) {
	t.Parallel()

	regs := []uint16{0x1111, 0x2222, 0x3333, 0x4444}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	points := []protocol.Resource{
		*newTestResource(float64(1), 2, "HOLDING_REGISTERS"), // offset 0, len 2
		*newTestResource(float64(3), 2, "HOLDING_REGISTERS"), // offset 2, len 2
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	// Point 0: length=2 → extractValue returns []uint16.
	s0, ok := points[0].Value.([]uint16)
	if !ok || len(s0) != 2 || s0[0] != 0x1111 || s0[1] != 0x2222 {
		t.Errorf("points[0].Value = %v (%T), want [0x1111 0x2222]", points[0].Value, points[0].Value)
	}

	s1, ok := points[1].Value.([]uint16)
	if !ok || len(s1) != 2 || s1[0] != 0x3333 || s1[1] != 0x4444 {
		t.Errorf("points[1].Value = %v (%T), want [0x3333 0x4444]", points[1].Value, points[1].Value)
	}
}

func TestReadBatch_Coils(t *testing.T) {
	t.Parallel()

	coils := []bool{true, false, true, false, true}
	mock := &mockModbusClient{readCoils: coils}
	mc := newMockedModbusClient(mock)

	// 1-based addresses >= 1 to avoid underflow.
	points := []protocol.Resource{
		*newTestResource(float64(1), 1, "COILS"), // offset 0
		*newTestResource(float64(3), 1, "COILS"), // offset 2
		*newTestResource(float64(5), 1, "COILS"), // offset 4
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	if len(mock.readCoilsCalls) != 1 {
		t.Errorf("expected 1 ReadCoils call, got %d", len(mock.readCoilsCalls))
	}
	// minAddr=1 → read(0, 5, COILS)
	if mock.readCoilsCalls[0].address != 0 {
		t.Errorf("ReadCoils address = %d, want 0", mock.readCoilsCalls[0].address)
	}
	if mock.readCoilsCalls[0].quantity != 5 {
		t.Errorf("ReadCoils quantity = %d, want 5", mock.readCoilsCalls[0].quantity)
	}

	// Each length=1 → bare bool.
	for i, want := range []bool{true, true, true} {
		v, ok := points[i].Value.(bool)
		if !ok {
			t.Errorf("points[%d].Value should be bool, got %T", i, points[i].Value)
			continue
		}
		if v != want {
			t.Errorf("points[%d].Value = %v, want %v", i, v, want)
		}
	}
}

func TestReadBatch_DiscreteInputs(t *testing.T) {
	t.Parallel()

	d := []bool{false, true, false, true}
	mock := &mockModbusClient{readDiscretes: d}
	mc := newMockedModbusClient(mock)

	points := []protocol.Resource{
		*newTestResource(float64(1), 2, "DISCRETES"), // offset 0, len 2
		*newTestResource(float64(3), 2, "DISCRETES"), // offset 2, len 2
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	if len(mock.readDiscretesCalls) != 1 {
		t.Errorf("expected 1 ReadDiscreteInputs call, got %d", len(mock.readDiscretesCalls))
	}
	if mock.readDiscretesCalls[0].address != 0 {
		t.Errorf("ReadDiscreteInputs address = %d, want 0", mock.readDiscretesCalls[0].address)
	}
	if mock.readDiscretesCalls[0].quantity != 4 {
		t.Errorf("ReadDiscreteInputs quantity = %d, want 4", mock.readDiscretesCalls[0].quantity)
	}

	// Length=2 → []bool.
	s0, ok := points[0].Value.([]bool)
	if !ok || len(s0) != 2 || s0[0] != false || s0[1] != true {
		t.Errorf("points[0].Value = %v, want [false true]", points[0].Value)
	}
	s1, ok := points[1].Value.([]bool)
	if !ok || len(s1) != 2 || s1[0] != false || s1[1] != true {
		t.Errorf("points[1].Value = %v, want [false true]", points[1].Value)
	}
}

func TestReadBatch_Error(t *testing.T) {
	t.Parallel()

	deviceErr := errors.New("connection lost")
	mock := &mockModbusClient{readRegsErr: deviceErr}
	mc := newMockedModbusClient(mock)

	points := []protocol.Resource{
		*newTestResource(float64(1), 1, "HOLDING_REGISTERS"),
		*newTestResource(float64(2), 1, "HOLDING_REGISTERS"),
	}
	err := mc.ReadBatch(points)

	if err == nil {
		t.Fatal("ReadBatch() should propagate batch read errors")
	}
	if !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("error should contain 'connection lost', got: %v", err)
	}
	// Error message is wrapped: "batch read failed for type HOLDING_REGISTERS: connection lost"
	if !strings.Contains(err.Error(), "batch read failed") {
		t.Errorf("error should be wrapped with 'batch read failed', got: %v", err)
	}
}

func TestReadBatch_MixedTypes_ReadsFirstTable(t *testing.T) {
	t.Parallel()

	// ReadBatch only uses points[0].Args to determine the table type.
	// All points are read as the first point's table type.
	regs := []uint16{0xAAAA, 0xBBBB}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	points := []protocol.Resource{
		*newTestResource(float64(1), 1, "HOLDING_REGISTERS"),
		*newTestResource(float64(2), 1, "COILS"), // COILS ignored; read as HOLDING_REGISTERS
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	// Both points are read via ReadRegisters (first point's table type).
	if len(mock.readCalls) != 1 {
		t.Errorf("expected 1 ReadRegisters call, got %d", len(mock.readCalls))
	}
	if len(mock.readCoilsCalls) != 0 {
		t.Errorf("expected 0 ReadCoils calls, got %d", len(mock.readCoilsCalls))
	}

	v0, _ := points[0].Value.(uint16)
	v1, _ := points[1].Value.(uint16)
	if v0 != 0xAAAA {
		t.Errorf("points[0].Value = 0x%04X, want 0xAAAA", v0)
	}
	if v1 != 0xBBBB {
		t.Errorf("points[1].Value = 0x%04X, want 0xBBBB", v1)
	}
}
