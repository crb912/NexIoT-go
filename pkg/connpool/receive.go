package connpool

import "better-iot-edge/pkg/adapter/receiver"

// NewReceivers creates and returns a slice of ReceiverAdapter interfaces
func NewReceivers(httpAddress string) []ReceiverAdapter {
	// Create a new HTTP receiver instance
	httpRecv := receiver.NewHttpReceiver(httpAddress)

	// Put the receiver into a slice and return it
	// This works because *HttpReceiver implements the Receiver interface
	receivers := []ReceiverAdapter{
		httpRecv,
	}
	return receivers
}
