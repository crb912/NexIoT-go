// Package client Package protocol define the IoT protocol layer interfaces and the data structures for the returned adapter.Resources.
package client

import (
	"better-iot-edge/pkg/adapter"
	"time"
)

type ProType string

const ModbusProtocol ProType = "modbus-"

// Config represents the unified configuration for any protocol.
// This is usually parsed from config.toml or JSON.
type Config struct {
	protocol ProType       // e.g., "modbus-tcp", "modbus-rtu", "opcua", "http"
	Endpoint string        // e.g., "tcp://127.0.0.1:502" or "/dev/ttyUSB0"
	Timeout  time.Duration // Connection and read timeout
	// You can add more protocol-specific settings here if needed
}

// Reader defines the standard read interface for all protocol plugins.
type Reader interface {
	ReadSingle(pointID string) (adapter.Resource, error)
	ReadBatch(pointIDs []string) ([]adapter.Resource, error)
}

// ReaderClient extends the Reader interface with lifecycle management.
// The main program will ONLY interact with this interface.
type ReaderClient interface {
	Connect() error
	Disconnect() error
	GetProtocolType() ProType
	Reader
}

// ProtocolAdapter defines the interface that all protocol adapters must implement
// to be managed by the ClientPool connection pool.
type ProtocolAdapter interface {
	Connect() error
	Disconnect() error
	GetProtocolType() ProType
	IsConnect() bool
	Reader
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
	WriteBatch(points []adapter.Resource) error // 连续写 n 个点
}

type SingleReader interface {
	ReadSingle(pointID string) (adapter.Resource, error)
}

type BatchReader interface {
	ReadBatch(pointIDs []string) ([]adapter.Resource, error)
}

type RWClient interface {
	Reader
	Writer
	GetProtocolType() ProType
}
