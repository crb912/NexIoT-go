// Package httpclient provides an HTTP protocol adapter implementing the RWClient interface.
// It supports active polling of HTTP-based devices with optional authentication
// (Bearer token, custom header, cookie session, or Basic Auth).
package httpclient

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"next-iot-go/pkg/conv"
	"next-iot-go/pkg/model"
)

// defaultHTTPTimeout is used when no timeout is configured.
const defaultHTTPTimeout = 10 * time.Second

// HttpClient holds connection parameters and the underlying HTTP client.
// It implements the protocol.RWClient interface (Session + Reader + Writer).
type HttpClient struct {
	BaseURL      string
	ProtocolType model.ProtocolType
	Timeout      time.Duration

	// Authentication
	LoginURL  string // e.g. "/api/v1/login"
	LogoutURL string // e.g. "/api/v1/logout"
	Username  string
	Password  string
	AuthType  string // "bearer", "header", "cookie", "basic"
	TokenField string // JSON field name for extracting token from login response
	AuthHeader string // Header name for auth (default "Authorization")

	// HTTP method configuration
	ReadMethod  string // "GET" (default) or "POST"
	WriteMethod string // "POST" (default) or "PUT"

	mu        sync.Mutex
	connected bool
	client    *http.Client
	token     string
}

// NewHttpClient constructs an HttpClient from the generic args map.
// Supported args keys:
//
//	LoginURL       — login endpoint path, e.g. "/api/v1/login"
//	LogoutURL      — logout endpoint path, e.g. "/api/v1/logout"
//	Username        — login username
//	Password        — login password
//	AuthType        — "bearer" (default), "header", "cookie", "basic"
//	TokenField      — JSON field to extract token from login response (default "token")
//	AuthHeader      — custom header name for token (default "Authorization")
//	ReadMethod      — "GET" (default) or "POST"
//	WriteMethod     — "POST" (default) or "PUT"
func NewHttpClient(endpoint string, pt model.ProtocolType, defaultTimeout time.Duration, args map[string]string) (*HttpClient, error) {
	if defaultTimeout <= 0 {
		defaultTimeout = defaultHTTPTimeout
	}

	c := &HttpClient{
		BaseURL:      endpoint,
		ProtocolType: pt,
		Timeout:      defaultTimeout,
		AuthType:     "bearer",
		AuthHeader:   "Authorization",
		TokenField:   "token",
		ReadMethod:   "GET",
		WriteMethod:  "POST",
	}

	if args == nil {
		return c, nil
	}

	if v, ok := args["LoginURL"]; ok {
		c.LoginURL = v
	}
	if v, ok := args["LogoutURL"]; ok {
		c.LogoutURL = v
	}
	if v, ok := args["Username"]; ok {
		c.Username = v
	}
	if v, ok := args["Password"]; ok {
		c.Password = v
	}
	if v, ok := args["AuthType"]; ok && v != "" {
		c.AuthType = v
	}
	if v, ok := args["TokenField"]; ok && v != "" {
		c.TokenField = v
	}
	if v, ok := args["AuthHeader"]; ok && v != "" {
		c.AuthHeader = v
	}
	if v, ok := args["ReadMethod"]; ok && v != "" {
		c.ReadMethod = v
	}
	if v, ok := args["WriteMethod"]; ok && v != "" {
		c.WriteMethod = v
	}

	return c, nil
}

// Connect initializes the HTTP client and performs login if credentials are configured.
func (h *HttpClient) Connect() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.client == nil {
		h.client = &http.Client{
			Timeout: h.Timeout,
		}
	} else if h.connected {
		return nil
	}

	// Perform login if credentials are configured.
	if h.LoginURL != "" && h.Username != "" {
		if err := h.login(); err != nil {
			return fmt.Errorf("HTTP login: %w", err)
		}
	}

	h.connected = true
	return nil
}

// Disconnect performs logout if applicable and marks the client as disconnected.
func (h *HttpClient) Disconnect() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.connected {
		return nil
	}

	if h.LogoutURL != "" && h.token != "" {
		if err := h.logout(); err != nil {
			h.connected = false
			h.token = ""
			return fmt.Errorf("HTTP logout: %w", err)
		}
	}

	h.connected = false
	h.token = ""
	return nil
}

// ReadSingle reads a single resource from the HTTP API endpoint.
// The resource Address is used as the API path.
func (h *HttpClient) ReadSingle(res *model.Resource) error {
	addr, err := toString(res.Address)
	if err != nil {
		return fmt.Errorf("ReadSingle %s: %w", res.Name, err)
	}

	body, err := h.doRequest(h.ReadMethod, addr, nil)
	if err != nil {
		return fmt.Errorf("ReadSingle %s: %w", res.Name, err)
	}

	val, err := parseJSONValue(body, res.Name)
	if err != nil {
		return fmt.Errorf("ReadSingle %s: %w", res.Name, err)
	}

	typed, err := conv.ValueToType(val, res.Type)
	if err != nil {
		return fmt.Errorf("ReadSingle %s: %w", res.Name, err)
	}
	res.Value = typed
	return nil
}

// ReadBatch reads multiple resources sequentially.
// Each resource's Address is used as a separate API path.
func (h *HttpClient) ReadBatch(points []model.Resource) error {
	for i := range points {
		if err := h.ReadSingle(&points[i]); err != nil {
			return err
		}
	}
	return nil
}

// WriteSingle writes a single value to the HTTP API endpoint.
func (h *HttpClient) WriteSingle(res *model.Resource) error {
	addr, err := toString(res.Address)
	if err != nil {
		return fmt.Errorf("WriteSingle %s: %w", res.Name, err)
	}

	payload, err := json.Marshal(res.Value)
	if err != nil {
		return fmt.Errorf("WriteSingle %s: marshal value: %w", res.Name, err)
	}

	if _, err := h.doRequest(h.WriteMethod, addr, payload); err != nil {
		return fmt.Errorf("WriteSingle %s: %w", res.Name, err)
	}
	return nil
}

// WriteBatch writes multiple resources sequentially.
func (h *HttpClient) WriteBatch(points []model.Resource) error {
	for i := range points {
		if err := h.WriteSingle(&points[i]); err != nil {
			return err
		}
	}
	return nil
}

// ─── private helpers ───────────────────────────────────────────────────────

// doRequest performs an HTTP request and returns the raw response body.
func (h *HttpClient) doRequest(method, path string, body []byte) ([]byte, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	url := h.BaseURL + path
	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Set authentication header.
	if h.token != "" {
		switch h.AuthType {
		case "bearer":
			req.Header.Set(h.AuthHeader, "Bearer "+h.token)
		case "header":
			req.Header.Set(h.AuthHeader, h.token)
		case "cookie":
			req.AddCookie(&http.Cookie{Name: h.AuthHeader, Value: h.token})
		case "basic":
			req.SetBasicAuth(h.Username, h.Password)
		}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// login sends POST with credentials and extracts the auth token.
func (h *HttpClient) login() error {
	loginURL := h.BaseURL + h.LoginURL

	creds, err := json.Marshal(map[string]string{
		"username": h.Username,
		"password": h.Password,
	})
	if err != nil {
		return fmt.Errorf("marshal credentials: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, loginURL, bytes.NewReader(creds))
	if err != nil {
		return fmt.Errorf("create login request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read login response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("login failed: HTTP %d: %s", resp.StatusCode, string(respBody))
	}

	// Parse token from JSON response.
	var data map[string]interface{}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return fmt.Errorf("parse login response: %w", err)
	}

	token, ok := data[h.TokenField].(string)
	if !ok {
		return fmt.Errorf("token field %q not found or not a string in login response", h.TokenField)
	}

	h.token = token
	return nil
}

// logout sends a request to the logout endpoint.
func (h *HttpClient) logout() error {
	url := h.BaseURL + h.LogoutURL

	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create logout request: %w", err)
	}

	if h.token != "" {
		switch h.AuthType {
		case "bearer":
			req.Header.Set(h.AuthHeader, "Bearer "+h.token)
		case "header":
			req.Header.Set(h.AuthHeader, h.token)
		case "cookie":
			req.AddCookie(&http.Cookie{Name: h.AuthHeader, Value: h.token})
		case "basic":
			req.SetBasicAuth(h.Username, h.Password)
		}
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return fmt.Errorf("logout request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("logout failed: HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// toString converts the Address field (string or float64 from JSON) to string.
func toString(addr any) (string, error) {
	switch v := addr.(type) {
	case string:
		return v, nil
	case float64:
		return fmt.Sprintf("%.0f", v), nil
	default:
		return "", fmt.Errorf("HTTP address must be string, got %T", addr)
	}
}

// parseJSONValue unmarshals JSON bytes and extracts a single value.
// If the JSON is an object with a field matching fieldName, that field's value is extracted.
// If the JSON is an object with a single field, that field's value is extracted as a fallback.
// If it's an array, it returns the raw slice.
// Otherwise, it returns the unmarshaled value directly.
func parseJSONValue(body []byte, fieldName string) (interface{}, error) {
	if len(body) == 0 {
		return nil, errors.New("empty response body")
	}

	if bytes.HasPrefix(bytes.TrimSpace(body), []byte("{")) ||
		bytes.HasPrefix(bytes.TrimSpace(body), []byte("[")) {
		var result interface{}
		if err := json.Unmarshal(body, &result); err != nil {
			return nil, fmt.Errorf("parse JSON: %w", err)
		}

		// If it's a JSON object, try to extract a field by name, or fall back to the first field.
		if m, ok := result.(map[string]interface{}); ok {
			// Try exact field name match first.
			if v, found := m[fieldName]; found {
				return v, nil
			}
			// Fall back to the first (and only) field if there's just one.
			if len(m) == 1 {
				for _, v := range m {
					return v, nil
				}
			}
			// Multiple fields and no match — return the whole map.
			return result, nil
		}

		return result, nil
	}

	// Plain text — return as string.
	return string(bytes.TrimSpace(body)), nil
}
