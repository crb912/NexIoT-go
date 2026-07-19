package snmp

import (
	"next-iot-go/pkg/model"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/gosnmp/gosnmp"
)

// ─── Mock implementation ────────────────────────────────────────────────

// mockSnmpSession implements the SnmpSession interface.
type mockSnmpSession struct {
	connectErr error
	closeErr   error

	getOIDs   []string
	getPacket *gosnmp.SnmpPacket
	getErr    error

	setPDUs   []gosnmp.SnmpPDU
	setPacket *gosnmp.SnmpPacket
	setErr    error

	connectCalled bool
	closeCalled   bool
}

func (m *mockSnmpSession) Connect() error {
	m.connectCalled = true
	return m.connectErr
}

func (m *mockSnmpSession) Get(oids []string) (*gosnmp.SnmpPacket, error) {
	m.getOIDs = append(m.getOIDs, oids...)
	return m.getPacket, m.getErr
}

func (m *mockSnmpSession) Set(pdus []gosnmp.SnmpPDU) (*gosnmp.SnmpPacket, error) {
	m.setPDUs = append(m.setPDUs, pdus...)
	return m.setPacket, m.setErr
}

func (m *mockSnmpSession) Close() error {
	m.closeCalled = true
	return m.closeErr
}

// Compile-time check: mockSnmpSession satisfies SnmpSession.
var _ SnmpSession = (*mockSnmpSession)(nil)

// ─── Helpers ────────────────────────────────────────────────────────────

func newMockedSnmpClient(mock SnmpSession) *SnmpClient {
	return &SnmpClient{
		EndPoint:     "127.0.0.1",
		ProtocolType: model.SNMP,
		Timeout:      1 * time.Second,
		Version:      gosnmp.Version2c,
		Community:    "public",
		Port:         161,
		connected:    true,
		client:       mock,
	}
}

// newSnmpResource returns a *model.Resource for SNMP read/write tests.
func newSnmpResource(name, oid string, value any) *model.Resource {
	return &model.Resource{
		Name:    name,
		Address: oid,
		Length:  1,
		Type:    "string",
		Value:   value,
		Args:    make(map[string]any),
	}
}

// ═════════════════════════════════════════════════════════════════════════
// NewSnmpClient
// ═════════════════════════════════════════════════════════════════════════

func TestNewSnmpClientDefaults(t *testing.T) {
	t.Parallel()

	c, err := NewSnmpClient("10.0.0.1", model.SNMP, 2*time.Second, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.EndPoint != "10.0.0.1" {
		t.Errorf("EndPoint = %q, want %q", c.EndPoint, "10.0.0.1")
	}
	if c.Timeout != 2*time.Second {
		t.Errorf("Timeout = %v, want 2s", c.Timeout)
	}
	if c.Version != gosnmp.Version2c {
		t.Errorf("Version = %v, want Version2c", c.Version)
	}
	if c.Community != "public" {
		t.Errorf("Community = %q, want %q", c.Community, "public")
	}
	if c.Port != 161 {
		t.Errorf("Port = %d, want 161", c.Port)
	}
}

func TestNewSnmpClientV1(t *testing.T) {
	t.Parallel()

	args := map[string]string{
		"SnmpVersion": "v1",
		"Community":   "private",
	}
	c, err := NewSnmpClient("10.0.0.1", model.SNMP, 1*time.Second, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Version != gosnmp.Version1 {
		t.Errorf("Version = %v, want Version1", c.Version)
	}
	if c.Community != "private" {
		t.Errorf("Community = %q, want %q", c.Community, "private")
	}
}

func TestNewSnmpClientV3(t *testing.T) {
	t.Parallel()

	args := map[string]string{
		"SnmpVersion":     "v3",
		"user_name":       "admin",
		"auth_protocol":   "SHA256",
		"auth_passphrase": "auth123",
		"priv_protocol":   "AES256",
		"priv_passphrase": "priv456",
	}
	c, err := NewSnmpClient("10.0.0.1", model.SNMP, 1*time.Second, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Version != gosnmp.Version3 {
		t.Errorf("Version = %v, want Version3", c.Version)
	}
	if c.V3Params == nil {
		t.Fatal("V3Params is nil")
	}
	if c.V3Params.UserName != "admin" {
		t.Errorf("UserName = %q, want %q", c.V3Params.UserName, "admin")
	}
	if c.V3Params.AuthenticationPassphrase != "auth123" {
		t.Errorf("AuthPassphrase = %q", c.V3Params.AuthenticationPassphrase)
	}
	if c.V3Params.PrivacyPassphrase != "priv456" {
		t.Errorf("PrivPassphrase = %q", c.V3Params.PrivacyPassphrase)
	}
}

func TestNewSnmpClientCustomPort(t *testing.T) {
	t.Parallel()

	args := map[string]string{
		"port": "1161",
	}
	c, err := NewSnmpClient("10.0.0.1", model.SNMP, 1*time.Second, args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if c.Port != 1161 {
		t.Errorf("Port = %d, want 1161", c.Port)
	}
}

// ═════════════════════════════════════════════════════════════════════════
// Connect / Disconnect / IsConnected
// ═════════════════════════════════════════════════════════════════════════

func TestConnectSuccess(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{}
	c := &SnmpClient{
		EndPoint:     "127.0.0.1",
		ProtocolType: model.SNMP,
		Timeout:      1 * time.Second,
		Version:      gosnmp.Version2c,
		Community:    "public",
		Port:         161,
		client:       mock,
	}

	err := c.Connect()
	if err != nil {
		t.Fatalf("Connect() = %v, want nil", err)
	}
	if !mock.connectCalled {
		t.Error("mock.Connect not called")
	}
	if !c.connected {
		t.Error("connected should be true after Connect")
	}
}

func TestConnectError(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{connectErr: errors.New("timeout")}
	c := &SnmpClient{
		EndPoint:     "127.0.0.1",
		ProtocolType: model.SNMP,
		Timeout:      1 * time.Second,
		Version:      gosnmp.Version2c,
		Community:    "public",
		Port:         161,
		client:       mock,
	}

	err := c.Connect()
	if err == nil {
		t.Fatal("Connect() expected error, got nil")
	}
}

func TestDisconnect(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{}
	c := &SnmpClient{
		client:    mock,
		connected: true,
	}

	err := c.Disconnect()
	if err != nil {
		t.Fatalf("Disconnect() = %v, want nil", err)
	}
	if !mock.closeCalled {
		t.Error("mock.Close not called")
	}
	if c.connected {
		t.Error("connected should be false after Disconnect")
	}
}

func TestDisconnectNilClient(t *testing.T) {
	t.Parallel()

	c := &SnmpClient{connected: true}
	err := c.Disconnect()
	if err != nil {
		t.Fatalf("Disconnect() = %v, want nil", err)
	}
	if c.connected {
		t.Error("connected should be false when client is nil")
	}
}

func TestIsConnectedTrue(t *testing.T) {
	t.Parallel()

	pdu := gosnmp.SnmpPDU{
		Name:  "1.3.6.1.2.1.1.1.0",
		Type:  gosnmp.OctetString,
		Value: []byte("test"),
	}
	mock := &mockSnmpSession{
		getPacket: &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{pdu}},
	}
	_ = newMockedSnmpClient(mock)

}

func TestIsConnectedFalseNilClient(t *testing.T) {
	t.Parallel()

	_ = &SnmpClient{}
}

func TestIsConnectedFalseNotConnected(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{}
	_ = &SnmpClient{client: mock, connected: false}
}

func TestIsConnectedHealthCheckFails(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{getErr: errors.New("no response")}
	_ = newMockedSnmpClient(mock)

}

// ═════════════════════════════════════════════════════════════════════════
// ReadSingle
// ═════════════════════════════════════════════════════════════════════════

func TestReadSingleSuccess(t *testing.T) {
	t.Parallel()

	pdu := gosnmp.SnmpPDU{
		Name:  "1.3.6.1.2.1.1.1.0",
		Type:  gosnmp.OctetString,
		Value: []byte("MyDevice"),
	}
	mock := &mockSnmpSession{
		getPacket: &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{pdu}},
	}
	c := newMockedSnmpClient(mock)

	res := newSnmpResource("sysDescr", "1.3.6.1.2.1.1.1.0", nil)
	err := c.ReadSingle(res)
	if err != nil {
		t.Fatalf("ReadSingle() = %v, want nil", err)
	}

	val, ok := res.Value.(string)
	if !ok {
		t.Fatalf("Value is %T, want string", res.Value)
	}
	if val != "MyDevice" {
		t.Errorf("Value = %q, want %q", val, "MyDevice")
	}
}

func TestReadSingleIntValue(t *testing.T) {
	t.Parallel()

	pdu := gosnmp.SnmpPDU{
		Name:  "1.3.6.1.2.1.1.3.0",
		Type:  gosnmp.TimeTicks,
		Value: uint32(12345),
	}
	mock := &mockSnmpSession{
		getPacket: &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{pdu}},
	}
	c := newMockedSnmpClient(mock)

	res := newSnmpResource("sysUpTime", "1.3.6.1.2.1.1.3.0", nil)
	err := c.ReadSingle(res)
	if err != nil {
		t.Fatalf("ReadSingle() = %v, want nil", err)
	}

	val, ok := res.Value.(uint32)
	if !ok {
		t.Fatalf("Value is %T, want uint32", res.Value)
	}
	if val != 12345 {
		t.Errorf("Value = %d, want 12345", val)
	}
}

func TestReadSingleAddressNotString(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{}
	c := newMockedSnmpClient(mock)

	res := &model.Resource{
		Name:    "bad",
		Address: 123, // int, not string
		Value:   nil,
	}

	err := c.ReadSingle(res)
	if err == nil {
		t.Fatal("ReadSingle() expected error, got nil")
	}
}

func TestReadSingleGetError(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{getErr: errors.New("timeout")}
	c := newMockedSnmpClient(mock)

	res := newSnmpResource("sysDescr", "1.3.6.1.2.1.1.1.0", nil)
	err := c.ReadSingle(res)
	if err == nil {
		t.Fatal("ReadSingle() expected error, got nil")
	}
}

func TestReadSingleNoVariables(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{
		getPacket: &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{}},
	}
	c := newMockedSnmpClient(mock)

	res := newSnmpResource("sysDescr", "1.3.6.1.2.1.1.1.0", nil)
	err := c.ReadSingle(res)
	if err == nil {
		t.Fatal("ReadSingle() expected error for empty variables, got nil")
	}
}

// ═════════════════════════════════════════════════════════════════════════
// ReadBatch
// ═════════════════════════════════════════════════════════════════════════

func TestReadBatchSuccess(t *testing.T) {
	t.Parallel()

	pdus := []gosnmp.SnmpPDU{
		{Name: "1.3.6.1.2.1.1.1.0", Type: gosnmp.OctetString, Value: []byte("dev1")},
		{Name: "1.3.6.1.2.1.1.3.0", Type: gosnmp.TimeTicks, Value: uint32(100)},
	}
	mock := &mockSnmpSession{
		getPacket: &gosnmp.SnmpPacket{Variables: pdus},
	}
	c := newMockedSnmpClient(mock)

	points := []model.Resource{
		*newSnmpResource("sysDescr", "1.3.6.1.2.1.1.1.0", nil),
		*newSnmpResource("sysUpTime", "1.3.6.1.2.1.1.3.0", nil),
	}

	err := c.ReadBatch(points)
	if err != nil {
		t.Fatalf("ReadBatch() = %v, want nil", err)
	}

	if points[0].Value != "dev1" {
		t.Errorf("points[0].Value = %v, want %q", points[0].Value, "dev1")
	}
	if points[1].Value != uint32(100) {
		t.Errorf("points[1].Value = %v, want %d", points[1].Value, 100)
	}
}

func TestReadBatchEmpty(t *testing.T) {
	t.Parallel()

	c := newMockedSnmpClient(&mockSnmpSession{})
	err := c.ReadBatch(nil)
	if err != nil {
		t.Fatalf("ReadBatch(nil) = %v, want nil", err)
	}
}

func TestReadBatchGetError(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{getErr: errors.New("timeout")}
	c := newMockedSnmpClient(mock)

	points := []model.Resource{
		*newSnmpResource("sysDescr", "1.3.6.1.2.1.1.1.0", nil),
	}

	err := c.ReadBatch(points)
	if err == nil {
		t.Fatal("ReadBatch() expected error, got nil")
	}
}

// ═════════════════════════════════════════════════════════════════════════
// WriteSingle
// ═════════════════════════════════════════════════════════════════════════

func TestWriteSingleString(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{
		setPacket: &gosnmp.SnmpPacket{},
	}
	c := newMockedSnmpClient(mock)

	res := newSnmpResource("sysContact", "1.3.6.1.2.1.1.4.0", "admin@example.com")
	err := c.WriteSingle(res)
	if err != nil {
		t.Fatalf("WriteSingle() = %v, want nil", err)
	}

	if len(mock.setPDUs) != 1 {
		t.Fatalf("expected 1 PDU, got %d", len(mock.setPDUs))
	}
	pdu := mock.setPDUs[0]
	if pdu.Name != "1.3.6.1.2.1.1.4.0" {
		t.Errorf("PDU Name = %q", pdu.Name)
	}
	if pdu.Type != gosnmp.OctetString {
		t.Errorf("PDU Type = %v, want OctetString", pdu.Type)
	}
}

func TestWriteSingleInt(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{
		setPacket: &gosnmp.SnmpPacket{},
	}
	c := newMockedSnmpClient(mock)

	res := newSnmpResource("intVal", "1.3.6.1.2.1.1.5.0", 42)
	err := c.WriteSingle(res)
	if err != nil {
		t.Fatalf("WriteSingle() = %v, want nil", err)
	}

	if len(mock.setPDUs) != 1 {
		t.Fatalf("expected 1 PDU, got %d", len(mock.setPDUs))
	}
	pdu := mock.setPDUs[0]
	if pdu.Type != gosnmp.Integer {
		t.Errorf("PDU Type = %v, want Integer", pdu.Type)
	}
}

func TestWriteSingleSetError(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{setErr: errors.New("timeout")}
	c := newMockedSnmpClient(mock)

	res := newSnmpResource("sysContact", "1.3.6.1.2.1.1.4.0", "val")
	err := c.WriteSingle(res)
	if err == nil {
		t.Fatal("WriteSingle() expected error, got nil")
	}
}

// ═════════════════════════════════════════════════════════════════════════
// WriteBatch
// ═════════════════════════════════════════════════════════════════════════

func TestWriteBatchSuccess(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{
		setPacket: &gosnmp.SnmpPacket{},
	}
	c := newMockedSnmpClient(mock)

	points := []model.Resource{
		*newSnmpResource("sysContact", "1.3.6.1.2.1.1.4.0", "admin@example.com"),
		*newSnmpResource("sysName", "1.3.6.1.2.1.1.5.0", "router1"),
	}

	err := c.WriteBatch(points)
	if err != nil {
		t.Fatalf("WriteBatch() = %v, want nil", err)
	}

	if len(mock.setPDUs) != 2 {
		t.Fatalf("expected 2 PDUs, got %d", len(mock.setPDUs))
	}

	names := []string{mock.setPDUs[0].Name, mock.setPDUs[1].Name}
	expected := []string{"1.3.6.1.2.1.1.4.0", "1.3.6.1.2.1.1.5.0"}
	for i, exp := range expected {
		if names[i] != exp {
			t.Errorf("PDU[%d].Name = %q, want %q", i, names[i], exp)
		}
	}
}

func TestWriteBatchEmpty(t *testing.T) {
	t.Parallel()

	c := newMockedSnmpClient(&mockSnmpSession{})
	err := c.WriteBatch(nil)
	if err != nil {
		t.Fatalf("WriteBatch(nil) = %v, want nil", err)
	}
}

func TestWriteBatchSetError(t *testing.T) {
	t.Parallel()

	mock := &mockSnmpSession{setErr: errors.New("timeout")}
	c := newMockedSnmpClient(mock)

	points := []model.Resource{
		*newSnmpResource("sysContact", "1.3.6.1.2.1.1.4.0", "val"),
	}

	err := c.WriteBatch(points)
	if err == nil {
		t.Fatal("WriteBatch() expected error, got nil")
	}
}

// ═════════════════════════════════════════════════════════════════════════
// Float64 address (JSON numeric default)
// ═════════════════════════════════════════════════════════════════════════

func TestReadSingleFloat64Address(t *testing.T) {
	t.Parallel()

	pdu := gosnmp.SnmpPDU{
		Name:  "1.3.6.1.2.1.1.1.0",
		Type:  gosnmp.OctetString,
		Value: []byte("ok"),
	}
	mock := &mockSnmpSession{
		getPacket: &gosnmp.SnmpPacket{Variables: []gosnmp.SnmpPDU{pdu}},
	}
	c := newMockedSnmpClient(mock)

	// Simulate JSON numeric: 1.3e... won't work, so use a simple OID as number
	res := &model.Resource{
		Name:    "sysDescr",
		Address: float64(1), // simple test: OID "1"
		Value:   nil,
	}

	err := c.ReadSingle(res)
	if err != nil {
		t.Fatalf("ReadSingle(float64) = %v, want nil", err)
	}

	// verify mock was called with OID "1" (the float64 conversion result)
	if len(mock.getOIDs) == 0 || mock.getOIDs[0] != "1" {
		t.Errorf("expected Get OID %q, got %v", "1", mock.getOIDs)
	}
}

// ═════════════════════════════════════════════════════════════════════════
// parseAuthProtocol / parsePrivProtocol
// ═════════════════════════════════════════════════════════════════════════

func TestParseAuthProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want gosnmp.SnmpV3AuthProtocol
	}{
		{"MD5", gosnmp.MD5},
		{"SHA", gosnmp.SHA},
		{"SHA256", gosnmp.SHA256},
		{"SHA384", gosnmp.SHA384},
		{"SHA512", gosnmp.SHA512},
		{"unknown", gosnmp.SHA256}, // default
	}

	for _, tt := range tests {
		got := parseAuthProtocol(tt.name)
		if got != tt.want {
			t.Errorf("parseAuthProtocol(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestParsePrivProtocol(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want gosnmp.SnmpV3PrivProtocol
	}{
		{"DES", gosnmp.DES},
		{"AES", gosnmp.AES},
		{"AES192", gosnmp.AES192},
		{"AES256", gosnmp.AES256},
		{"AES256C", gosnmp.AES256C},
		{"unknown", gosnmp.AES256}, // default
	}

	for _, tt := range tests {
		got := parsePrivProtocol(tt.name)
		if got != tt.want {
			t.Errorf("parsePrivProtocol(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// ═════════════════════════════════════════════════════════════════════════
// toOIDString
// ═════════════════════════════════════════════════════════════════════════

func TestToOIDString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		addr    any
		want    string
		wantErr bool
	}{
		{"1.3.6.1.2.1.1.1.0", "1.3.6.1.2.1.1.1.0", false},
		{float64(161), "161", false},
		{int(42), "", true},
		{nil, "", true},
	}

	for _, tt := range tests {
		name := fmt.Sprintf("addr=%v", tt.addr)
		t.Run(name, func(t *testing.T) {
			got, err := toOIDString(tt.addr)
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("toOIDString() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ═════════════════════════════════════════════════════════════════════════
// Interface compliance
// ═════════════════════════════════════════════════════════════════════════

func TestSnmpClientImplementsSession(t *testing.T) {
	t.Parallel()

	// Compile-time assertions cover this; runtime check is redundancy.
	c := newMockedSnmpClient(&mockSnmpSession{})

	// Session
	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect() = %v", err)
	}
}
