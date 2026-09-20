package intent

import (
	"context"

	"github.com/zqk-os/zqk/pkg/orchestration"
)

// IntentMapper defines the contract for translating events into actionable intents.
type IntentMapper interface {
	Map(ctx context.Context, eventID string, payload map[string]any) (orchestration.RawIntent, error)
}

// DefaultIntentMapper provides a robust, rule-based mapping engine.
type DefaultIntentMapper struct{}

func NewDefaultIntentMapper() *DefaultIntentMapper {
	return &DefaultIntentMapper{}
}

func (m *DefaultIntentMapper) Map(ctx context.Context, eventID string, payload map[string]any) (orchestration.RawIntent, error) {
	// Logic to translate event properties to a RawIntent
	// Based on the 'Capability' object specification:
	// Intent must contain a clear signature, priority, and metadata for synthesis.
	return orchestration.RawIntent{
		Signature: eventID,
		Payload:   payload,
		Priority:  1, // Default priority
	}, nil
}
