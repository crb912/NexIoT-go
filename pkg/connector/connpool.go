package connector

import (
	"context"
	"octopus-edge/pkg/model"
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
	ReadSingle(ponit *model.Resource) error
	ReadBatch(ponits []*model.Resource) error
}

// Writer defines the standard write interface for all protocol plugins.
type Writer interface {
	WriteSingle(ponit *model.Resource) error
	WriteBatch(ponits []*model.Resource) error // 连续写 n 个点
}

// ReaderAdapter embeds the Reader interface with lifecycle management.
type ReaderAdapter interface {
	Session
	Reader
	GetProtocolType() protocol.ProtocolType
}

// WriterAdapter embeds the Writer interface with lifecycle management.
type WriterAdapter interface {
	Session
	Writer
	GetProtocolType() protocol.ProtocolType
}

// RWAdapter embeds the Reader and Writer interfaces with lifecycle management.
type RWAdapter interface {
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
