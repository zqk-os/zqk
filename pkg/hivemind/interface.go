package hivemind

import (
	"context"
)

// MemoryResult represents a unified graph-vector lookup outcome.
type MemoryResult struct {
	ID       string
	Score    float32
	Metadata map[string]any
}

// HybridConstraints defines filters for semantic-structural hybrid queries.
type HybridConstraints struct {
	MaxDepth int
	Kind     string
}

// GraphSubgraph represents the result of a structural graph traversal.
type GraphSubgraph struct {
	Nodes map[string]any
	Edges []map[string]any
}

// MemoryStore defines the interface for actor-based semantic and structural memory retrieval.
type MemoryStore interface {
	// RetrieveSemantically performs K-Nearest Neighbors search.
	RetrieveSemantically(ctx context.Context, query string, k int) ([]MemoryResult, error)

	// RetrieveGraphContext fetches structural neighbors for a given ID.
	RetrieveGraphContext(ctx context.Context, id string, depth int) (*GraphSubgraph, error)

	// QueryHybrid performs a combined semantic-structural query.
	QueryHybrid(ctx context.Context, query string, limit int, constraints HybridConstraints) ([]MemoryResult, error)

	// FindObjectsMissingVectors finds objects that lack vector embeddings.
	FindObjectsMissingVectors(ctx context.Context, batchSize int) ([]string, error)

	// LinkVectorID links a generated vector ID back to the graph object.
	LinkVectorID(ctx context.Context, objectID string, vectorID string) error
}
