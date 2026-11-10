package eval

import (
	"context"
	"fmt"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// RegressionEvaluationResult encapsulates the outcome of a benchmark scorecard evaluation against baseline.
type RegressionEvaluationResult struct {
	Passed        bool                       `json:"passed" yaml:"passed"`
	BaselineScore float64                    `json:"baseline_score" yaml:"baseline_score"`
	CurrentScore  float64                    `json:"current_score" yaml:"current_score"`
	DeltaRatio    float64                    `json:"delta_ratio" yaml:"delta_ratio"`
	State         QualityCircuitBreakerState `json:"state" yaml:"state"`
	Message       string                     `json:"message" yaml:"message"`
	Tripped       bool                       `json:"tripped" yaml:"tripped"`
	Recovered     bool                       `json:"recovered" yaml:"recovered"`
	EvaluatedAt   time.Time                  `json:"evaluated_at" yaml:"evaluated_at"`
}

// RecordEvaluation evaluates a candidate scorecard against baseline and updates the circuit breaker state.
func (cb *QualityCircuitBreaker) RecordEvaluation(current *RegressionScorecard, customNow ...time.Time) (*RegressionEvaluationResult, error) {
	if current == nil {
		return nil, errfmt.Errorf("candidate benchmark scorecard cannot be nil")
	}
	if current.CompositeScore < 0 {
		return nil, ErrInvalidRegressionScore
	}

	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	if len(customNow) > 0 && !customNow[0].IsZero() {
		now = customNow[0]
	}

	// Update any open->half-open transition based on elapsed cooldown
	cb.currentStateLocked(now)

	result := &RegressionEvaluationResult{
		CurrentScore: current.CompositeScore,
		EvaluatedAt:  now,
	}

	// Baseline handling: if zero or uninitialized baseline, safely initialize baseline with current
	if cb.baseline == nil || cb.baseline.CompositeScore <= 0 {
		cp := *current
		cb.baseline = &cp
		result.BaselineScore = current.CompositeScore
		result.Passed = true
		result.State = cb.state
		result.Message = fmt.Sprintf("initial benchmark baseline established at composite score %.4f", current.CompositeScore)
		return result, nil
	}

	baseScore := cb.baseline.CompositeScore
	result.BaselineScore = baseScore

	// Calculate fractional regression: delta = (baseline - current) / baseline
	delta := (baseScore - current.CompositeScore) / baseScore
	result.DeltaRatio = delta

	isRegression := delta > cb.config.MaxAllowedRegressionDelta

	if isRegression {
		cb.tripCount++
		cb.canaryPassCount = 0
		cb.lastTripTime = now
		cb.state = CircuitOpen
		result.Passed = false
		result.Tripped = true
		result.State = CircuitOpen

		// Apply flapping dampener
		cb.pruneAndRecordTripLocked(now)

		result.Message = fmt.Sprintf(
			"score regression detected: delta %.2f%% exceeds max allowed %.2f%% (baseline: %.4f, current: %.4f) - circuit tripped to Open",
			delta*100, cb.config.MaxAllowedRegressionDelta*100, baseScore, current.CompositeScore,
		)
		return result, nil
	}

	// Score within acceptable delta
	result.Passed = true

	if cb.state == CircuitHalfOpen {
		cb.canaryPassCount++
		if cb.canaryPassCount >= cb.config.CanaryConsecutivePassThreshold {
			cb.state = CircuitClosed
			cb.canaryPassCount = 0
			cb.effectiveCool = cb.config.CooldownDuration
			result.Recovered = true
			result.State = CircuitClosed
			result.Message = fmt.Sprintf(
				"canary recovery threshold reached (%d consecutive passes) - circuit returned to Closed",
				cb.config.CanaryConsecutivePassThreshold,
			)
			return result, nil
		}
		result.State = CircuitHalfOpen
		result.Message = fmt.Sprintf(
			"canary pass %d of %d recorded - circuit remains HalfOpen",
			cb.canaryPassCount, cb.config.CanaryConsecutivePassThreshold,
		)
		return result, nil
	}

	result.State = cb.state
	result.Message = fmt.Sprintf("evaluation passed with delta %.2f%% within acceptable threshold", delta*100)

	// Upward ratchet: if model strictly improved, update baseline
	if current.CompositeScore > baseScore {
		cp := *current
		cb.baseline = &cp
	}

	return result, nil
}

// pruneAndRecordTripLocked appends the trip and applies exponential backoff if flapping is detected.
func (cb *QualityCircuitBreaker) pruneAndRecordTripLocked(now time.Time) {
	cb.tripHistory = append(cb.tripHistory, now)

	// Prune events outside the flapping window
	windowStart := now.Add(-cb.config.FlappingWindow)
	validIdx := 0
	for i, t := range cb.tripHistory {
		if !t.Before(windowStart) {
			validIdx = i
			break
		}
	}
	if validIdx > 0 {
		cb.tripHistory = cb.tripHistory[validIdx:]
	}

	// If trips within window meet or exceed FlappingThreshold, double cooldown (exponential backoff capped at 24h)
	if len(cb.tripHistory) >= cb.config.FlappingThreshold {
		newCool := cb.effectiveCool * 2
		if newCool > 24*time.Hour {
			newCool = 24 * time.Hour
		}
		cb.effectiveCool = newCool
	}
}

// EffectiveCooldown returns the current cooldown duration (adjusted by flapping dampener).
func (cb *QualityCircuitBreaker) EffectiveCooldown() time.Duration {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.effectiveCool
}

// Reset clears circuit breaker state back to Closed and resets metrics.
func (cb *QualityCircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = CircuitClosed
	cb.canaryPassCount = 0
	cb.tripCount = 0
	cb.tripHistory = cb.tripHistory[:0]
	cb.effectiveCool = cb.config.CooldownDuration
	cb.lastTripTime = time.Time{}
}

// BenchmarkRegressionGate coordinates benchmark evaluations with the underlying quality circuit breaker.
type BenchmarkRegressionGate struct {
	breaker *QualityCircuitBreaker
}

// NewBenchmarkRegressionGate constructs a new regression gate wrapping the circuit breaker.
func NewBenchmarkRegressionGate(breaker *QualityCircuitBreaker) (*BenchmarkRegressionGate, error) {
	if breaker == nil {
		return nil, errfmt.Errorf("quality circuit breaker cannot be nil")
	}
	return &BenchmarkRegressionGate{breaker: breaker}, nil
}

// Breaker returns the underlying circuit breaker.
func (g *BenchmarkRegressionGate) Breaker() *QualityCircuitBreaker {
	return g.breaker
}

// Evaluate performs regression checking for a given benchmark scorecard under context deadline.
func (g *BenchmarkRegressionGate) Evaluate(ctx context.Context, current *RegressionScorecard) (*RegressionEvaluationResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return g.breaker.RecordEvaluation(current)
}
