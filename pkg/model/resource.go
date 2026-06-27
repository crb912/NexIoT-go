package model

import (
	"time"

	sdkModels "github.com/edgexfoundry/device-sdk-go/v2/pkg/models"
)

// Resource is the generic device resource definition for all protocols
type Resource struct {
	Name string
	// Address is the unique identifier for the register or tag.
	// Examples: "40001" (Modbus), "ns=2;i=123" (OPC UA), "/api/v1/temp" (HTTP).
	Address any
	Length  uint16
	// Value holds the raw data from the protocol.
	// For Modbus/HTTP, this is usually []byte.
	// For OPC UA, this could be native Go types (e.g., float64, int32).
	Value   any
	Type    string
	Decoder string
	Encoder string
	Args    map[string]any // Holds protocol-specific attributes
}

type ReceiveEvent struct {
	EventName string
	EventTime time.Time
	EventData []byte
}

// NewResource converts EdgeX model to generic Resource
func NewResource(deviceRes sdkModels.CommandRequest) Resource {
	res := Resource{
		Name: deviceRes.DeviceResourceName,
		Type: deviceRes.Type,
	}

	for key, value := range deviceRes.Attributes {
		switch key {
		case "address":
			res.Address = value
		case "length":
			res.Length = uint16(value.(float64))
		case "decodeFunc":
			res.Decoder, _ = value.(string)
		case "encodeFunc":
			res.Encoder, _ = value.(string)
		default:
			if res.Args == nil {
				res.Args = make(map[string]any)
			}
			res.Args[key] = value
		}
	}
	return res
}

// NewResourceN converts EdgeX model to generic Resource Slice
func NewResourceN(deviceResList []sdkModels.CommandRequest) []Resource {
	res := make([]Resource, len(deviceResList))
	for i, deviceRes := range deviceResList {
		res[i] = NewResource(deviceRes)
	}
	return res
}
