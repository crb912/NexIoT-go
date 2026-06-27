// Package conv provides type conversion helpers used across protocol adapters.
package conv

import (
	"fmt"
	"reflect"

	"github.com/gosnmp/gosnmp"
)

// ToBool only accept bool value.
func ToBool(v any) (bool, error) {
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("only support bool value")
	}
	return b, nil
}

// ToUint16 only accept uint16 value.
func ToUint16(v any) (uint16, error) {
	u, ok := v.(uint16)
	if !ok {
		return 0, fmt.Errorf("only support uint16 value")
	}
	return u, nil
}

// ToUint converts common numeric types stored in interface{} to uint.
func ToUint(v interface{}) (uint, bool) {
	switch val := v.(type) {
	case uint:
		return val, true
	case int:
		if val >= 0 {
			return uint(val), true
		}
	case uint64:
		return uint(val), true
	case int64:
		if val >= 0 {
			return uint(val), true
		}
	case float64:
		if val >= 0 {
			return uint(val), true
		}
	}
	return 0, false
}

// ToBoolSlice converts input to []bool for modbus coil write.
// Supports native bool or []bool input.
func ToBoolSlice(v any) ([]bool, error) {
	if v == nil {
		return nil, fmt.Errorf("only bool, []bool supported, got nil")
	}
	switch val := v.(type) {
	case bool:
		return []bool{val}, nil
	case []bool:
		return val, nil
	default:
		return nil, fmt.Errorf("only bool, []bool supported, got %s", reflect.TypeOf(v))
	}
}

// ToUint16Slice converts input to []uint16 for modbus register write.
// Supports uint16 or []uint16 input.
func ToUint16Slice(v any) ([]uint16, error) {
	if v == nil {
		return nil, fmt.Errorf("only uint16, []uint16 supported, got nil")
	}
	switch val := v.(type) {
	case uint16:
		return []uint16{val}, nil
	case []uint16:
		return val, nil
	default:
		return nil, fmt.Errorf("only uint16, []uint16 supported, got %s", reflect.TypeOf(v))
	}
}

// ─── SNMP type conversion helpers ───────────────────────────────────────

// SnmpPDUValueToGo converts a gosnmp PDU value to a native Go type.
// gosnmp already returns native Go types for most ASN.1 types;
// this helper converts []byte OctetString to string for convenience.
func SnmpPDUValueToGo(asn1Type gosnmp.Asn1BER, value interface{}) interface{} {
	if asn1Type == gosnmp.OctetString {
		if b, ok := value.([]byte); ok {
			return string(b)
		}
	}
	return value
}

// GoValueToSnmpPDU maps a native Go value to the appropriate
// ASN.1 type and value pair for an SNMP SET operation.
func GoValueToSnmpPDU(v interface{}) (gosnmp.Asn1BER, interface{}, error) {
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
