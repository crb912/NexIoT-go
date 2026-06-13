package connector

import (
	"context"
	"octopus-edge/pkg/protocol"
)

// Session manages the lifecycle of a connection.
type Session interface {
	Connect() error
	Disconnect() error
	IsConnected() bool
}

// Reader defines the standard read interface for all protocol plugins.
type Reader interface {
	ReadSingle(ponit *protocol.Resource) error
	ReadBatch(ponits []*protocol.Resource) error
}

// Writer defines the standard write interface for all protocol plugins.
type Writer interface {
	WriteSingle(ponit *protocol.Resource) error
	WriteBatch(ponits []*protocol.Resource) error // 连续写 n 个点
}

// ReadClient embeds the Reader interface with lifecycle management.
type ReadClient interface {
	Session
	Reader
	GetProtocolType() protocol.ProtocolType
}

// WriteClient embeds the Writer interface with lifecycle management.
type WriteClient interface {
	Session
	Writer
	GetProtocolType() protocol.ProtocolType
}

// RWClient embeds the Reader and Writer interfaces with lifecycle management.
type RWClient interface {
	Session
	Reader
	Writer
	GetProtocolType() protocol.ProtocolType
}

// ReceiverAdapter interface: all passive protocols must implement this
type ReceiverAdapter interface {
	Start(ctx context.Context, outCh chan<- *protocol.AsyncData) error
	Stop() error
}
