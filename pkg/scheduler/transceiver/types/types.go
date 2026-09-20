package types

import (
	"context"
	"time"
)

// Message represents a protocol-agnostic message
type Message struct {
	EventType   string            // Semantic event type (e.g., "scheduler_job_completed")
	Source      string            // Source identifier (e.g., "scheduler", "external")
	Destination string            // Destination identifier (optional, for direct routing)
	Timestamp   time.Time         // Message timestamp
	Payload     map[string]any    // Message payload
	Metadata    map[string]string // Additional metadata (job_id, category, etc.)
}

// Action defines what happens when a route matches
type Action struct {
	Protocol  string            // webhook, grpc, websocket, queue, event, command
	Endpoint  string            // Protocol-specific endpoint (URL, command, event name, etc.)
	Transform *PayloadTransform // Optional payload transformation
	Auth      *AuthConfig       // Authentication configuration
	Retry     *RetryConfig      // Retry configuration
	Timeout   time.Duration     // Action timeout (0 = use default)
}

// PayloadTransform defines how to transform the message payload
type PayloadTransform struct {
	IncludeFields []string          // Fields to include (empty = all)
	ExcludeFields []string          // Fields to exclude
	AddFields     map[string]any    // Fields to add
	RenameFields  map[string]string // Field renames (old -> new)
}

// AuthConfig defines authentication for the action
type AuthConfig struct {
	Type        string            // bearer, basic, jwt, x509, api_key, oauth2
	Credentials string            // Credential reference (env var, secret key, etc.)
	Headers     map[string]string // Additional headers
}

// RetryConfig defines retry behavior
type RetryConfig struct {
	MaxAttempts  int           // Maximum retry attempts (0 = no retries)
	Backoff      string        // exponential, linear, fixed
	InitialDelay time.Duration // Initial delay before first retry
	MaxDelay     time.Duration // Maximum delay between retries
}

// ProtocolAdapter defines the interface for protocol implementations
type ProtocolAdapter interface {
	// Send sends a message via this protocol
	Send(ctx context.Context, message Message, action Action) error

	// Name returns the protocol name (e.g., "webhook", "grpc", "command")
	Name() string

	// Validate validates action configuration for this protocol
	Validate(action Action) error
}
