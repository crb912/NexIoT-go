// Package parser:encode provides functions to encode structured data into raw formats for devices.

package parser

import (
	"fmt"
	"strconv"
	"strings"
)

// EncodeFunc converts a user-facing value into []uint16 for modbus register writes.
type EncodeFunc func(any) ([]uint16, error)

// encoderMap provides the reverse codec for every decoder — keyed by decoder name.
var encoderMap = map[string]EncodeFunc{
	"encodeMACAddress":  encodeMACAddress,
	"encodeIPv4Address": encodeIPv4Address,
	"toUint16Slice":     toUint16Slice,
}

// EncodeWData looks up the parser by name and executes it with the data to be written.
// It returns the parsed result or an error if the parser name is invalid.
func EncodeWData(functionName string, rawWriteData any) (any, error) {
	fn, exists := encoderMap[functionName]
	if !exists {
		return nil, fmt.Errorf("parser with name '%s' not found", functionName)
	}
	return fn(rawWriteData)
}

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

// toUint16Slice converts a uint32 value to 2 uint16 registers (big-endian: hi, lo).
func toUint16Slice(v any) ([]uint16, error) {
	val, ok := v.(uint32)
	if !ok {
		return nil, fmt.Errorf("input type must be uint32, got %T", v)
	}

	hi := uint16(val >> 16)
	lo := uint16(val & 0xFFFF)
	return []uint16{hi, lo}, nil
}
