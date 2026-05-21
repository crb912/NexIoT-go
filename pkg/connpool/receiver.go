package connpool

import (
	"better-iot-edge/pkg/adapter/receiver"
)

type Receivers struct {
	servers []ReceiverAdapter
}

// NewReceivers creates and returns a slice of ReceiverAdapter interfaces
func NewReceivers(httpAddress string) *Receivers {
	rc := new(Receivers)
	rc.servers = make([]ReceiverAdapter, 0)
	// Create a new HTTP receiver instance
	httpRecv := receiver.NewHttpReceiver(httpAddress)
	rc.servers = append(rc.servers, httpRecv)
	return rc
}
