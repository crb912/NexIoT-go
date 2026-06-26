package parser

import (
	"errors"
	"fmt"
)

// DecodeFunc defines the decoder for resources from devices.
type DecodeFunc func(any) (any, error)

var decoderMap = map[string]DecodeFunc{
	"decodeMACAddress":  decodeMACAddress,
	"decodeIPv4Address": decodeIPv4Address,
	"toUint32":          toUint32,
}

// DecodeRData looks up the parser by name and executes it with the provided raw data.
// It returns the parsed result or an error if the parser name is invalid.
func DecodeRData(functionName string, rawReadData any) (any, error) {
	fn, exists := decoderMap[functionName]
	if !exists {
		return nil, fmt.Errorf("parser with name '%s' not found", functionName)
	}
	return fn(rawReadData)
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

// toUint32 converts 2 uint16 registers into a uint32 value
// Modbus standard: data[0] = high word, data[1] = low word
func toUint32(rawData any) (any, error) {
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
