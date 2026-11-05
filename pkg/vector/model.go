package vector

import (
	"fmt"
	"strings"
	"time"
)

// Dimension represents the evaluation and steering dimension category.
type Dimension string

const (
	DimensionSecurity    Dimension = "security"
	DimensionGovernance  Dimension = "governance"
	DimensionReliability Dimension = "reliability"
	DimensionCapability  Dimension = "capability"
	DimensionReadability Dimension = "readability"
	DimensionEfficiency  Dimension = "efficiency"
	DimensionPortability Dimension = "portability"
	DimensionCustom      Dimension = "custom"
)

// ValidDimension checks if the given dimension is supported.
func ValidDimension(d Dimension) bool {
	switch d {
	case DimensionSecurity, DimensionGovernance, DimensionReliability,
		DimensionCapability, DimensionReadability, DimensionEfficiency,
		DimensionPortability, DimensionCustom:
		return true
	default:
		return false
	}
}

// Vector represents a first-class evaluation and steering vector in the Knowledge Kernel.
type Vector struct {
	ID            string         `json:"id" yaml:"id"`                       // e.g. VEC-SECURITY
	Dimension     Dimension      `json:"dimension" yaml:"dimension"`         // security | governance | reliability | capability | readability | efficiency | portability | custom
	PriorityRank  int            `json:"priority_rank" yaml:"priority_rank"` // 1 to 100 (100 = highest priority)
	Weight        float64        `json:"weight" yaml:"weight"`               // 0.0 to 1.0
	Threshold     float64        `json:"threshold" yaml:"threshold"`         // e.g. 0.85
	CurrentScore  float64        `json:"current_score" yaml:"current_score"` // 0.0 to 1.0
	Status        string         `json:"status" yaml:"status"`               // passing | breached | pending
	Description   string         `json:"description" yaml:"description"`
	LastEvaluated time.Time      `json:"last_evaluated" yaml:"last_evaluated"`
	Metadata      map[string]any `json:"metadata,omitempty" yaml:"metadata,omitempty"`
}

// Validate checks that the vector satisfies invariant constraints.
func (v *Vector) Validate() error {
	if !strings.HasPrefix(v.ID, "VEC-") {
		return fmt.Errorf("vector ID %q must start with 'VEC-' prefix", v.ID)
	}
	if !ValidDimension(v.Dimension) {
		return fmt.Errorf("invalid dimension %q for vector %s", v.Dimension, v.ID)
	}
	if v.PriorityRank < 1 || v.PriorityRank > 100 {
		return fmt.Errorf("priority_rank %d must be between 1 and 100 for vector %s", v.PriorityRank, v.ID)
	}
	if v.Weight < 0.0 || v.Weight > 1.0 {
		return fmt.Errorf("weight %.2f must be between 0.0 and 1.0 for vector %s", v.Weight, v.ID)
	}
	if v.Threshold < 0.0 || v.Threshold > 1.0 {
		return fmt.Errorf("threshold %.2f must be between 0.0 and 1.0 for vector %s", v.Threshold, v.ID)
	}
	return nil
}

// EvaluationResult represents the evaluation outcome for a vector.
type EvaluationResult struct {
	VectorID    string         `json:"vector_id"`
	Dimension   Dimension      `json:"dimension"`
	Score       float64        `json:"score"`
	Threshold   float64        `json:"threshold"`
	Breached    bool           `json:"breached"`
	Status      string         `json:"status"`
	Details     map[string]any `json:"details,omitempty"`
	EvaluatedAt time.Time      `json:"evaluated_at"`
}

// VectorSteerDelta defines an adjustment to a vector's properties.
type VectorSteerDelta struct {
	VectorID     string   `json:"vector_id"`
	WeightDelta  *float64 `json:"weight_delta,omitempty"`
	RankDelta    *int     `json:"rank_delta,omitempty"`
	NewWeight    *float64 `json:"new_weight,omitempty"`
	NewRank      *int     `json:"new_rank,omitempty"`
	NewThreshold *float64 `json:"new_threshold,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// Event topic names
const (
	EventVectorEvaluated       = "vector.evaluated"
	EventVectorThresholdBreach = "vector.threshold_breached"
	EventVectorSteered         = "vector.steered"
)
