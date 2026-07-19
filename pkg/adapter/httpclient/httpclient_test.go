package httpclient

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"next-iot-go/pkg/model"
)

// newTestServer starts an httptest.Server that simulates an HTTP device API.
// Returns the server and a cleanup function.
func newTestServer() (*httptest.Server, *testDeviceState) {
	s := &testDeviceState{
		temperature: 25.0,
		humidity:    60.0,
		setpoint:    22.0,
		tokens:      make(map[string]*http.Cookie),
	}

	mux := http.NewServeMux()

	// Login
	mux.HandleFunc("/api/v1/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var creds map[string]string
		if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if creds["username"] != "admin" || creds["password"] != "admin123" {
			http.Error(w, "unauthorized", 401)
			return
		}
		token := "test-token-12345"
		cookie := &http.Cookie{Name: "session", Value: token}
		s.tokens[token] = cookie
		json.NewEncoder(w).Encode(map[string]string{"token": token})
	})

	// Logout
	mux.HandleFunc("/api/v1/logout", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"message": "logged out"})
	})

	// Temperature
	mux.HandleFunc("/api/v1/temperature", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		json.NewEncoder(w).Encode(map[string]float64{"temperature": s.temperature})
	})

	// Humidity
	mux.HandleFunc("/api/v1/humidity", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		json.NewEncoder(w).Encode(map[string]float64{"humidity": s.humidity})
	})

	// Status
	mux.HandleFunc("/api/v1/status", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", 405)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// Setpoint
	mux.HandleFunc("/api/v1/setpoint", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", 405)
			return
		}
		var body interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", 400)
			return
		}
		if m, ok := body.(map[string]interface{}); ok {
			if sp, ok := m["setpoint"].(float64); ok {
				s.setpoint = sp
			}
		} else if sp, ok := body.(float64); ok {
			s.setpoint = sp
		}
		json.NewEncoder(w).Encode(map[string]float64{"setpoint": s.setpoint})
	})

	return httptest.NewServer(mux), s
}

// testDeviceState holds the mutable state of the test HTTP server.
type testDeviceState struct {
	temperature float64
	humidity    float64
	setpoint    float64
	tokens      map[string]*http.Cookie
}

// newTestClient creates an HttpClient pointed at the given server with auth config.
func newTestClient(serverURL string, authType string) *HttpClient {
	args := map[string]string{
		"LoginURL":   "/api/v1/login",
		"LogoutURL":  "/api/v1/logout",
		"Username":   "admin",
		"Password":   "admin123",
		"AuthType":   authType,
		"TokenField": "token",
		"AuthHeader": "Authorization",
		"ReadMethod": "GET",
		"WriteMethod": "POST",
	}
	c, _ := NewHttpClient(serverURL, model.HTTPClient, 5*time.Second, args)
	return c
}

func TestNewHttpClient(t *testing.T) {
	c, err := NewHttpClient("http://127.0.0.1:9000", model.HTTPClient, 10*time.Second, map[string]string{
		"LoginURL": "/api/login",
	})
	if err != nil {
		t.Fatalf("NewHttpClient: %v", err)
	}
	if c.BaseURL != "http://127.0.0.1:9000" {
		t.Errorf("BaseURL = %q, want %q", c.BaseURL, "http://127.0.0.1:9000")
	}
	if c.LoginURL != "/api/login" {
		t.Errorf("LoginURL = %q, want %q", c.LoginURL, "/api/login")
	}
}

func TestNewHttpClientNilArgs(t *testing.T) {
	c, err := NewHttpClient("http://localhost:8080", model.HTTPClient, 0, nil)
	if err != nil {
		t.Fatalf("NewHttpClient with nil args: %v", err)
	}
	if c.Timeout != defaultHTTPTimeout {
		t.Errorf("default timeout = %v, want %v", c.Timeout, defaultHTTPTimeout)
	}
}

func TestConnectAndDisconnect(t *testing.T) {
	server, _ := newTestServer()
	defer server.Close()

	c := newTestClient(server.URL, "bearer")
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !c.connected {
		t.Error("expected connected=true")
	}
	if c.token != "test-token-12345" {
		t.Errorf("token = %q, want %q", c.token, "test-token-12345")
	}

	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	if c.connected {
		t.Error("expected connected=false")
	}
}

func TestReadSingle(t *testing.T) {
	server, _ := newTestServer()
	defer server.Close()

	c := newTestClient(server.URL, "bearer")
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Disconnect()

	res := model.Resource{
		Name:    "Temperature",
		Address: "/api/v1/temperature",
		Type:    "Float64",
	}
	if err := c.ReadSingle(&res); err != nil {
		t.Fatalf("ReadSingle: %v", err)
	}
	if res.Value == nil {
		t.Fatal("expected non-nil value")
	}
	// The response is {"temperature": 25.0} → a map[string]interface{}
	// ValueToType converts float64(25.0) to float64
	if v, ok := res.Value.(float64); !ok {
		t.Errorf("expected float64, got %T: %v", res.Value, res.Value)
	} else if v < 20 || v > 30 {
		t.Errorf("temperature out of range: %f", v)
	}
}

func TestReadBatch(t *testing.T) {
	server, _ := newTestServer()
	defer server.Close()

	c := newTestClient(server.URL, "bearer")
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Disconnect()

	points := []model.Resource{
		{Name: "Temperature", Address: "/api/v1/temperature", Type: "Float64"},
		{Name: "Humidity", Address: "/api/v1/humidity", Type: "Float64"},
		{Name: "Status", Address: "/api/v1/status", Type: "String"},
	}
	if err := c.ReadBatch(points); err != nil {
		t.Fatalf("ReadBatch: %v", err)
	}
	for i := range points {
		if points[i].Value == nil {
			t.Errorf("points[%d].Value is nil", i)
		}
	}
}

func TestWriteSingle(t *testing.T) {
	server, state := newTestServer()
	defer server.Close()

	c := newTestClient(server.URL, "bearer")
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Disconnect()

	res := model.Resource{
		Name:    "Setpoint",
		Address: "/api/v1/setpoint",
		Type:    "Float64",
		Value:   26.5,
	}
	if err := c.WriteSingle(&res); err != nil {
		t.Fatalf("WriteSingle: %v", err)
	}
	if state.setpoint != 26.5 {
		t.Errorf("setpoint = %f, want 26.5", state.setpoint)
	}
}

func TestWriteBatch(t *testing.T) {
	server, state := newTestServer()
	defer server.Close()

	c := newTestClient(server.URL, "bearer")
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer c.Disconnect()

	points := []model.Resource{
		{Name: "Setpoint", Address: "/api/v1/setpoint", Type: "Float64", Value: 24.0},
	}
	if err := c.WriteBatch(points); err != nil {
		t.Fatalf("WriteBatch: %v", err)
	}
	if state.setpoint != 24.0 {
		t.Errorf("setpoint = %f, want 24.0", state.setpoint)
	}
}

func TestDoubleConnect(t *testing.T) {
	server, _ := newTestServer()
	defer server.Close()

	c := newTestClient(server.URL, "bearer")
	if err := c.Connect(); err != nil {
		t.Fatalf("first Connect: %v", err)
	}
	// Second Connect should be a no-op.
	if err := c.Connect(); err != nil {
		t.Fatalf("second Connect: %v", err)
	}
	c.Disconnect()
}

func TestReadSingleUnauthorized(t *testing.T) {
	// Create a client without login credentials.
	args := map[string]string{
		"ReadMethod": "GET",
	}
	c, err := NewHttpClient("http://127.0.0.1:9000", model.HTTPClient, 5*time.Second, args)
	if err != nil {
		t.Fatalf("NewHttpClient: %v", err)
	}
	// No login configured, so connect is just creating the http.Client.
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if c.token != "" {
		t.Errorf("expected empty token, got %q", c.token)
	}
	c.Disconnect()
}

func TestToString(t *testing.T) {
	tests := []struct {
		input  any
		want   string
		wantOK bool
	}{
		{"/api/v1/status", "/api/v1/status", true},
		{float64(42), "42", true},
		{42, "", false},
		{nil, "", false},
	}
	for _, tc := range tests {
		got, err := toString(tc.input)
		if tc.wantOK && err != nil {
			t.Errorf("toString(%v) error: %v", tc.input, err)
		}
		if !tc.wantOK && err == nil {
			t.Errorf("toString(%v) expected error", tc.input)
		}
		if got != tc.want {
			t.Errorf("toString(%v) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestParseJSONValue(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		fieldName string
	}{
		{"object with matching field", `{"temperature": 25.0}`, "temperature"},
		{"object with single field fallback", `{"temp": 30.0}`, "notfound"},
		{"array", `[1, 2, 3]`, ""},
		{"plain text", "ok", ""},
		{"number", "42", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v, err := parseJSONValue([]byte(tc.input), tc.fieldName)
			if err != nil {
				t.Errorf("parseJSONValue(%q): %v", tc.input, err)
			}
			if v == nil {
				t.Errorf("parseJSONValue(%q): nil result", tc.input)
			}
		})
	}

	if _, err := parseJSONValue([]byte(""), ""); err == nil {
		t.Error("expected error for empty body")
	}
}

func TestParseJSONValueExtractField(t *testing.T) {
	// Exact field match
	v, err := parseJSONValue([]byte(`{"temperature": 25.0}`), "temperature")
	if err != nil {
		t.Fatalf("parseJSONValue: %v", err)
	}
	if f, ok := v.(float64); !ok || f != 25.0 {
		t.Errorf("expected 25.0, got %v", v)
	}

	// Single field fallback
	v, err = parseJSONValue([]byte(`{"temp": 30.0}`), "nonexistent")
	if err != nil {
		t.Fatalf("parseJSONValue: %v", err)
	}
	if f, ok := v.(float64); !ok || f != 30.0 {
		t.Errorf("expected 30.0, got %v", v)
	}

	// Multi-field, no match — returns whole map
	v, err = parseJSONValue([]byte(`{"a": 1, "b": 2}`), "c")
	if err != nil {
		t.Fatalf("parseJSONValue: %v", err)
	}
	if _, ok := v.(map[string]interface{}); !ok {
		t.Errorf("expected map, got %T", v)
	}
}
