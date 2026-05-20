package adapter

import "time"

type ProtocolType string

const (
	ProtocolMQTT      ProtocolType = "mqtt"
	ProtocolModbusTCP ProtocolType = "modbus-tcp"
	ProtocolModbusRTU ProtocolType = "modbus-rtu"
	ProtocolUnknown   ProtocolType = "unknown"
)

// Resource represents a single piece of data read from a device.
type Resource struct {
	// Address is the unique identifier for the register or tag.
	// Examples: "40001" (Modbus), "ns=2;i=123" (OPC UA), "/api/v1/temp" (HTTP).
	Address string

	// RawValue holds the raw data from the protocol.
	// For Modbus/HTTP, this is usually []byte.
	// For OPC UA, this could be native Go types (e.g., float64, int32).
	RawValue any

	// Timestamp records the exact time the data was received.
	Timestamp time.Time

	// IsValid indicates if the read operation for this specific point was successful.
	// This is critical for batch reads where some points might fail while others succeed.
	IsValid bool

	// Error holds the specific failure reason if IsValid is false.
	Error error
}

// AsyncData defines the unified structure for pushed data
type AsyncData struct {
	DeviceName   string
	ResourceName string
	Value        interface{}
}
