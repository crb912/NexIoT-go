package connector

import (
	"context"
	"fmt"
	"octopus-edge/pkg/protocol"
	"octopus-edge/pkg/protocol/receiver"
	"sync"
	"time"
)

type Receivers struct {
	mu              sync.RWMutex
	timeout         time.Duration
	maxSize         int
	Servers         []ReceiverAdapter
	MergedAsyncData chan protocol.ReceiveEvent // Shared channel for all servers
}

type ReceiveEvent = protocol.ReceiveEvent

// AsyncData defines the unified structure for pushed data
type AsyncData struct {
	EventGUID        string `json:"event_guid"`
	DeviceID         string `json:"device_id"`
	DeviceName       string `json:"device_name"`
	PointID          string `json:"point_id"`
	PointName        string `json:"point_name"`
	PointValue       string `json:"point_value"`
	PointType        int    `json:"point_type"`
	EventTime        int64  `json:"event_time"` // Unix timestamp in milliseconds
	EventType        int    `json:"event_type"`
	EventContent     string `json:"event_content"`
	EventLevel       int    `json:"event_level"`
	EventLocation    string `json:"event_location"`
	EventLocationMsg string `json:"event_location_msg"`
	EventSuggest     string `json:"event_suggest"`
}

// NewReceivers creates and returns a new Receivers instance.
// It initializes the slice and the shared channel to prevent nil pointer panics.
func NewReceivers(timeout time.Duration, maxSize int) *Receivers {
	return &Receivers{
		// sync.RWMutex does not need initialization. The zero value is ready to use.
		timeout: timeout,
		maxSize: maxSize,
		// Initialize an empty slice for servers.
		Servers: make([]ReceiverAdapter, 0),
		// Create a buffered channel.
		MergedAsyncData: make(chan protocol.ReceiveEvent, maxSize),
	}
}

func (rc *Receivers) RegisterHttpServer(host string, port uint16, urlHandle string, ch chan ReceiveEvent) {
	server := receiver.NewHttpReceiver(host, port, urlHandle)

	// Override the server's AsyncData channel with the shared one
	server.AsyncData = ch
	rc.Servers = append(rc.Servers, server)
}

// StartAll calls Start on every ReceiverAdapter in parallel.
func (rc *Receivers) StartAll(ctx context.Context) error {
	for i := range rc.Servers {
		go rc.Servers[i].Start()
	}

	return nil
}

// StopAll calls Stop on every ReceiverAdapter and returns joined errors.
func (rc *Receivers) StopAll() error {
	if rc == nil {
		return nil
	}

	rc.mu.Lock()
	defer rc.mu.Unlock()

	var errs []error
	for _, s := range rc.Servers {
		if err := s.Stop(); err != nil {
			errs = append(errs, err)
		}
	}

	// In standard library error handling can join multiple errors (Go 1.20+)
	if len(errs) > 0 {
		importErrors := fmt.Errorf("stop errors occurred: %v", errs)
		return importErrors
	}
	return nil
}
