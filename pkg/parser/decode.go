package parser

import (
	"fmt"
)

// DecoderFunc defines the standard signature for all parsing logic
type DecoderFunc func(any) (any, error)

var decoderMap = map[string]DecoderFunc{
	"decodeMACAddress":  decodeMACAddress,
	"decodeIPv4Address": decodeIPv4Address,
}

// DecodeRawData looks up the parser by name and executes it with the provided raw data.
// It returns the parsed result or an error if the parser name is invalid.
func DecodeRawData(funcionName string, rawData any) (any, error) {
	// Look up the function in the map
	fn, exists := decoderMap[funcionName]
	if !exists {
		return nil, fmt.Errorf("parser with name '%s' not found", funcionName)
	}

	// Execute the function
	return fn(rawData)
}

// decodeMACAddress converts 3 uint16 registers into a MAC string
func decodeMACAddress(rawData any) (any, error) {
	data, ok := rawData.([]uint16)
	if !ok {
		return "", fmt.Errorf("invalid data type: expected []uint16, got %T", rawData)
	}

	if len(data) < 3 {
		return "", fmt.Errorf("data too short for MAC address: expected 3, got %d", len(data))
	}

	mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		byte(data[0]>>8), byte(data[0]&0xFF),
		byte(data[1]>>8), byte(data[1]&0xFF),
		byte(data[2]>>8), byte(data[2]&0xFF),
	)
	return mac, nil
}

// decodeIPv4Address converts 2 uint16 registers into an IPv4 string
func decodeIPv4Address(rawData any) (any, error) {
	// Check if data is an array of uint16
	data, ok := rawData.([]uint16)
	if !ok {
		return "", fmt.Errorf("invalid data type: expected []uint16, got %T", rawData)
	}

	// Check if data length is at least 2
	if len(data) < 2 {
		return "", fmt.Errorf("data too short for IPv4 address: expected 2, got %d", len(data))
	}

	// Format 4 bytes into an IP string (e.g., 192.168.1.100)
	ip := fmt.Sprintf("%d.%d.%d.%d",
		byte(data[0]>>8), byte(data[0]&0xFF),
		byte(data[1]>>8), byte(data[1]&0xFF),
	)

	return ip, nil
}
