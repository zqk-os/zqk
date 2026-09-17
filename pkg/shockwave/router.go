package shockwave

import (
	"time"
)

// ShockwaveRouter is the primary struct for distributed Shockwave Protocol mesh routing.
// Implements requirements defined in [REDACTED-ID].
type ShockwaveRouter struct {
	NodeID         string
	Rules          PropagationRules
	Splitters      Splitters
	Duplicators    Duplicators
	ReduceStrategy ReduceStrategy
}

// Splitters configures how messages are split into chunks.
type Splitters struct {
	ChunkSize int
	MaxChunks int
}

// Duplicators configures message redundancy across the mesh.
type Duplicators struct {
	RedundancyFactor int
	Enabled          bool
}

// ReduceStrategy configures how the mesh reduces/reassembles incoming messages.
type ReduceStrategy struct {
	RequireAll bool
	Timeout    time.Duration
}

// NewShockwaveRouter creates a new instance of ShockwaveRouter with default settings.
func NewShockwaveRouter(nodeID string) *ShockwaveRouter {
	return &ShockwaveRouter{
		NodeID: nodeID,
		Rules: PropagationRules{
			MaxDepth:   5,
			MaxWorkers: 100,
		},
		Splitters: Splitters{
			ChunkSize: 1024,
			MaxChunks: 100,
		},
		Duplicators: Duplicators{
			RedundancyFactor: 2,
			Enabled:          true,
		},
		ReduceStrategy: ReduceStrategy{
			RequireAll: false,
			Timeout:    2 * time.Second,
		},
	}
}
