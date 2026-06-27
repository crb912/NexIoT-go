// Package snmp provides an SNMP protocol adapter implementing the RWClient interface.
// It supports SNMP v1, v2c, and v3 for active polling (Get/Set) of device resources.
package snmp

import (
	"devices-iot-go/pkg/conv"
	"devices-iot-go/pkg/model"
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
//	snmp_version     — "v1", "v2c" (default), "v3"
//	community        — community string (default "public", v1/v2c only)
//	port             — SNMP agent port (default 161)
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
		Community:    "public",
		Port:         161,
	}

	if args == nil {
		return c, nil
	}

	if v, ok := args["snmp_version"]; ok {
		switch v {
		case "v1":
			c.Version = gosnmp.Version1
		case "v2c":
			c.Version = gosnmp.Version2c
		case "v3":
			c.Version = gosnmp.Version3
		}
	}

	if v, ok := args["community"]; ok {
		c.Community = v
	}

	if v, ok := args["port"]; ok {
		if u, ok2 := conv.ToUint(v); ok2 {
			c.Port = uint16(u)
		}
	}

	if c.Version == gosnmp.Version3 {
		c.V3Params = &gosnmp.UsmSecurityParameters{}
		if v, ok := args["user_name"]; ok {
			c.V3Params.UserName = v
		}
		if v, ok := args["auth_protocol"]; ok {
			c.V3Params.AuthenticationProtocol = parseAuthProtocol(v)
		}
		if v, ok := args["auth_passphrase"]; ok {
			c.V3Params.AuthenticationPassphrase = v
		}
		if v, ok := args["priv_protocol"]; ok {
			c.V3Params.PrivacyProtocol = parsePrivProtocol(v)
		}
		if v, ok := args["priv_passphrase"]; ok {
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
	}

	if err := s.client.Connect(); err != nil {
		return fmt.Errorf("SNMP connect to %s:%d: %w", s.EndPoint, s.Port, err)
	}
	s.connected = true
	return nil
}

// Disconnect closes the SNMP connection.
func (s *SnmpClient) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client == nil {
		s.connected = false
		return nil
	}

	if err := s.client.Close(); err != nil {
		s.connected = false
		return fmt.Errorf("SNMP disconnect: %w", err)
	}
	s.connected = false
	return nil
}

// IsConnected checks whether the SNMP session is alive.
func (s *SnmpClient) IsConnected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client == nil || !s.connected {
		return false
	}

	// Health check: try Get on sysDescr.0 (standard OID).
	_, err := s.client.Get([]string{"1.3.6.1.2.1.1.1.0"})
	if err != nil {
		s.connected = false
		return false
	}
	return true
}

// ReadSingle reads a single SNMP OID.
func (s *SnmpClient) ReadSingle(res *model.Resource) error {
	oid, err := toOIDString(res.Address)
	if err != nil {
		return fmt.Errorf("ReadSingle %s: %w", res.Name, err)
	}

	packet, err := s.client.Get([]string{oid})
	if err != nil {
		return fmt.Errorf("SNMP Get %s: %w", oid, err)
	}
	if len(packet.Variables) == 0 {
		return fmt.Errorf("SNMP Get %s: no variables returned", oid)
	}

	pdu := packet.Variables[0]
	res.Value = conv.SnmpPDUValueToGo(pdu.Type, pdu.Value)
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
		points[idx].Value = conv.SnmpPDUValueToGo(pdu.Type, pdu.Value)
	}
	return nil
}

// WriteSingle writes a single value via SNMP SET.
func (s *SnmpClient) WriteSingle(res *model.Resource) error {
	oid, err := toOIDString(res.Address)
	if err != nil {
		return fmt.Errorf("WriteSingle %s: %w", res.Name, err)
	}

	asn1Type, value, err := conv.GoValueToSnmpPDU(res.Value)
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

		asn1Type, value, err := conv.GoValueToSnmpPDU(points[i].Value)
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
		Target:    s.EndPoint,
		Port:      s.Port,
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
