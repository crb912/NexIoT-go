package parser

import (
	"fmt"
)

// DecoderFunc defines the standard signature for all parsing logic
type DecoderFunc func(any) (any, error)

var decoderMap = map[string]DecoderFunc{
	"parseMACAddress": decodeMACAddress,
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
	// Type assert rawData to []uint16
	data, ok := rawData.([]uint16)
	if !ok {
		// Return empty string on error
		return "", fmt.Errorf("invalid data type: expected []uint16, got %T", rawData)
	}

	// Check if we have at least 3 registers (6 bytes)
	if len(data) < 3 {
		// Return empty string on error
		return "", fmt.Errorf("data too short for MAC address: expected 3, got %d", len(data))
	}

	// Extract high and low bytes from each uint16
	mac := fmt.Sprintf("%02X:%02X:%02X:%02X:%02X:%02X",
		byte(data[0]>>8), byte(data[0]&0xFF),
		byte(data[1]>>8), byte(data[1]&0xFF),
		byte(data[2]>>8), byte(data[2]&0xFF),
	)

	return mac, nil
}
