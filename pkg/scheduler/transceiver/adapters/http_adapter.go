package adapters

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/httpheaders"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/scheduler/transceiver/types"
)

const emptyValue = ""

// HTTPAdapter implements the ProtocolAdapter interface for HTTP/webhook communication
type HTTPAdapter struct {
	client *http.Client
	logger logging.Logger
}

// NewHTTPAdapter creates a new HTTP adapter
func NewHTTPAdapter(logger logging.Logger) *HTTPAdapter {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &HTTPAdapter{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		logger: logger,
	}
}

// Name returns the protocol name
func (a *HTTPAdapter) Name() string {
	return "webhook"
}

// Validate validates action configuration for HTTP protocol
func (a *HTTPAdapter) Validate(action types.Action) error {
	if action.Endpoint == emptyValue {
		return errfmt.Errorf("endpoint required for webhook protocol")
	}
	// Basic URL validation
	if !strings.HasPrefix(action.Endpoint, "http://") && !strings.HasPrefix(action.Endpoint, "https://") {
		return errfmt.Errorf("endpoint must be a valid HTTP/HTTPS URL")
	}
	return nil
}

// Send sends a message via HTTP POST
//
//nolint:gocritic // Message passed by value to avoid mutation during routing
func (a *HTTPAdapter) Send(ctx context.Context, message types.Message, action types.Action) error {
	// Marshal message payload to JSON
	jsonData, err := json.Marshal(message.Payload)
	if err != nil {
		return errfmt.Newf("failed to marshal message").Wrap(err)
	}

	// Create HTTP request with context
	req, err := http.NewRequestWithContext(ctx, "POST", action.Endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return errfmt.Newf("failed to create request").Wrap(err)
	}

	// Set headers
	req.Header.Set(httpheaders.ContentType, "application/json")
	req.Header.Set(httpheaders.UserAgent, "ZQK-Transceiver/1.0")
	req.Header.Set(httpheaders.XEventType, message.EventType)
	req.Header.Set(httpheaders.XSource, message.Source)
	req.Header.Set(httpheaders.XTimestamp, message.Timestamp.Format(time.RFC3339))

	// Add metadata as headers
	for k, v := range message.Metadata {
		// Convert key to header format (e.g., "job_id" -> "X-Metadata-Job-Id")
		headerKey := "X-Metadata-" + a.toHeaderCase(k)
		req.Header.Set(headerKey, v)
	}

	// Apply authentication if configured
	if action.Auth != nil {
		a.applyAuth(req, action.Auth)
	}

	// Add custom headers from auth config
	if action.Auth != nil && action.Auth.Headers != nil {
		for k, v := range action.Auth.Headers {
			req.Header.Set(k, v)
		}
	}

	// Execute request
	resp, err := a.client.Do(req)
	if err != nil {
		return errfmt.Newf("webhook request failed").Wrap(err)
	}
	defer resp.Body.Close()

	// Read response body (limit to 1KB)
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body) //nolint:errcheck // EOF is expected, partial read is acceptable
	responseBody := string(body[:n])

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		logging.Fluent(a.logger).Debug(LogEventSchedulerTransceiverHTTPWebhookSucceeded).
			URL(action.Endpoint).
			HTTPStatusCode(resp.StatusCode).
			Log()
		return nil
	}

	return errfmt.Errorf("webhook returned non-2xx status: %d, response: %s", resp.StatusCode, responseBody)
}

// applyAuth applies authentication to the HTTP request
func (a *HTTPAdapter) applyAuth(req *http.Request, auth *types.AuthConfig) {
	switch auth.Type {
	case "bearer":
		token := a.resolveCredentials(auth.Credentials)
		req.Header.Set(httpheaders.Authorization, "Bearer "+token)
	case "basic":
		// Basic auth would need username:password format
		// For now, just use credentials as token
		token := a.resolveCredentials(auth.Credentials)
		req.Header.Set(httpheaders.Authorization, "Basic "+token)
	case "api_key":
		// API key in header (default: X-API-Key)
		key := a.resolveCredentials(auth.Credentials)
		req.Header.Set(httpheaders.XAPIKey, key)
	case "jwt":
		token := a.resolveCredentials(auth.Credentials)
		req.Header.Set(httpheaders.Authorization, "Bearer "+token)
	default:
		logging.Fluent(a.logger).Warn(LogEventSchedulerTransceiverHTTPUnknownAuth).
			AuthType(auth.Type).
			Log()
	}
}

// resolveCredentials resolves credential references (env vars, secrets, etc.)
// For now, simple implementation - can be enhanced later
func (a *HTTPAdapter) resolveCredentials(credRef string) string {
	// If it starts with $, treat as env var
	if strings.HasPrefix(credRef, "$") {
		envVar := strings.TrimPrefix(credRef, "$")
		if val := os.Getenv(envVar); val != emptyValue {
			return val
		}
	}

	// If it contains :, treat as secret:key format
	if strings.Contains(credRef, ":") {
		parts := strings.SplitN(credRef, ":", 2)
		if len(parts) == 2 && parts[0] == "secret" {
			// TODO: Integrate with secret management system
			// For now, return as-is (would need secret lookup)
			return credRef
		}
	}

	// Otherwise, return as-is (direct credential - not recommended)
	return credRef
}

// toHeaderCase converts a string to header case (e.g., "job_id" -> "Job-Id")
func (a *HTTPAdapter) toHeaderCase(s string) string {
	parts := strings.Split(s, "_")
	for i, part := range parts {
		if part != emptyValue {
			parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	return strings.Join(parts, "-")
}
