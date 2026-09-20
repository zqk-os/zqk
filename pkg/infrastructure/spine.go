package infrastructure

import (
	"context"
)

// Event represents a mutation or signal in the Sovereign Mesh.
type Event struct {
	ObjectID string
	Kind     string
	Op       string
	Payload  map[string]any
	Metadata map[string]any
}

// Handler defines the processing logic for a streamed event.
type Handler func(ctx context.Context, event Event) error

// SpinalSpine defines the industrial-strength transport layer for high-volume kernels.
// It bridges local CAS events to enterprise-grade streams like Kafka or Kinesis.
type SpinalSpine interface {
	// Publish emits an event to the industrial spine.
	Publish(ctx context.Context, event Event) error

	// Subscribe attaches a handler to events of a specific kind.
	Subscribe(ctx context.Context, kind string, handler Handler) error

	// Replay allows catching up on events from a specific sequence number.
	Replay(ctx context.Context, appliedSeq int64, handler Handler) error

	// Close shuts down the connection to the utility.
	Close() error
}

// ProxyMode defines how the local node interacts with the infrastructure.
type ProxyMode string

const (
	ProxyModeTransparent ProxyMode = "transparent_proxy"
	ProxyModeBuffered    ProxyMode = "buffered_bridge"
	ProxyModePassive     ProxyMode = "passive_observer"
)
