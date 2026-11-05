package steer

import (
	"time"

	"github.com/zqk-os/zqk/pkg/vector"
)

// Event topic for steering
const (
	EventSteeringApplied = "steering.applied"
)

// SteeringIntent represents the parsed NLP intention.
type SteeringIntent struct {
	RawText           string             `json:"raw_text"`
	Dimensions        []vector.Dimension `json:"dimensions"`
	TargetVectorIDs   []string           `json:"target_vector_ids"`
	Intensity         float64            `json:"intensity"`
	Direction         string             `json:"direction"` // "boost" | "reduce"
	ExtractedKeywords []string           `json:"extracted_keywords"`
}

// SteeringAdjustment describes changes made to a specific vector.
type SteeringAdjustment struct {
	VectorID    string           `json:"vector_id"`
	Dimension   vector.Dimension `json:"dimension"`
	OldWeight   float64          `json:"old_weight"`
	NewWeight   float64          `json:"new_weight"`
	OldRank     int              `json:"old_rank"`
	NewRank     int              `json:"new_rank"`
	WeightDelta float64          `json:"weight_delta"`
}

// SteeringOutcome represents the result of metabolizing a steering directive.
type SteeringOutcome struct {
	Intent           SteeringIntent       `json:"intent"`
	Adjustments      []SteeringAdjustment `json:"adjustments"`
	SchedulerWeights map[string]float64   `json:"scheduler_weights"`
	AppliedAt        time.Time            `json:"applied_at"`
	IsReset          bool                 `json:"is_reset"`
}

// SteeringState represents the persisted steering state on disk.
type SteeringState struct {
	ActiveOffsets    map[string]float64   `json:"active_offsets"`
	LastIntent       string               `json:"last_intent,omitempty"`
	LastAppliedAt    time.Time            `json:"last_applied_at"`
	SchedulerWeights map[string]float64   `json:"scheduler_weights"`
	Adjustments      []SteeringAdjustment `json:"adjustments,omitempty"`
}
