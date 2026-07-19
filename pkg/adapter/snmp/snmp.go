// Package snmp provides an SNMP protocol adapter implementing the RWClient interface.
// It supports SNMP v1, v2c, and v3 for active polling (Get/Set) of device resources.
package snmp

import (
	"next-iot-go/pkg/conv"
	"next-iot-go/pkg/model"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gosnmp/gosnmp"
)

// SnmpSession declares the methods that SnmpClient calls on the underlying gosnmp connection.
// This interface allows mocking in tests without a real network connection.
type SnmpSession interface {
	Connect() error
	Get(oids []string) (*gosnmp.SnmpPacket, error)
	Set(pdus []gosnmp.SnmpPDU) (*gosnmp.SnmpPacket, error)
	Close() error
}

// gosnmpSession wraps *gosnmp.GoSNMP to satisfy SnmpSession.
// The stock *gosnmp.GoSNMP lacks a Close() method — it exposes Conn.Close() instead.
type gosnmpSession struct {
	*gosnmp.GoSNMP
}

func (g *gosnmpSession) Close() error {
	if g.GoSNMP.Conn != nil {
		return g.GoSNMP.Conn.Close()
	}
	return nil
}

// Compile-time check: gosnmpSession satisfies SnmpSession.
var _ SnmpSession = (*gosnmpSession)(nil)

// SnmpClient holds connection parameters and the underlying session.
// It implements the protocol.RWClient interface (Session + Reader + Writer).
type SnmpClient struct {
	Target       string
	Transport    string
	EndPoint     string
	ProtocolType model.ProtocolType
	Timeout      time.Duration
	Version      gosnmp.SnmpVersion
	Community    string
	V3Params     *gosnmp.UsmSecurityParameters
	Port         uint16 // default 161

	mu        sync.Mutex
	connected bool
	client    SnmpSession
}

// NewSnmpClient constructs an SnmpClient from the generic args map.
// Supported args keys:
//
//	EndPoint         — eg. "udp://127.0.0.1:1610"
//	Address          — SNMP agent host/IP (overrides the Target parameter, preferred)
//	SnmpVersion     — "v1", "v2c" (default), "v3"
//	Community        — Community string (default "public", v1/v2c only)
//	Port             — SNMP agent port (default 161)
//	user_name        — v3 security name
//	auth_protocol    — v3 auth protocol: MD5, SHA, SHA256, SHA384, SHA512
//	auth_passphrase  — v3 auth passphrase
//	priv_protocol    — v3 privacy protocol: DES, AES, AES192, AES256, AES256C
//	priv_passphrase  — v3 privacy passphrase
func NewSnmpClient(endpoint string, pt model.ProtocolType, defaultTimeout time.Duration, args map[string]string) (*SnmpClient, error) {
	c := &SnmpClient{
		EndPoint:     endpoint,
		ProtocolType: pt,
		Timeout:      defaultTimeout,
		Version:      gosnmp.Version2c,
		Transport:    "udp",
		Community:    "public",
		Port:         161,
	}

	if args == nil {
		return nil, errors.New("snmp client args is nil")
	}

	if v, ok := args["Community"]; ok {
		c.Community = v
	}
	if v, ok := args["Address"]; ok && v != "" {
		c.Target = v
	}
	if v, ok := args["Transport"]; ok && v != "" {
		c.Transport = v
	}

	if v, ok := args["Port"]; ok {
		if u, ok2 := conv.ToUint(v); ok2 {
			c.Port = uint16(u)
		}
	}

	if v, ok := args["SnmpVersion"]; ok {
		switch v {
		case "v1":
			c.Version = gosnmp.Version1
		case "v2c":
			c.Version = gosnmp.Version2c
		case "v3":
			c.Version = gosnmp.Version3
		}
	}

	if c.Version == gosnmp.Version3 {
		c.V3Params = &gosnmp.UsmSecurityParameters{}
		if v, ok := args["UserName"]; ok {
			c.V3Params.UserName = v
		}
		if v, ok := args["AuthProtocol"]; ok {
			c.V3Params.AuthenticationProtocol = parseAuthProtocol(v)
		}
		if v, ok := args["AuthPassphrase"]; ok {
			c.V3Params.AuthenticationPassphrase = v
		}
		if v, ok := args["PrivProtocol"]; ok {
			c.V3Params.PrivacyProtocol = parsePrivProtocol(v)
		}
		if v, ok := args["PrivPassphrase"]; ok {
			c.V3Params.PrivacyPassphrase = v
		}
	}

	return c, nil
}

// Connect opens the SNMP connection.
func (s *SnmpClient) Connect() error {

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client == nil {
		client, err := s.newSession()
		if err != nil {
			return err
		}
		s.client = client
	} else if s.connected {
		return nil
	}

	if err := s.client.Connect(); err != nil {
		return fmt.Errorf("SNMP connect to %s:%d: %w", s.Target, s.Port, err)
	}
	s.connected = true
	return nil
}

// Disconnect closes the SNMP connection.
func (s *SnmpClient) Disconnect() error {
	if s.client == nil {
		s.connected = false
		return nil
	}

	if err := s.client.Close(); err != nil {
		s.connected = false
		s.client = nil
		return fmt.Errorf("SNMP disconnect: %w", err)
	}
	s.connected = false
	return nil
}

// ReadSingle reads a single SNMP OID.
func (s *SnmpClient) ReadSingle(res *model.Resource) error {
	oid, err := toOIDString(res.Address)
	if err != nil {
		return fmt.Errorf("ReadSingle %s: %w", res.Name, err)
	}

	packet, err := s.client.Get([]string{oid})
	if err != nil {
		s.connected = false
		return fmt.Errorf("SNMP Get %s: %w", oid, err)
	}
	if len(packet.Variables) == 0 {
		return fmt.Errorf("SNMP Get %s: no variables returned", oid)
	}

	pdu := packet.Variables[0]
	val, err := conv.ValueToType(pdu.Value, res.Type)
	if err != nil {
		return fmt.Errorf("SNMP Get %s: %w", oid, err)
	}
	res.Value = val
	return nil
}

// ReadBatch reads multiple SNMP OIDs in a single Get request.
func (s *SnmpClient) ReadBatch(points []model.Resource) error {
	if len(points) == 0 {
		return nil
	}

	oids := make([]string, len(points))
	oidIndex := make(map[string]int, len(points))
	for i := range points {
		oid, err := toOIDString(points[i].Address)
		if err != nil {
			return fmt.Errorf("ReadBatch %s: %w", points[i].Name, err)
		}
		oids[i] = oid
		oidIndex[oid] = i
	}

	packet, err := s.client.Get(oids)
	if err != nil {
		return fmt.Errorf("SNMP Get batch: %w", err)
	}

	for _, pdu := range packet.Variables {
		idx, found := oidIndex[pdu.Name]
		if !found {
			continue
		}
		val, err := conv.ValueToType(pdu.Value, points[idx].Type)
		if err != nil {
			return fmt.Errorf("SNMP Get batch %s: %w", pdu.Name, err)
		}
		points[idx].Value = val
	}
	return nil
}

// WriteSingle writes a single value via SNMP SET.
func (s *SnmpClient) WriteSingle(res *model.Resource) error {
	oid, err := toOIDString(res.Address)
	if err != nil {
		return fmt.Errorf("WriteSingle %s: %w", res.Name, err)
	}

	asn1Type, value, err := goValueToSnmpPDU(res.Value)
	if err != nil {
		return fmt.Errorf("WriteSingle %s: %w", res.Name, err)
	}

	pdu := gosnmp.SnmpPDU{
		Name:  oid,
		Type:  asn1Type,
		Value: value,
	}

	_, err = s.client.Set([]gosnmp.SnmpPDU{pdu})
	if err != nil {
		return fmt.Errorf("SNMP Set %s: %w", oid, err)
	}
	return nil
}

// WriteBatch writes multiple values in a single SNMP SET request.
func (s *SnmpClient) WriteBatch(points []model.Resource) error {
	if len(points) == 0 {
		return nil
	}

	pdus := make([]gosnmp.SnmpPDU, 0, len(points))
	for i := range points {
		oid, err := toOIDString(points[i].Address)
		if err != nil {
			return fmt.Errorf("WriteBatch %s: %w", points[i].Name, err)
		}

		asn1Type, value, err := goValueToSnmpPDU(points[i].Value)
		if err != nil {
			return fmt.Errorf("WriteBatch %s: %w", points[i].Name, err)
		}

		pdus = append(pdus, gosnmp.SnmpPDU{
			Name:  oid,
			Type:  asn1Type,
			Value: value,
		})
	}

	_, err := s.client.Set(pdus)
	if err != nil {
		return fmt.Errorf("SNMP Set batch: %w", err)
	}
	return nil
}

// newSession creates and configures the underlying gosnmp.GoSNMP instance.
func (s *SnmpClient) newSession() (SnmpSession, error) {
	g := &gosnmp.GoSNMP{
		Target:    s.Target,
		Port:      s.Port,
		Transport: s.Transport,
		Version:   s.Version,
		Timeout:   s.Timeout,
		Retries:   3,
		Community: s.Community,
	}

	if s.Version == gosnmp.Version3 {
		g.SecurityModel = gosnmp.UserSecurityModel
		g.MsgFlags = gosnmp.AuthPriv
		if s.V3Params != nil {
			g.SecurityParameters = s.V3Params
		} else {
			g.SecurityParameters = &gosnmp.UsmSecurityParameters{}
		}
	}

	return &gosnmpSession{GoSNMP: g}, nil
}

// toOIDString converts the resource Address to an OID string.
// Supports both string (preferred) and float64 (JSON numeric default).
func toOIDString(addr any) (string, error) {
	switch v := addr.(type) {
	case string:
		return v, nil
	case float64:
		return fmt.Sprintf("%.0f", v), nil
	default:
		return "", fmt.Errorf("SNMP address must be string or number, got %T", addr)
	}
}

// parseAuthProtocol maps a string name to a gosnmp authentication protocol.
func parseAuthProtocol(name string) gosnmp.SnmpV3AuthProtocol {
	switch name {
	case "MD5":
		return gosnmp.MD5
	case "SHA":
		return gosnmp.SHA
	case "SHA256":
		return gosnmp.SHA256
	case "SHA384":
		return gosnmp.SHA384
	case "SHA512":
		return gosnmp.SHA512
	default:
		return gosnmp.SHA256
	}
}

// parsePrivProtocol maps a string name to a gosnmp privacy protocol.
func parsePrivProtocol(name string) gosnmp.SnmpV3PrivProtocol {
	switch name {
	case "DES":
		return gosnmp.DES
	case "AES":
		return gosnmp.AES
	case "AES192":
		return gosnmp.AES192
	case "AES256":
		return gosnmp.AES256
	case "AES256C":
		return gosnmp.AES256C
	default:
		return gosnmp.AES256
	}
}

// goValueToSnmpPDU maps a native Go value to the appropriate
// ASN.1 type and value pair for an SNMP SET operation.
func goValueToSnmpPDU(v interface{}) (gosnmp.Asn1BER, interface{}, error) {
	switch val := v.(type) {
	case int:
		return gosnmp.Integer, val, nil
	case int32:
		return gosnmp.Integer, int(val), nil
	case int64:
		return gosnmp.Counter64, val, nil
	case uint:
		return gosnmp.Gauge32, uint32(val), nil
	case uint32:
		return gosnmp.Gauge32, val, nil
	case uint64:
		return gosnmp.Counter64, val, nil
	case float32:
		return gosnmp.OpaqueFloat, val, nil
	case float64:
		return gosnmp.OpaqueDouble, val, nil
	case string:
		return gosnmp.OctetString, []byte(val), nil
	case []byte:
		return gosnmp.OctetString, val, nil
	case bool:
		if val {
			return gosnmp.Integer, 1, nil
		}
		return gosnmp.Integer, 0, nil
	default:
		return gosnmp.OctetString, []byte(fmt.Sprintf("%v", v)), nil
	}
}
