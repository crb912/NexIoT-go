package connpool

import (
	"better-iot-edge/pkg/adapter"
	"context"
)

// Reader defines the standard read interface for all protocol plugins.
type Reader interface {
	ReadSingle(pointID string) (adapter.Resource, error)
	ReadBatch(pointIDs []string) ([]adapter.Resource, error)
}

// Writer defines the standard write interface for all protocol plugins.
type Writer interface {
	WriteSingle(addr string, value interface{}) error
	WriteBatch(points []adapter.Resource) error // 连续写 n 个点
}

// Session manages the lifecycle of a connection.
type Session interface {
	Connect() error
	Disconnect() error
	IsConnected() bool
}

// ReaderAdapter embeds the Reader interface with lifecycle management.
type ReaderAdapter interface {
	Session
	Reader
	GetProtocolType() adapter.ProtocolType
}

// WriterAdapter embeds the Writer interface with lifecycle management.
type WriterAdapter interface {
	Session
	Writer
	GetProtocolType() adapter.ProtocolType
}

// RWAdapter embeds the Reader and Writer interfaces with lifecycle management.
type RWAdapter interface {
	Session
	Reader
	Writer
	GetProtocolType() adapter.ProtocolType
}

// ReceiverAdapter interface: all passive protocols must implement this
type ReceiverAdapter interface {
	Start(ctx context.Context, outCh chan<- *adapter.AsyncData) error
	Stop() error
}
