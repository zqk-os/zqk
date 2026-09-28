package diagnostics

import (
	"fmt"
	"time"
)

// Stagnation evaluation constants.
const (
	DefaultMaxStagnantAttempts = 2
	DefaultMaxIdenticalSigs    = 2

	ReasonZeroEntropyLoop   = "loop stagnation detected: consecutive retry attempts introduced zero diagnostic delta"
	ReasonSignatureRepeated = "loop stagnation detected: identical failure signature repeated %d times exceeding threshold %d"
	ReasonActiveProgress    = "active progress: retry attempts introduced diagnostic entropy"

	RecommendationContinue = "continue"
	RecommendationPivot    = "pivot_strategy"
	RecommendationEscalate = "escalate_to_supervisor"
	RecommendationHalt     = "halt_loop"
)

// StagnationPolicy defines configurable thresholds for detecting stagnant loops.
type StagnationPolicy struct {
	MaxStagnantAttempts    int  `json:"max_stagnant_attempts"`
	MaxIdenticalSignatures int  `json:"max_identical_signatures"`
	ZeroDeltaFailFast      bool `json:"zero_delta_fail_fast"`
}

// DefaultStagnationPolicy returns default fail-fast protection against blind retries.
func DefaultStagnationPolicy() StagnationPolicy {
	return StagnationPolicy{
		MaxStagnantAttempts:    DefaultMaxStagnantAttempts,
		MaxIdenticalSignatures: DefaultMaxIdenticalSigs,
		ZeroDeltaFailFast:      true,
	}
}

// StagnationAssessment represents the outcome of evaluating retry history.
type StagnationAssessment struct {
	IsStagnant             bool           `json:"is_stagnant"`
	ConsecutiveZeroDelta   int            `json:"consecutive_zero_delta"`
	IdenticalSigOccurrences int            `json:"identical_sig_occurrences"`
	Escalate               bool           `json:"escalate"`
	Recommendation         string         `json:"recommendation"`
	Reason                 string         `json:"reason"`
}

// StateCheckpoint captures the state signature and entropy delta at an attempt boundary.
type StateCheckpoint struct {
	Attempt      int       `json:"attempt"`
	Signature    string    `json:"signature"`
	Timestamp    time.Time `json:"timestamp"`
	EntropyDelta bool      `json:"entropy_delta"`
}

// BuildCheckpoints converts failure history into state checkpoints.
func BuildCheckpoints(history []FailureEnvelope) []StateCheckpoint {
	if len(history) == 0 {
		return nil
	}
	checkpoints := make([]StateCheckpoint, len(history))
	for i, env := range history {
		entropy := true
		if i > 0 && history[i-1].Signature == env.Signature {
			entropy = false
		}
		checkpoints[i] = StateCheckpoint{
			Attempt:      env.Attempt,
			Signature:    env.Signature,
			Timestamp:    env.Timestamp,
			EntropyDelta: entropy,
		}
	}
	return checkpoints
}

// EvaluateStagnation evaluates failure history against a StagnationPolicy
// to prevent fruitless, repetitive retries when no new information is introduced.
func EvaluateStagnation(history []FailureEnvelope, policy StagnationPolicy) StagnationAssessment {
	if policy.MaxStagnantAttempts <= 0 {
		policy.MaxStagnantAttempts = DefaultMaxStagnantAttempts
	}
	if policy.MaxIdenticalSignatures <= 0 {
		policy.MaxIdenticalSignatures = DefaultMaxIdenticalSigs
	}

	if len(history) <= 1 {
		return StagnationAssessment{
			IsStagnant:     false,
			Escalate:       false,
			Recommendation: RecommendationContinue,
			Reason:         ReasonActiveProgress,
		}
	}

	// Track consecutive zero delta from the most recent attempts backward
	consecutiveZeroDelta := 0
	for i := len(history) - 1; i > 0; i-- {
		if history[i].Signature == history[i-1].Signature {
			consecutiveZeroDelta++
		} else {
			break
		}
	}

	// Count occurrences of the latest signature across entire history
	latestSig := history[len(history)-1].Signature
	sigCount := 0
	if latestSig != "" {
		for _, env := range history {
			if env.Signature == latestSig {
				sigCount++
			}
		}
	}

	// Check condition 1: Consecutive zero-delta attempts
	if consecutiveZeroDelta >= policy.MaxStagnantAttempts || (policy.ZeroDeltaFailFast && consecutiveZeroDelta >= 1) {
		rec := RecommendationPivot
		if consecutiveZeroDelta >= policy.MaxStagnantAttempts {
			rec = RecommendationEscalate
		}
		return StagnationAssessment{
			IsStagnant:             true,
			ConsecutiveZeroDelta:   consecutiveZeroDelta,
			IdenticalSigOccurrences: sigCount,
			Escalate:               rec == RecommendationEscalate,
			Recommendation:         rec,
			Reason:                 ReasonZeroEntropyLoop,
		}
	}

	// Check condition 2: Overall identical signature frequency
	if sigCount >= policy.MaxIdenticalSignatures {
		return StagnationAssessment{
			IsStagnant:             true,
			ConsecutiveZeroDelta:   consecutiveZeroDelta,
			IdenticalSigOccurrences: sigCount,
			Escalate:               true,
			Recommendation:         RecommendationEscalate,
			Reason:                 fmt.Sprintf(ReasonSignatureRepeated, sigCount, policy.MaxIdenticalSignatures),
		}
	}

	return StagnationAssessment{
		IsStagnant:             false,
		ConsecutiveZeroDelta:   consecutiveZeroDelta,
		IdenticalSigOccurrences: sigCount,
		Escalate:               false,
		Recommendation:         RecommendationContinue,
		Reason:                 ReasonActiveProgress,
	}
}

// StagnationError creates a structured error when loop stagnation is detected.
func StagnationError(assessment StagnationAssessment) error {
	if !assessment.IsStagnant {
		return nil
	}
	return fmt.Errorf("loop stagnation: %s (recommendation: %s)", assessment.Reason, assessment.Recommendation)
}
