package model

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/edgexfoundry/go-mod-core-contracts/v2/models"
)

// ParseFunc defines the standard signature for all parsing logic
type ParseFunc func(rawData []byte) (any, error)

// Resource is the generic device resource definition
type Resource struct {
	Name      string
	Address   any
	Length    int
	Value     any // Holds the final parsed value
	ValueType string
	Parser    ParseFunc
	Args      map[string]any // Holds protocol-specific attributes
}

// ConvertEdgeXResource translates EdgeX profile into internal Resource
// ConvertEdgeXResource converts EdgeX model to generic Resource
func ConvertEdgeXResource(deviceRes models.DeviceResource) Resource {
	// Initialize the generic resource
	res := Resource{
		Name:      deviceRes.Name,
		ValueType: deviceRes.Properties.ValueType,
		Args:      make(map[string]any),
		Length:    1, // Default length is 1
	}

	// Copy all attributes into Args map
	for key, value := range deviceRes.Attributes {
		res.Args[key] = value
	}

	// Try to extract Address from common attribute keys
	// Modbus usually uses "startingAddress", SNMP uses "oid", etc.
	if addr, ok := res.Args["startingAddress"]; ok {
		res.Address = addr
	} else if oid, ok := res.Args["oid"]; ok {
		res.Address = oid
	}

	// Try to extract Length if it exists in attributes
	if length, ok := res.Args["length"]; ok {
		// JSON numbers are parsed as float64 by default
		if floatLen, isFloat := length.(float64); isFloat {
			res.Length = int(floatLen)
		}
	}

	// Bind the correct parsing function based on ValueType
	res.Parser = assignParser(res.ValueType)

	return res
}

// assignParser acts as a factory to return the correct parser
func assignParser(valueType string) ParseFunc {
	switch strings.ToUpper(valueType) {
	case "STRING":
		return parseStringData
	case "INT16":
		return parseInt16Data
	default:
		return parseDefaultData
	}
}

// ParseMACAddress converts 6 bytes into a MAC string
func ParseMACAddress(rawData []byte) (any, error) {
	if len(rawData) < 6 {
		return nil, fmt.Errorf("data too short for MAC address")
	}
	// Format bytes into standard MAC string
	mac := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		rawData[0], rawData[1], rawData[2],
		rawData[3], rawData[4], rawData[5])
	return mac, nil
}

// ParseScaledTemp converts 2 bytes into a float with 0.1 scale
func ParseScaledTemp(rawData []byte) (any, error) {
	if len(rawData) < 2 {
		return nil, fmt.Errorf("data too short for temperature")
	}
	// Read as uint16, then apply scale
	rawValue := binary.BigEndian.Uint16(rawData)
	temp := float32(rawValue) * 0.1
	return temp, nil
}

// parseStringData converts bytes to string
func parseStringData(rawData []byte, args map[string]any) (any, error) {
	// Example: check if it is a MAC address based on args
	if isMac, ok := args["isMacAddress"].(bool); ok && isMac {
		// Do MAC address formatting here
		return "00:1A:2B:3C:4D:5E", nil
	}
	// Normal string parsing
	return string(rawData), nil
}

// Dummy functions for compilation
func parseInt16Data(rawData []byte, args map[string]any) (any, error)   { return 0, nil }
func parseDefaultData(rawData []byte, args map[string]any) (any, error) { return nil, nil }
