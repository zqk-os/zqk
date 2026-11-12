package community

import (
	"context"
	"errors"
	"math"
	"sort"
	"sync"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

var (
	ErrEmptyVector         = errors.New("vector embedding cannot be empty")
	ErrDimensionMismatch   = errors.New("vector dimension mismatch")
	ErrCapabilityDuplicate = errors.New("capability already registered in index")
)

// CapabilityEntry represents an indexed provider capability with its semantic vector embedding.
type CapabilityEntry struct {
	ID          string    `json:"id"`
	ProviderID  string    `json:"provider_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Tags        []string  `json:"tags,omitempty"`
	Vector      []float64 `json:"vector"`
}

// MatchResult represents a matched capability scored by cosine similarity.
type MatchResult struct {
	Capability CapabilityEntry `json:"capability"`
	Similarity float64         `json:"similarity"`
}

// SemanticCapabilityIndex provides in-memory vector indexing and nearest-neighbor search for capabilities.
type SemanticCapabilityIndex struct {
	mu           sync.RWMutex
	dimensions   int
	capabilities map[string]CapabilityEntry
}

// NewSemanticCapabilityIndex initializes an index with expected embedding dimensions.
func NewSemanticCapabilityIndex(dimensions int) *SemanticCapabilityIndex {
	return &SemanticCapabilityIndex{
		dimensions:   dimensions,
		capabilities: make(map[string]CapabilityEntry),
	}
}

// Register indexes a new provider capability.
func (idx *SemanticCapabilityIndex) Register(entry CapabilityEntry) error {
	if len(entry.Vector) == 0 {
		return ErrEmptyVector
	}
	if idx.dimensions > 0 && len(entry.Vector) != idx.dimensions {
		return ErrDimensionMismatch
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()

	if _, exists := idx.capabilities[entry.ID]; exists {
		return ErrCapabilityDuplicate
	}

	if idx.dimensions == 0 {
		idx.dimensions = len(entry.Vector)
	}

	idx.capabilities[entry.ID] = entry
	return nil
}

// FindNearest searches for the top-K most similar capabilities exceeding the similarity threshold.
func (idx *SemanticCapabilityIndex) FindNearest(query []float64, topK int, minSim float64) []MatchResult {
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	if len(query) == 0 || len(query) != idx.dimensions || topK <= 0 {
		return nil
	}

	results := make([]MatchResult, 0, len(idx.capabilities))
	for _, entry := range idx.capabilities {
		sim := CosineSimilarity(query, entry.Vector)
		if sim >= minSim {
			results = append(results, MatchResult{
				Capability: entry,
				Similarity: sim,
			})
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	if len(results) > topK {
		results = results[:topK]
	}
	return results
}

// Count returns the number of indexed capabilities.
func (idx *SemanticCapabilityIndex) Count() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return len(idx.capabilities)
}

// CosineSimilarity computes the normalized dot product between two vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0.0
	}

	var dot, magA, magB float64
	for i := 0; i < len(a); i++ {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}

	denom := math.Sqrt(magA) * math.Sqrt(magB)
	if denom == 0.0 {
		return 0.0
	}
	return dot / denom
}

// SafeRunGoroutine executes an asynchronous worker under POL-CONCURRENCY guidelines.
func (idx *SemanticCapabilityIndex) SafeRunGoroutine(ctx context.Context, label string, fn func()) {
	goroutinelabels.NewGoroutine("community.semantic."+label, "async semantic indexer worker").StartSimple(fn)
}
