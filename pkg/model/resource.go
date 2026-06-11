package model

import (
	"fmt"

	sdkModels "github.com/edgexfoundry/device-sdk-go/v2/pkg/models"
)

// Resource is the generic device resource definition for all protocols
type Resource struct {
	Name    string
	Address any
	Length  int
	Value   any
	Type    string
	Parser  ParseFunc
	Args    map[string]any // Holds protocol-specific attributes
}

// ParseFunc defines the standard signature for all parsing logic
type ParseFunc func(rawData []byte) (any, error)

var parserMap = map[string]ParseFunc{
	"parseMACAddress": parseMACAddress,
}

// NewResource converts EdgeX model to generic Resource
func NewResource(deviceRes sdkModels.CommandRequest) *Resource {
	res := Resource{
		Name: deviceRes.DeviceResourceName,
		Type: deviceRes.Type,
		Args: make(map[string]any),
	}

	for key, value := range deviceRes.Attributes {
		switch key {
		case "address":
			res.Address = value
		case "length":
			res.Length = int(value.(float64))
		case "parsefunc":
			res.Parser, _ = parserMap[value.(string)]
		default:
			res.Args[key] = value
		}
	}
	return &res
}

// parseMACAddress converts 6 bytes into a MAC string
func parseMACAddress(rawData []byte) (any, error) {
	if len(rawData) < 6 {
		return nil, fmt.Errorf("data too short for MAC address")
	}
	// Format bytes into standard MAC string
	mac := fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x",
		rawData[0], rawData[1], rawData[2],
		rawData[3], rawData[4], rawData[5])
	return mac, nil
}
