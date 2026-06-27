// Package poller provides unit tests for the Modbus protocol poller.
package modbus

import (
	"errors"
	"fmt"
	"octopus-edge/pkg/model"
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

	writeSingleRegErr  error
	writeMultiRegErr   error
	writeSingleCoilErr error
	writeMultiCoilErr  error

	openCalled  bool
	closeCalled bool

	readCalls          []readCall
	readCoilsCalls     []readCoilCall
	readDiscretesCalls []readDiscreteCall

	writeSingleRegCalls  []writeSingleRegCall
	writeMultiRegCalls   []writeMultiRegCall
	writeSingleCoilCalls []writeSingleCoilCall
	writeMultiCoilCalls  []writeMultiCoilCall
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

type writeSingleRegCall struct {
	address uint16
	value   uint16
}

type writeMultiRegCall struct {
	address uint16
	values  []uint16
}

type writeSingleCoilCall struct {
	address uint16
	value   bool
}

type writeMultiCoilCall struct {
	address uint16
	values  []bool
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

func (m *mockModbusClient) WriteRegister(address, value uint16) error {
	m.writeSingleRegCalls = append(m.writeSingleRegCalls, writeSingleRegCall{address, value})
	return m.writeSingleRegErr
}

func (m *mockModbusClient) WriteRegisters(address uint16, values []uint16) error {
	m.writeMultiRegCalls = append(m.writeMultiRegCalls, writeMultiRegCall{address, values})
	return m.writeMultiRegErr
}

func (m *mockModbusClient) WriteCoil(address uint16, value bool) error {
	m.writeSingleCoilCalls = append(m.writeSingleCoilCalls, writeSingleCoilCall{address, value})
	return m.writeSingleCoilErr
}

func (m *mockModbusClient) WriteCoils(address uint16, values []bool) error {
	m.writeMultiCoilCalls = append(m.writeMultiCoilCalls, writeMultiCoilCall{address, values})
	return m.writeMultiCoilErr
}

// Compile-time check: mockModbusClient satisfies Client.
var _ Client = (*mockModbusClient)(nil)

// ─── Helpers ──────────────────────────────────────────────────────────

func newMockedModbusClient(mock Client) *ModbusClient {
	return &ModbusClient{
		EndPoint:     "tcp://127.0.0.1:502",
		ProtocolType: model.ModbusTCP,
		Timeout:      1 * time.Second,
		connected:    true,
		client:       mock,
	}
}

// newTestResource returns a *protocoladapter.Resource for use in ReadSingle.
// For ReadBatch, callers should dereference: *newTestResource(...).
// Type is derived from primaryTable for sensible defaults.
func newTestResource(addr float64, length uint16, primaryTable string) *model.Resource {
	r := &model.Resource{
		Name:    fmt.Sprintf("res-%v", addr),
		Address: addr,
		Length:  length,
		Type:    "Uint16", // default for HOLDING_REGISTERS
		Args:    make(map[string]any),
	}
	switch primaryTable {
	case "COILS":
		r.Type = "Bool"
	case "INPUT_REGISTERS":
		r.Type = "Uint16"
	default:
		r.Type = "Uint16"
	}
	if primaryTable != "" {
		r.Args["primaryTable"] = primaryTable
	}
	return r
}

// newTestResourceWithValue returns a *protocoladapter.Resource with a pre-set Value for write tests.
func newTestResourceWithValue(addr float64, length uint16, primaryTable string, value any) *model.Resource {
	r := newTestResource(addr, length, primaryTable)
	r.Value = value
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
		points  []model.Resource
		wantMin uint16
		wantQty uint16
	}{
		{
			name: "single point",
			points: []model.Resource{
				{Address: float64(10), Length: 2},
			},
			wantMin: 10,
			wantQty: 2,
		},
		{
			name: "contiguous points",
			points: []model.Resource{
				{Address: float64(1), Length: 1},
				{Address: float64(2), Length: 1},
				{Address: float64(3), Length: 1},
			},
			wantMin: 1,
			wantQty: 3,
		},
		{
			name: "sparse points",
			points: []model.Resource{
				{Address: float64(5), Length: 1},
				{Address: float64(1), Length: 2},
				{Address: float64(9), Length: 1},
			},
			wantMin: 1,
			wantQty: 9, // endAddr max: 9+1=10, quantity: 10-1=9
		},
		{
			name: "multi-length resources",
			points: []model.Resource{
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
	points := []model.Resource{}
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
			got := extractRegisterValue(tt.rawData, tt.resType, tt.length)
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

	points := []model.Resource{
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

	points := []model.Resource{
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

func TestAssignResValues_OutOfBounds(t *testing.T) {
	t.Parallel()

	points := []model.Resource{
		{Address: float64(1), Length: 100}, // offset + length > len(data)
	}
	data := []uint16{0x0001, 0x0002}

	// Should not panic.
	assignResValues(points, 1, data)

	// Value is not set because the safety check prevents it.
	if points[0].Value != nil {
		t.Errorf("points[0].Value should remain nil for OOB access, got %v", points[0].Value)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// Connect / Disconnect
// ═══════════════════════════════════════════════════════════════════════

func TestConnect_Success(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)
	mc.connected = false

	err := mc.Connect()
	if err != nil {
		t.Fatalf("Connect() unexpected error: %v", err)
	}
	if !mock.openCalled {
		t.Error("Open() should have been called")
	}
	if !mc.connected {
		t.Error("connected should be true after successful Connect")
	}
}

func TestConnect_Error(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{openErr: errors.New("port not found")}
	mc := newMockedModbusClient(mock)
	mc.connected = false

	err := mc.Connect()
	if err == nil {
		t.Fatal("Connect() should return an error")
	}
	if mc.connected {
		t.Error("connected should be false after failed Connect")
	}
}

func TestDisconnect(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	err := mc.Disconnect()
	if err != nil {
		t.Fatalf("Disconnect() unexpected error: %v", err)
	}
	if !mock.closeCalled {
		t.Error("Close() should have been called")
	}
	if mc.connected {
		t.Error("connected should be false after Disconnect")
	}
}

func TestDisconnect_Error(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{closeErr: errors.New("io timeout")}
	mc := newMockedModbusClient(mock)

	err := mc.Disconnect()
	if err == nil {
		t.Fatal("Disconnect() should return an error")
	}
}

// ═══════════════════════════════════════════════════════════════════════
// IsConnected
// ═══════════════════════════════════════════════════════════════════════

func TestIsConnected_NilClient(t *testing.T) {
	t.Parallel()

	mc := &ModbusClient{client: nil, connected: false}
	if mc.IsConnected() {
		t.Error("IsConnected() should be false when protocol is nil")
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

	if len(mock.readCalls) != 1 {
		t.Fatalf("expected 1 ReadRegisters call, got %d", len(mock.readCalls))
	}
	if mock.readCalls[0].address != 0 {
		t.Errorf("ReadRegisters address = %d, want 0", mock.readCalls[0].address)
	}
	if mock.readCalls[0].quantity != 1 {
		t.Errorf("ReadRegisters quantity = %d, want 1", mock.readCalls[0].quantity)
	}

	v, ok := point.Value.(uint16)
	if !ok || v != 0xBEEF {
		t.Errorf("point.Value = 0x%04X (%T), want uint16(0xBEEF)", v, point.Value)
	}
}

func TestReadSingle_HoldingRegister_Length2(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readRegs: []uint16{0xDEAD, 0xBEEF}}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(10), 2, "HOLDING_REGISTERS")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}

	// Address float64(10) → uint16(10)-1 = 9
	if mock.readCalls[0].address != 9 {
		t.Errorf("ReadRegisters address = %d, want 9", mock.readCalls[0].address)
	}
	if mock.readCalls[0].quantity != 2 {
		t.Errorf("ReadRegisters quantity = %d, want 2", mock.readCalls[0].quantity)
	}

	arr, ok := point.Value.([]uint16)
	if !ok || len(arr) != 2 || arr[0] != 0xDEAD || arr[1] != 0xBEEF {
		t.Errorf("point.Value = %v (%T), want []uint16{0xDEAD, 0xBEEF}", arr, point.Value)
	}
}

func TestReadSingle_Coil(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{readCoils: []bool{true, false, true}}
	mc := newMockedModbusClient(mock)

	point := newTestResource(float64(3), 3, "COILS")
	err := mc.ReadSingle(point)

	if err != nil {
		t.Fatalf("ReadSingle() unexpected error: %v", err)
	}

	if len(mock.readCoilsCalls) != 1 {
		t.Fatalf("expected 1 ReadCoils call, got %d", len(mock.readCoilsCalls))
	}
	// Address float64(3) → uint16(3)-1 = 2
	if mock.readCoilsCalls[0].address != 2 {
		t.Errorf("ReadCoils address = %d, want 2", mock.readCoilsCalls[0].address)
	}

	coils, ok := point.Value.([]bool)
	if !ok || len(coils) != 3 || coils[1] != false {
		t.Errorf("point.Value = %v, want [true false true]", coils)
	}
}

func TestReadSingle_InputRegisters(t *testing.T) {
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

	point := &model.Resource{
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

func TestReadBatch_HoldingRegisters(t *testing.T) {
	t.Parallel()

	regs := []uint16{0x0001, 0x0002, 0x0003, 0x0004}
	mock := &mockModbusClient{readRegs: regs}
	mc := newMockedModbusClient(mock)

	points := []model.Resource{
		*newTestResource(float64(1), 1, "HOLDING_REGISTERS"), // offset 0, len 1
		*newTestResource(float64(3), 2, "HOLDING_REGISTERS"), // offset 2, len 2
	}
	err := mc.ReadBatch(points)

	if err != nil {
		t.Fatalf("ReadBatch() unexpected error: %v", err)
	}

	if len(mock.readCalls) != 1 {
		t.Errorf("expected 1 ReadRegisters call, got %d", len(mock.readCalls))
	}
	// minAddr=1 → read(0, 4, HOLDING_REGISTER)
	if mock.readCalls[0].address != 0 {
		t.Errorf("ReadRegisters address = %d, want 0", mock.readCalls[0].address)
	}
	if mock.readCalls[0].quantity != 4 {
		t.Errorf("ReadRegisters quantity = %d, want 4", mock.readCalls[0].quantity)
	}

	// Length=1 → bare uint16.
	v0, ok := points[0].Value.(uint16)
	if !ok || v0 != 0x0001 {
		t.Errorf("points[0].Value = 0x%04X, want 0x0001", v0)
	}
	// Length=2 → []uint16.
	s1, ok := points[1].Value.([]uint16)
	if !ok || len(s1) != 2 || s1[0] != 0x0003 || s1[1] != 0x0004 {
		t.Errorf("points[1].Value = %v, want [0x0003 0x0004]", s1)
	}
}

func TestReadBatch_Coils(t *testing.T) {
	t.Parallel()

	c := []bool{true, false, true, false, true}
	mock := &mockModbusClient{readCoils: c}
	mc := newMockedModbusClient(mock)

	// 1-based addresses >= 1 to avoid underflow.
	points := []model.Resource{
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

	points := []model.Resource{
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

	points := []model.Resource{
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

	points := []model.Resource{
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

// ═══════════════════════════════════════════════════════════════════════
// WriteSingle
// ═══════════════════════════════════════════════════════════════════════

func TestWriteSingle_HoldingRegister(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	point := newTestResourceWithValue(float64(1), 1, "HOLDING_REGISTERS", uint16(0x2A))
	err := mc.WriteSingle(point)

	if err != nil {
		t.Fatalf("WriteSingle() unexpected error: %v", err)
	}

	if len(mock.writeSingleRegCalls) != 1 {
		t.Fatalf("expected 1 WriteSingleRegister call, got %d", len(mock.writeSingleRegCalls))
	}
	// Address float64(1) → uint16(1)-1 = 0
	wantAddr := uint16(0)
	if mock.writeSingleRegCalls[0].address != wantAddr {
		t.Errorf("WriteSingleRegister address = %d, want %d", mock.writeSingleRegCalls[0].address, wantAddr)
	}
	if mock.writeSingleRegCalls[0].value != 0x2A {
		t.Errorf("WriteSingleRegister value = 0x%04X, want 0x2A", mock.writeSingleRegCalls[0].value)
	}
}

func TestWriteSingle_HoldingRegister_StringValue(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	// conv.ToUint16Slice rejects non-uint16/[]uint16 values.
	point := newTestResourceWithValue(float64(8003), 1, "HOLDING_REGISTERS", "4")
	err := mc.WriteSingle(point)

	if err == nil {
		t.Fatal("WriteSingle() should return error for string value")
	}
	if !strings.Contains(err.Error(), "only uint16") {
		t.Errorf("error should mention 'only uint16', got: %v", err)
	}

	// No write call should have been made.
	if len(mock.writeSingleRegCalls) != 0 {
		t.Errorf("expected 0 WriteSingleRegister calls, got %d", len(mock.writeSingleRegCalls))
	}
}

func TestWriteSingle_Coil(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	point := newTestResourceWithValue(float64(10), 1, "COILS", true)
	err := mc.WriteSingle(point)

	if err != nil {
		t.Fatalf("WriteSingle() unexpected error: %v", err)
	}

	if len(mock.writeSingleCoilCalls) != 1 {
		t.Fatalf("expected 1 WriteSingleCoil call, got %d", len(mock.writeSingleCoilCalls))
	}
	// Address float64(10) → 10-1 = 9
	if mock.writeSingleCoilCalls[0].address != 9 {
		t.Errorf("WriteSingleCoil address = %d, want 9", mock.writeSingleCoilCalls[0].address)
	}
	if mock.writeSingleCoilCalls[0].value != true {
		t.Error("WriteSingleCoil value should be true")
	}
}

func TestWriteSingle_DiscreteInputs_ReadOnly(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	point := newTestResourceWithValue(float64(1), 1, "DISCRETES", true)
	err := mc.WriteSingle(point)

	if err == nil {
		t.Fatal("WriteSingle() should return error for discrete inputs")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error should mention 'read-only', got: %v", err)
	}
}

func TestWriteSingle_InputRegisters_ReadOnly(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	point := newTestResourceWithValue(float64(1), 1, "INPUT_REGISTERS", uint16(42))
	err := mc.WriteSingle(point)

	if err == nil {
		t.Fatal("WriteSingle() should return error for input registers")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error should mention 'read-only', got: %v", err)
	}
}

func TestWriteSingle_DeviceError(t *testing.T) {
	t.Parallel()

	deviceErr := errors.New("write timeout")
	mock := &mockModbusClient{writeSingleRegErr: deviceErr}
	mc := newMockedModbusClient(mock)

	point := newTestResourceWithValue(float64(1), 1, "HOLDING_REGISTERS", uint16(5))
	err := mc.WriteSingle(point)

	if err == nil {
		t.Fatal("WriteSingle() should propagate device errors")
	}
	if !strings.Contains(err.Error(), "write timeout") {
		t.Errorf("error should contain 'write timeout', got: %v", err)
	}
}

// ═══════════════════════════════════════════════════════════════════════
// WriteBatch
// ═══════════════════════════════════════════════════════════════════════

func TestWriteBatch_HoldingRegisters(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	points := []model.Resource{
		*newTestResourceWithValue(float64(1), 1, "HOLDING_REGISTERS", uint16(0x10)),
		*newTestResourceWithValue(float64(2), 1, "HOLDING_REGISTERS", uint16(0x20)),
		*newTestResourceWithValue(float64(3), 1, "HOLDING_REGISTERS", uint16(0x30)),
	}
	err := mc.WriteBatch(points)

	if err != nil {
		t.Fatalf("WriteBatch() unexpected error: %v", err)
	}

	if len(mock.writeMultiRegCalls) != 1 {
		t.Fatalf("expected 1 WriteMultipleRegisters call, got %d", len(mock.writeMultiRegCalls))
	}
	// minAddr=1 → protocol address 0
	if mock.writeMultiRegCalls[0].address != 0 {
		t.Errorf("WriteMultipleRegisters address = %d, want 0", mock.writeMultiRegCalls[0].address)
	}
	// quantity = maxEnd(3+1=4) - minAddr(1) = 3
	want := []uint16{0x10, 0x20, 0x30}
	if len(mock.writeMultiRegCalls[0].values) != 3 {
		t.Errorf("WriteMultipleRegisters values len = %d, want 3", len(mock.writeMultiRegCalls[0].values))
	}
	for i, v := range mock.writeMultiRegCalls[0].values {
		if v != want[i] {
			t.Errorf("WriteMultipleRegisters values[%d] = 0x%04X, want 0x%04X", i, v, want[i])
		}
	}
}

func TestWriteBatch_SparseRegisters(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	points := []model.Resource{
		*newTestResourceWithValue(float64(10), 1, "HOLDING_REGISTERS", uint16(0xAA)),
		*newTestResourceWithValue(float64(13), 1, "HOLDING_REGISTERS", uint16(0xDD)),
	}
	err := mc.WriteBatch(points)

	if err != nil {
		t.Fatalf("WriteBatch() unexpected error: %v", err)
	}

	if len(mock.writeMultiRegCalls) != 1 {
		t.Fatalf("expected 1 WriteMultipleRegisters call, got %d", len(mock.writeMultiRegCalls))
	}
	// minAddr=10 → protocol address 9
	if mock.writeMultiRegCalls[0].address != 9 {
		t.Errorf("WriteMultipleRegisters address = %d, want 9", mock.writeMultiRegCalls[0].address)
	}
	// toUint16SliceBatch concatenates point values without zero-filling gaps.
	want := []uint16{0xAA, 0xDD}
	if len(mock.writeMultiRegCalls[0].values) != 2 {
		t.Errorf("WriteMultipleRegisters values len = %d, want 2", len(mock.writeMultiRegCalls[0].values))
	}
	for i, v := range mock.writeMultiRegCalls[0].values {
		if v != want[i] {
			t.Errorf("WriteMultipleRegisters values[%d] = 0x%04X, want 0x%04X", i, v, want[i])
		}
	}
}

func TestWriteBatch_Coils(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	points := []model.Resource{
		*newTestResourceWithValue(float64(1), 1, "COILS", true),
		*newTestResourceWithValue(float64(2), 1, "COILS", false),
		*newTestResourceWithValue(float64(3), 1, "COILS", true),
	}
	err := mc.WriteBatch(points)

	if err != nil {
		t.Fatalf("WriteBatch() unexpected error: %v", err)
	}

	if len(mock.writeMultiCoilCalls) != 1 {
		t.Fatalf("expected 1 WriteMultipleCoils call, got %d", len(mock.writeMultiCoilCalls))
	}
	if mock.writeMultiCoilCalls[0].address != 0 {
		t.Errorf("WriteMultipleCoils address = %d, want 0", mock.writeMultiCoilCalls[0].address)
	}
	want := []bool{true, false, true}
	if len(mock.writeMultiCoilCalls[0].values) != 3 {
		t.Errorf("WriteMultipleCoils values len = %d, want 3", len(mock.writeMultiCoilCalls[0].values))
	}
	for i, v := range mock.writeMultiCoilCalls[0].values {
		if v != want[i] {
			t.Errorf("WriteMultipleCoils values[%d] = %v, want %v", i, v, want[i])
		}
	}
}

func TestWriteBatch_Empty(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	points := []model.Resource{}
	err := mc.WriteBatch(points)

	if err == nil {
		t.Fatal("WriteBatch() should return error for empty input")
	}
	if !strings.Contains(err.Error(), "no points to write") {
		t.Errorf("error should mention 'no points to write', got: %v", err)
	}
}

func TestWriteBatch_Error(t *testing.T) {
	t.Parallel()

	deviceErr := errors.New("modbus: exception '3' (illegal data value)")
	mock := &mockModbusClient{writeMultiRegErr: deviceErr}
	mc := newMockedModbusClient(mock)

	points := []model.Resource{
		*newTestResourceWithValue(float64(1), 1, "HOLDING_REGISTERS", uint16(9999)),
	}
	err := mc.WriteBatch(points)

	if err == nil {
		t.Fatal("WriteBatch() should propagate write errors")
	}
	if !strings.Contains(err.Error(), "illegal data value") {
		t.Errorf("error should contain 'illegal data value', got: %v", err)
	}
}

func TestWriteBatch_DiscreteInputs_ReadOnly(t *testing.T) {
	t.Parallel()

	mock := &mockModbusClient{}
	mc := newMockedModbusClient(mock)

	points := []model.Resource{
		*newTestResourceWithValue(float64(1), 1, "DISCRETES", true),
	}
	err := mc.WriteBatch(points)

	if err == nil {
		t.Fatal("WriteBatch() should return error for discrete inputs")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("error should mention 'read-only', got: %v", err)
	}
}
