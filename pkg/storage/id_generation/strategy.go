package id_generation

import (
	"context"
)

// Strategy defines the interface for ID generation strategies
// Each strategy generates IDs according to its specific algorithm
type Strategy interface {
	// GenerateNextID generates the next ID for an object kind
	// Returns the generated ID or an error
	GenerateNextID(ctx context.Context, kind string, prefix string, existingIDs []string) (string, error)

	// Name returns the strategy name (e.g., "sequential", "uuid", "timestamp")
	Name() string

	// Description returns a human-readable description of the strategy
	Description() string
}

// StrategyConfig holds configuration for a specific strategy
type StrategyConfig struct {
	// Strategy name (e.g., "sequential", "uuid", "timestamp")
	Strategy string `yaml:"strategy"`

	// Strategy-specific parameters (e.g., {"min_digits": 3, "start_at": 1})
	Params map[string]any `yaml:"params,omitempty"`

	// BufferSize is the target size for the pre-filled ID queue
	// Higher values reduce contention but use more memory
	// Default: 100 for low-volume, 500 for high-volume
	BufferSize int `yaml:"buffer_size,omitempty"`
}

// DefaultStrategyConfig returns the default strategy configuration (sequential)
func DefaultStrategyConfig() StrategyConfig {
	return StrategyConfig{
		Strategy: "sequential",
		Params: map[string]any{
			"min_digits": 3,
			"start_at":   1,
		},
	}
}
