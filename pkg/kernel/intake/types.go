package intake

import (
	"errors"
)

// ExecutionMode defines the dispatch topology for clustered intake items.
type ExecutionMode string

const (
	// ModeSequential requires items to execute in strict linear succession.
	ModeSequential ExecutionMode = "sequential"
	// ModeConcurrent allows independent items to execute concurrently in parallel workers.
	ModeConcurrent ExecutionMode = "concurrent"
	// ModeHybridDAG specifies a dependency-directed acyclic graph with fork/join barriers.
	ModeHybridDAG ExecutionMode = "hybrid_dag"
)

// Predefined error conditions for the intake membrane.
var (
	ErrEmptyIntakeRequests      = errors.New("intake membrane: requests slice cannot be empty")
	ErrRedundantOneToOneChains  = errors.New("intake membrane: rejected redundant 1:1 micro-chain proposals; cluster into unified workstream")
	ErrConflictingFileMutations = errors.New("intake membrane: unresolvable write conflict in concurrent batch")
	ErrMissingOperationalScope = errors.New("intake membrane: request missing operational scope or target paths")
)

// SimilarityThreshold defines the minimum Jaccard similarity score (0.0 to 1.0)
// to cluster separate requests into a single unified workstream.
const SimilarityThreshold = 0.40

// IntakeRequest represents an incoming work request entering the kernel intake membrane.
type IntakeRequest struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Description      string            `json:"description"`
	DomainCategory   string            `json:"domain_category"`
	TargetPaths      []string          `json:"target_paths"`
	RequestedTiers   []string          `json:"requested_tiers,omitempty"`
	Metadata         map[string]string `json:"metadata,omitempty"`
}

// ClusteredWorkstream groups related intake requests into a cohesive execution container.
type ClusteredWorkstream struct {
	ClusterID        string           `json:"cluster_id"`
	Title            string           `json:"title"`
	DomainCategory   string           `json:"domain_category"`
	Requests         []IntakeRequest  `json:"requests"`
	CommonPaths      []string         `json:"common_paths"`
	AverageSimilarity float64         `json:"average_similarity"`
}

// StageDependency represents a dependency edge in a hybrid DAG topology.
type StageDependency struct {
	RequestID string   `json:"request_id"`
	DependsOn []string `json:"depends_on"`
}

// ExecutionTopology details how a cluster of items must be sequenced.
type ExecutionTopology struct {
	Mode         ExecutionMode     `json:"mode"`
	Batches      [][]string        `json:"batches"` // Array of concurrent stages executed sequentially
	Dependencies []StageDependency `json:"dependencies"`
	TotalItems   int               `json:"total_items"`
}

// IntakeSynthesisResult represents the complete structured output from the intake membrane.
type IntakeSynthesisResult struct {
	Clusters          []*ClusteredWorkstream `json:"clusters"`
	Topologies        []*ExecutionTopology   `json:"topologies"`
	DeduplicatedCount int                    `json:"deduplicated_count"`
	TotalInputCount   int                    `json:"total_input_count"`
}
