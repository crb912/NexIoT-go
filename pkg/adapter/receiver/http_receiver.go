package receiver

import (
	"context"
	"encoding/json"
	"errors"
	"hermes-edge/pkg/adapter"
	"net/http"
)

// HttpReceiver implements the Receiver interface
type HttpReceiver struct {
	server *http.Server
}

func NewHttpReceiver(address string) *HttpReceiver {
	return &HttpReceiver{
		server: &http.Server{Addr: address},
	}
}

func (h *HttpReceiver) Start(ctx context.Context, outCh chan<- *adapter.AsyncData) error {
	mux := http.NewServeMux()

	// Listen on /api/v1/push endpoint
	mux.HandleFunc("/api/v1/push", func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Extract device info from payload
		data := &adapter.AsyncData{
			DeviceName:   payload["device"].(string),
			ResourceName: payload["resource"].(string),
			Value:        payload["value"],
		}

		// Write data to channel without blocking
		select {
		case outCh <- data:
			w.WriteHeader(http.StatusOK)
		case <-ctx.Done():
			// Gateway is shutting down
			http.Error(w, "Gateway is shutting down", http.StatusServiceUnavailable)
		default:
			// Data queue is full (implementing Backpressure)
			http.Error(w, "Data queue is full", http.StatusTooManyRequests)
		}
	})

	h.server.Handler = mux

	// Start HTTP server in the background
	go func() {
		if err := h.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			// Add your logger here
		}
	}()

	return nil
}

func (h *HttpReceiver) Stop() error {
	return h.server.Shutdown(context.Background())
}
