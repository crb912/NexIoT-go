package connector

import (
	"context"
	"errors"
	"sync"
	"time"

	"octopus-edge/pkg/adapter"
	"octopus-edge/pkg/adapter/receiver"
)

type Receivers struct {
	mu      sync.RWMutex
	timeout time.Duration
	maxSize int
	Servers []ReceiverAdapter
}

// NewReceivers creates and returns a slice of ReceiverAdapter interfaces
func NewReceivers(httpAddress string) *Receivers {
	rc := new(Receivers)
	rc.Servers = make([]ReceiverAdapter, 0)

	// Create a new HTTP receiver instance
	httpRecv := receiver.NewHttpReceiver(httpAddress)
	rc.Servers = append(rc.Servers, httpRecv)
	return rc
}

// StartAll calls Start on every ReceiverAdapter and returns joined errors.
// It attempts to start all receivers; if any fail, callers should call StopAll
// to shut down the receivers that started successfully.
func (rc *Receivers) StartAll(ctx context.Context, outCh chan<- *adapter.AsyncData) error {
	var errs []error
	for _, s := range rc.Servers {
		if err := s.Start(ctx, outCh); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// StopAll calls Stop on every ReceiverAdapter and returns joined errors.
// It always attempts to stop all receivers, even if some fail.
func (rc *Receivers) StopAll() error {
	if rc == nil {
		return nil
	}

	var errs []error
	for _, s := range rc.Servers {
		if err := s.Stop(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
