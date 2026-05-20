package connpool

import (
	"better-iot-edge/pkg/adapter"
)

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
	GetProtocolType() adapter.ProtocolType
	Reader
}

// ProtocolAdapter defines the interface that all protocol adapters must implement
// to be managed by the ClientPool connection pool.
type ProtocolAdapter interface {
	Connect() error
	Disconnect() error
	GetProtocolType() adapter.ProtocolType
	IsConnect() bool
	Reader
}

type WriterClient interface {
	Connect() error
	Disconnect() error
	GetProtocolType() adapter.ProtocolType
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
	GetProtocolType() adapter.ProtocolType
}
