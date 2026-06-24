package parser

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// DecoderFunc defines the standard signature for all parsing logic.
type DecoderFunc func(any) (any, error)

// EncoderFunc converts a user-facing value into []uint16 for modbus register writes.
type EncoderFunc func(any) ([]uint16, error)

var decoderMap = map[string]DecoderFunc{
	"decodeMACAddress":  decodeMACAddress,
	"decodeIPv4Address": decodeIPv4Address,
	"toUnit32":          toUnit32,
}

// encoderMap provides the reverse codec for every decoder — keyed by decoder name.
var encoderMap = map[string]EncoderFunc{
	"decodeMACAddress":  encodeMACAddress,
	"decodeIPv4Address": encodeIPv4Address,
	"toUnit32":          encodeUint32,
}

// DecodeRawData looks up the parser by name and executes it with the provided raw data.
// It returns the parsed result or an error if the parser name is invalid.
func DecodeRawData(funcionName string, rawData any) (any, error) {
	fn, exists := decoderMap[funcionName]
	if !exists {
		return nil, fmt.Errorf("parser with name '%s' not found", funcionName)
	}
	return fn(rawData)
}

// EncodeRawData converts a value to []uint16 for writing to modbus registers.
// If the resource has a decoder, the matching encoder is used.
// Otherwise, auto-encoding is attempted based on valueType and length.
func EncodeRawData(decoderName string, valueType string, length uint16, value any) ([]uint16, error) {
	// Try registered encoder (keyed by decoder name).
	if fn, exists := encoderMap[decoderName]; exists {
		return fn(value)
	}
	// Fall back to auto-encoding based on valueType.
	return autoEncode(valueType, length, value)
}

// ─── Custom encoders (reverse of each decoder) ────────────────────────────

// encodeMACAddress converts a MAC string "AA:BB:CC:DD:EE:FF" to 3 uint16 registers.
func encodeMACAddress(v any) ([]uint16, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("encodeMACAddress: expected string, got %T", v)
	}
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return nil, fmt.Errorf("encodeMACAddress: invalid format %q", s)
	}
	b := make([]byte, 6)
	for i, p := range parts {
		val, err := strconv.ParseUint(p, 16, 8)
		if err != nil {
			return nil, fmt.Errorf("encodeMACAddress: bad octet %q: %w", p, err)
		}
		b[i] = byte(val)
	}
	return []uint16{
		uint16(b[0])<<8 | uint16(b[1]),
		uint16(b[2])<<8 | uint16(b[3]),
		uint16(b[4])<<8 | uint16(b[5]),
	}, nil
}

// encodeIPv4Address converts an IPv4 string "192.168.0.1" to 2 uint16 registers.
func encodeIPv4Address(v any) ([]uint16, error) {
	s, ok := v.(string)
	if !ok {
		return nil, fmt.Errorf("encodeIPv4Address: expected string, got %T", v)
	}
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return nil, fmt.Errorf("encodeIPv4Address: invalid format %q", s)
	}
	b := make([]byte, 4)
	for i, p := range parts {
		val, err := strconv.ParseUint(p, 10, 8)
		if err != nil {
			return nil, fmt.Errorf("encodeIPv4Address: bad octet %q: %w", p, err)
		}
		b[i] = byte(val)
	}
	return []uint16{
		uint16(b[0])<<8 | uint16(b[1]),
		uint16(b[2])<<8 | uint16(b[3]),
	}, nil
}

// encodeUint32 converts a uint32 value to 2 uint16 registers (big-endian: hi, lo).
func encodeUint32(v any) ([]uint16, error) {
	val, err := toUint32Any(v)
	if err != nil {
		return nil, err
	}
	hi := uint16(val >> 16)
	lo := uint16(val & 0xFFFF)
	return []uint16{hi, lo}, nil
}

// ─── Auto encoding (no decoder registered) ───────────────────────────────

// autoEncode converts a value to []uint16 based on valueType and length.
func autoEncode(valueType string, length uint16, value any) ([]uint16, error) {
	switch valueType {
	case "Uint16":
		if length == 1 {
			v, err := toUint16Any(value)
			if err != nil {
				return nil, err
			}
			return []uint16{v}, nil
		}
		// length > 1: expect a slice of uint16 values.
		return toUint16SliceAny(value, length)
	case "Uint32":
		if length != 2 {
			return nil, fmt.Errorf("autoEncode: Uint32 expects length 2, got %d", length)
		}
		return encodeUint32(value)
	default:
		return nil, fmt.Errorf("autoEncode: no encoder for valueType %q (add a decodefunc to enable custom encoding)", valueType)
	}
}

// ─── Conversion helpers ──────────────────────────────────────────────────

// toUint16Any converts various numeric/string types to uint16.
func toUint16Any(v any) (uint16, error) {
	switch val := v.(type) {
	case uint16:
		return val, nil
	case uint8:
		return uint16(val), nil
	case int:
		if val < 0 || val > 65535 {
			return 0, fmt.Errorf("toUint16: value %d out of range [0, 65535]", val)
		}
		return uint16(val), nil
	case int64:
		if val < 0 || val > 65535 {
			return 0, fmt.Errorf("toUint16: value %d out of range [0, 65535]", val)
		}
		return uint16(val), nil
	case float64:
		if val < 0 || val > 65535 {
			return 0, fmt.Errorf("toUint16: value %v out of range [0, 65535]", val)
		}
		return uint16(val), nil
	case string:
		u, err := strconv.ParseUint(val, 10, 16)
		if err != nil {
			return 0, fmt.Errorf("toUint16: cannot parse %q: %w", val, err)
		}
		return uint16(u), nil
	default:
		return 0, fmt.Errorf("toUint16: unsupported type %T", v)
	}
}

// toUint32Any converts various numeric/string types to uint32.
func toUint32Any(v any) (uint32, error) {
	switch val := v.(type) {
	case uint32:
		return val, nil
	case uint16:
		return uint32(val), nil
	case int:
		if val < 0 || val > 4294967295 {
			return 0, fmt.Errorf("toUint32: value %d out of range [0, 4294967295]", val)
		}
		return uint32(val), nil
	case int64:
		if val < 0 || val > 4294967295 {
			return 0, fmt.Errorf("toUint32: value %d out of range [0, 4294967295]", val)
		}
		return uint32(val), nil
	case float64:
		if val < 0 || val > 4294967295 {
			return 0, fmt.Errorf("toUint32: value %v out of range [0, 4294967295]", val)
		}
		return uint32(val), nil
	case string:
		u, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return 0, fmt.Errorf("toUint32: cannot parse %q: %w", val, err)
		}
		return uint32(u), nil
	default:
		return 0, fmt.Errorf("toUint32: unsupported type %T", v)
	}
}

// toUint16SliceAny converts a value to []uint16 of expected length.
// Supports []uint16, []int, []float64, and other slice types.
func toUint16SliceAny(v any, expectedLen uint16) ([]uint16, error) {
	switch val := v.(type) {
	case []uint16:
		if len(val) != int(expectedLen) {
			return nil, fmt.Errorf("toUint16Slice: got %d elements, expected %d", len(val), expectedLen)
		}
		return val, nil
	case []int:
		if len(val) != int(expectedLen) {
			return nil, fmt.Errorf("toUint16Slice: got %d elements, expected %d", len(val), expectedLen)
		}
		result := make([]uint16, len(val))
		for i, iv := range val {
			if iv < 0 || iv > 65535 {
				return nil, fmt.Errorf("toUint16Slice: element %d out of range", iv)
			}
			result[i] = uint16(iv)
		}
		return result, nil
	case []float64:
		if len(val) != int(expectedLen) {
			return nil, fmt.Errorf("toUint16Slice: got %d elements, expected %d", len(val), expectedLen)
		}
		result := make([]uint16, len(val))
		for i, fv := range val {
			if fv < 0 || fv > 65535 {
				return nil, fmt.Errorf("toUint16Slice: element %v out of range", fv)
			}
			result[i] = uint16(fv)
		}
		return result, nil
	case []any:
		if len(val) != int(expectedLen) {
			return nil, fmt.Errorf("toUint16Slice: got %d elements, expected %d", len(val), expectedLen)
		}
		result := make([]uint16, len(val))
		for i, iv := range val {
			u, err := toUint16Any(iv)
			if err != nil {
				return nil, fmt.Errorf("toUint16Slice: element %d: %w", i, err)
			}
			result[i] = u
		}
		return result, nil
	default:
		return nil, fmt.Errorf("toUint16Slice: unsupported type %T", v)
	}
}

// decodeMACAddress converts 3 uint16 registers into a MAC string.
func decodeMACAddress(rawData any) (any, error) {
	data, ok := rawData.([]uint16)
	if !ok {
		return "", errors.New("invalid data type: expected []uint16 for MAC address")
	}

	if len(data) < 3 {
		return "", errors.New("data too short for MAC address: expected 3 registers")
	}

	mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		byte(data[0]>>8), byte(data[0]&0xFF),
		byte(data[1]>>8), byte(data[1]&0xFF),
		byte(data[2]>>8), byte(data[2]&0xFF),
	)
	return mac, nil
}

// decodeIPv4Address converts 2 uint16 registers into an IPv4 string.
func decodeIPv4Address(rawData any) (any, error) {
	data, ok := rawData.([]uint16)
	if !ok {
		return "", errors.New("invalid data type: expected []uint16 for IPv4 address")
	}

	if len(data) < 2 {
		return "", errors.New("data too short for IPv4 address: expected 2 registers")
	}

	ip := fmt.Sprintf("%d.%d.%d.%d",
		byte(data[0]>>8), byte(data[0]&0xFF),
		byte(data[1]>>8), byte(data[1]&0xFF),
	)

	return ip, nil
}

// toUnit32 converts 2 uint16 registers into a uint32 value
// Modbus standard: data[0] = high word, data[1] = low word
func toUnit32(rawData any) (any, error) {
	data, ok := rawData.([]uint16)
	if !ok {
		return uint32(0), errors.New("invalid data type: expected []uint16 for uint32")
	}

	if len(data) < 2 {
		return uint32(0), errors.New("data too short for uint32: expected 2 registers")
	}

	val := uint32(data[0])<<16 | uint32(data[1])
	return val, nil
}
