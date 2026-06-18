// Package adapter provides HTTP protocol adapters for passively receiving IoT device data.
package adapter

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

const (
	// maxDataChannelSize is the maximum number of push payloads buffered in the channel.
	maxDataChannelSize = 256

	// maxBodyBytes limits incoming request bodies to 1 MiB to prevent abuse.
	maxBodyBytes = 1 << 20

	// tokenHeader is the HTTP header key that carries session tokens.
	tokenHeader = "X-Auth-Token"
)

// HttpServerConfig holds all parameters needed to initialize an HttpReceiver.
type HttpServerConfig struct {
	Host              string        // bind address, e.g. "0.0.0.0"
	Port              int           // listen port, e.g. 8080
	Username          string        // shared credential username for device auth
	Password          string        // shared credential password for device auth
	HeartbeatInterval time.Duration // time after which a silent client is marked offline
	LoginPath         string        // endpoint path for device login, e.g. "/login"
	HeartbeatPath     string        // endpoint path for heartbeat, e.g. "/heartbeat"
	PushPath          string        // endpoint path for data push, e.g. "/push"
	ReadTimeout       time.Duration // HTTP server read timeout (defaults to 15s)
	WriteTimeout      time.Duration // HTTP server write timeout (defaults to 15s)
}

// ClientSession tracks the runtime state of one connected IoT device.
type ClientSession struct {
	Token        string
	DeviceID     string
	LastSeen     time.Time
	IsOnline     bool
	OfflineSince time.Time // set when the client first misses its heartbeat
}

// PushData wraps a raw device payload with metadata for downstream consumers.
type PushData struct {
	DeviceID  string
	Payload   []byte
	Timestamp time.Time
}

// OfflineCallback is invoked (in a separate goroutine) when a device goes offline.
type OfflineCallback func(deviceID string, lastSeen time.Time)

// loginRequest is the expected JSON body on the login endpoint.
type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	DeviceID string `json:"device_id"`
}

// loginResponse is the JSON body returned after a successful login.
type loginResponse struct {
	Token   string `json:"token"`
	Message string `json:"message"`
}

// -----------------------------------------------------------------------
// HttpReceiver
// -----------------------------------------------------------------------

// HttpReceiver is an HTTP-based protocol adapter that passively accepts IoT
// device connections, heartbeats, and data pushes.
type HttpReceiver struct {
	config    HttpServerConfig
	server    *http.Server
	mux       *http.ServeMux
	clients   map[string]*ClientSession // token -> session
	mu        sync.RWMutex
	dataChan  chan PushData
	stopChan  chan struct{}
	closeOnce sync.Once // ensures stopChan is closed exactly once
	logger    *log.Logger
	wg        sync.WaitGroup
	onOffline OfflineCallback
}

// NewHttpReceiver creates and returns a fully initialized HttpReceiver.
// Call Start to begin listening.
func NewHttpReceiver(config HttpServerConfig) *HttpReceiver {
	logger := log.New(os.Stdout, "[HttpReceiver] ", log.LstdFlags|log.Lshortfile)

	r := &HttpReceiver{
		config:   config,
		mux:      http.NewServeMux(),
		clients:  make(map[string]*ClientSession),
		dataChan: make(chan PushData, maxDataChannelSize),
		stopChan: make(chan struct{}),
		logger:   logger,
	}

	r.registerRoutes()

	r.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Host, config.Port),
		Handler:      r.mux,
		ReadTimeout:  orDefault(config.ReadTimeout, 15*time.Second),
		WriteTimeout: orDefault(config.WriteTimeout, 15*time.Second),
	}

	return r
}

// SetOfflineCallback registers a function called whenever a device goes offline.
// It is safe to call before or after Start.
func (r *HttpReceiver) SetOfflineCallback(cb OfflineCallback) {
	r.onOffline = cb
}

// DataChannel returns a read-only channel for consuming device push payloads.
// The caller should range over this channel in a separate goroutine.
func (r *HttpReceiver) DataChannel() <-chan PushData {
	return r.dataChan
}

// OnlineClientCount returns the number of currently online device sessions.
func (r *HttpReceiver) OnlineClientCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, s := range r.clients {
		if s.IsOnline {
			n++
		}
	}
	return n
}

// -----------------------------------------------------------------------
// Lifecycle
// -----------------------------------------------------------------------

// Start launches the HTTP server and the background heartbeat checker.
// It returns immediately; the server runs in separate goroutines.
func (r *HttpReceiver) Start() error {
	r.logger.Printf("starting HTTP receiver on %s", r.server.Addr)

	r.wg.Add(1)
	go r.runHeartbeatChecker()

	r.wg.Add(1)
	go r.serve()

	return nil
}

// Stop gracefully shuts down the HTTP server and all background workers,
// marks every tracked client as offline, and closes the data channel.
func (r *HttpReceiver) Stop() error {
	r.logger.Println("stopping HTTP receiver...")

	// signal all background goroutines
	r.closeOnce.Do(func() { close(r.stopChan) })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := r.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown: %w", err)
	}

	r.wg.Wait()
	r.markAllClientsOffline()
	close(r.dataChan)

	r.logger.Println("HTTP receiver stopped")
	return nil
}

// serve runs ListenAndServe and notifies the wait group when it exits.
func (r *HttpReceiver) serve() {
	defer r.wg.Done()
	if err := r.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		r.logger.Printf("server error: %v", err)
	}
}

// -----------------------------------------------------------------------
// Route registration
// -----------------------------------------------------------------------

// registerRoutes attaches all endpoint handlers to the HTTP multiplexer.
func (r *HttpReceiver) registerRoutes() {
	r.mux.HandleFunc(r.config.LoginPath, r.handleLogin)
	r.mux.HandleFunc(r.config.HeartbeatPath, r.authMiddleware(r.handleHeartbeat))
	r.mux.HandleFunc(r.config.PushPath, r.authMiddleware(r.handlePush))
	r.mux.HandleFunc("/logout", r.authMiddleware(r.handleLogout))
	r.mux.HandleFunc("/health", r.handleHealth)
}

// -----------------------------------------------------------------------
// Handlers
// -----------------------------------------------------------------------

// handleLogin authenticates a device and issues a session token.
// POST body: { "username", "password", "device_id" }
// Response:  { "token", "message" }
func (r *HttpReceiver) handleLogin(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	req.Body = http.MaxBytesReader(w, req.Body, maxBodyBytes)
	defer req.Body.Close()

	var creds loginRequest
	if err := json.NewDecoder(req.Body).Decode(&creds); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if !r.credentialsValid(creds.Username, creds.Password) {
		r.logger.Printf("login failed for device: %s", creds.DeviceID)
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}

	token, err := generateToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	r.upsertSession(token, creds.DeviceID)
	r.logger.Printf("device logged in: %s", creds.DeviceID)
	writeJSON(w, http.StatusOK, loginResponse{Token: token, Message: "login successful"})
}

// handleHeartbeat refreshes the last-seen timestamp for an authenticated device.
// Accepts GET or POST; the token is read from the X-Auth-Token header.
func (r *HttpReceiver) handleHeartbeat(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet && req.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	token := req.Header.Get(tokenHeader)

	r.mu.Lock()
	if s, ok := r.clients[token]; ok {
		s.LastSeen = time.Now()
		s.IsOnline = true
	}
	r.mu.Unlock()

	writeJSON(w, http.StatusOK, map[string]string{"message": "ok"})
}

// handlePush accepts a device payload and enqueues it in the data channel.
// POST body: raw device data (JSON, binary, etc.)
// When the channel is full the request is rejected with 503.
func (r *HttpReceiver) handlePush(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	req.Body = http.MaxBytesReader(w, req.Body, maxBodyBytes)
	defer req.Body.Close()

	deviceID := r.deviceIDFromToken(req.Header.Get(tokenHeader))
	if deviceID == "" {
		writeError(w, http.StatusUnauthorized, "session not found")
		return
	}

	payload, err := io.ReadAll(req.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read body")
		return
	}
	if len(payload) == 0 {
		writeError(w, http.StatusBadRequest, "empty payload")
		return
	}

	data := PushData{
		DeviceID:  deviceID,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	select {
	case r.dataChan <- data:
		r.logger.Printf("enqueued %d bytes from device %s", len(payload), deviceID)
		writeJSON(w, http.StatusOK, map[string]string{"message": "received"})
	default:
		r.logger.Printf("channel full, dropping push from device %s", deviceID)
		writeError(w, http.StatusServiceUnavailable, "server busy")
	}
}

// handleLogout removes the device session and frees its resources.
// POST; the token is read from the X-Auth-Token header.
func (r *HttpReceiver) handleLogout(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	token := req.Header.Get(tokenHeader)

	r.mu.Lock()
	s, ok := r.clients[token]
	if ok {
		delete(r.clients, token)
	}
	r.mu.Unlock()

	if ok {
		r.logger.Printf("device logged out: %s", s.DeviceID)
	}
	writeJSON(w, http.StatusOK, map[string]string{"message": "logged out"})
}

// handleHealth returns a simple liveness response with the current online client count.
func (r *HttpReceiver) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":         "ok",
		"online_clients": r.OnlineClientCount(),
	})
}

// -----------------------------------------------------------------------
// Middleware
// -----------------------------------------------------------------------

// authMiddleware rejects requests that lack a valid session token.
func (r *HttpReceiver) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		token := req.Header.Get(tokenHeader)
		if token == "" {
			writeError(w, http.StatusUnauthorized, "missing auth token")
			return
		}

		r.mu.RLock()
		_, ok := r.clients[token]
		r.mu.RUnlock()

		if !ok {
			writeError(w, http.StatusUnauthorized, "invalid or expired token")
			return
		}

		next(w, req)
	}
}

// -----------------------------------------------------------------------
// Heartbeat checker
// -----------------------------------------------------------------------

// runHeartbeatChecker ticks at every HeartbeatInterval and checks device liveness.
func (r *HttpReceiver) runHeartbeatChecker() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.config.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.checkClientHeartbeats()
		case <-r.stopChan:
			r.logger.Println("heartbeat checker stopped")
			return
		}
	}
}

// checkClientHeartbeats marks clients offline when they exceed the heartbeat
// deadline and removes stale sessions after a second full interval.
func (r *HttpReceiver) checkClientHeartbeats() {
	now := time.Now()
	offlineDeadline := now.Add(-r.config.HeartbeatInterval)
	cleanupDeadline := now.Add(-2 * r.config.HeartbeatInterval)

	r.mu.Lock()
	defer r.mu.Unlock()

	for token, s := range r.clients {
		switch {
		case s.IsOnline && s.LastSeen.Before(offlineDeadline):
			// client has gone quiet — mark offline
			s.IsOnline = false
			s.OfflineSince = now
			r.logger.Printf("device offline: %s (last seen: %s)",
				s.DeviceID, s.LastSeen.Format(time.RFC3339))
			if r.onOffline != nil {
				go r.onOffline(s.DeviceID, s.LastSeen)
			}

		case !s.IsOnline && s.OfflineSince.Before(cleanupDeadline):
			// session has been offline for two full intervals — clean it up
			delete(r.clients, token)
			r.logger.Printf("stale session removed: %s", s.DeviceID)
		}
	}
}

// markAllClientsOffline sets every session to offline during shutdown.
func (r *HttpReceiver) markAllClientsOffline() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, s := range r.clients {
		if s.IsOnline {
			s.IsOnline = false
			r.logger.Printf("device marked offline on shutdown: %s", s.DeviceID)
		}
	}
}

// -----------------------------------------------------------------------
// Session helpers
// -----------------------------------------------------------------------

// credentialsValid compares username and password against the config.
func (r *HttpReceiver) credentialsValid(username, password string) bool {
	return username == r.config.Username && password == r.config.Password
}

// upsertSession creates a session for the given device, removing any prior
// session for the same device to prevent token leaks on re-login.
func (r *HttpReceiver) upsertSession(token, deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// invalidate any existing session for this device
	for t, s := range r.clients {
		if s.DeviceID == deviceID {
			delete(r.clients, t)
			break
		}
	}

	r.clients[token] = &ClientSession{
		Token:    token,
		DeviceID: deviceID,
		LastSeen: time.Now(),
		IsOnline: true,
	}
}

// deviceIDFromToken returns the DeviceID for the given token, or "" if not found.
func (r *HttpReceiver) deviceIDFromToken(token string) string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if s, ok := r.clients[token]; ok {
		return s.DeviceID
	}
	return ""
}

// -----------------------------------------------------------------------
// Package-level helpers
// -----------------------------------------------------------------------

// generateToken returns a cryptographically random 32-hex-char string.
func generateToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// orDefault returns d when d > 0, otherwise returns fallback.
func orDefault(d, fallback time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return fallback
}

// writeJSON serialises payload as JSON and writes it with the given HTTP status.
func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}

// writeError writes a JSON error body { "error": message } with the given status.
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
