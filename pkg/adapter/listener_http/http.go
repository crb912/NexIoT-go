package listener_http

import (
	"context"
	"devices-iot-go/pkg/model"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time" // Imported for Shutdown timeout
)

const (
	eventName          = "http_alarm_event"
	maxDataChannelSize = 256
	maxBodyBytes       = 1 << 20
)

// PushData holds a raw payload received from an IoT device.

// HttpReceiver passively accepts HTTP POST pushes from IoT devices.
type HttpReceiver struct {
	Host      string
	Port      uint16
	PushUrl   string
	mux       *http.ServeMux
	AsyncData chan model.ReceiveEvent
	server    *http.Server
}

// NewHttpReceiver creates and initializes an HttpReceiver.
// Call Start to begin listening.
func NewHttpReceiver(host string, port uint16, pushPath string) *HttpReceiver {
	mux := http.NewServeMux()
	r := &HttpReceiver{
		Host:      host,
		Port:      port,
		PushUrl:   pushPath,
		mux:       mux,
		AsyncData: make(chan model.ReceiveEvent, maxDataChannelSize),
		// Initialize server here to avoid data race between Start and Stop.
		server: &http.Server{
			Addr:    fmt.Sprintf("%s:%d", host, port),
			Handler: mux,
		},
	}
	r.registerRoutes()
	return r
}

// GetAsyncData returns a read-only channel for consuming push payloads.
//func (r *HttpReceiver) GetAsyncData() <-chan protocol.ReceiveEvent {
//	return r.AsyncData
//}

// Start runs the HTTP server. It blocks until the server stops.
func (r *HttpReceiver) Start() error {
	// This blocks here.
	err := r.server.ListenAndServe()

	// Ignore the normal shutdown error.
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Stop safely shuts down the HTTP server.
func (r *HttpReceiver) Stop() error {
	// Create a new context with a timeout for Shutdown.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := r.server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("manual shutdown: %w", err)
	}
	return nil
}

// registerRoutes attaches all endpoint handlers to the HTTP multiplexer.
func (r *HttpReceiver) registerRoutes() {
	r.mux.HandleFunc(r.PushUrl, r.handlePush)
}

// handlePush accepts a device payload and enqueues it in the data channel.
// Returns 503 when the channel is full so the device can back off and retry.
func (r *HttpReceiver) handlePush(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	req.Body = http.MaxBytesReader(w, req.Body, maxBodyBytes)
	defer req.Body.Close()

	payload, err := io.ReadAll(req.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read body")
		return
	}
	if len(payload) == 0 {
		writeError(w, http.StatusBadRequest, "empty payload")
		return
	}

	select {
	case r.AsyncData <- model.ReceiveEvent{Source: "http", EventName: eventName, EventTime: time.Now(), EventData: payload}:
		writeJSON(w, http.StatusOK, map[string]string{"message": "received"})
	default:
		writeError(w, http.StatusServiceUnavailable, "server busy")
	}
}

// writeJSON serialises payload as JSON and writes it with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// writeError writes a JSON error body {"error": message} with the given status.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
