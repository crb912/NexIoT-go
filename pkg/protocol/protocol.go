// Package protocol define the IoT protocol layer interfaces and the data structures for the returned resources.
package protocol

import "time"

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

type ProType string

// ReaderClient  extends the Reader interface with lifecycle management.
// The main program will ONLY interact with this interface.
type ReaderClient interface {
	Connect() error
	Disconnect() error
	GetProtocolType() ProType
	Reader
}

// Reader defines the standard read interface for all protocol plugins.
type Reader interface {
	ReadSingle(pointID string) (Resource, error)
	ReadBatch(pointIDs []string) ([]Resource, error)
}

type WriterClient interface {
	Connect() error
	Disconnect() error
	GetProtocolType() ProType
	Writer
}

// Writer defines the standard write interface for all protocol plugins.
type Writer interface {
	WriteSingle(addr string, value interface{}) error
	WriteBatch(points []Resource) error // 连续写 n 个点
}

type SingleReader interface {
	ReadSingle(pointID string) (Resource, error)
}

type BatchReader interface {
	ReadBatch(pointIDs []string) ([]Resource, error)
}

type RWClient interface {
	LifeSpan
	Reader
	Writer
	GetProtocolType() ProType
}
