package eval

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// CanaryRecoveryStatus provides telemetry on the circuit breaker's recovery progression.
type CanaryRecoveryStatus struct {
	State               QualityCircuitBreakerState `json:"state" yaml:"state"`
	CanaryPassCount     int                        `json:"canary_pass_count" yaml:"canary_pass_count"`
	CanaryThreshold     int                        `json:"canary_threshold" yaml:"canary_threshold"`
	EffectiveCooldown   time.Duration              `json:"effective_cooldown" yaml:"effective_cooldown"`
	RemainingCooldown   time.Duration              `json:"remaining_cooldown" yaml:"remaining_cooldown"`
	TripCount           int                        `json:"trip_count" yaml:"trip_count"`
	RecentTripsInWindow int                        `json:"recent_trips_in_window" yaml:"recent_trips_in_window"`
	IsFlappingDampened  bool                       `json:"is_flapping_dampened" yaml:"is_flapping_dampened"`
	BaselineEstablished bool                       `json:"baseline_established" yaml:"baseline_established"`
	ActiveBaselineScore float64                    `json:"active_baseline_score" yaml:"active_baseline_score"`
}

// CanaryStatus returns comprehensive telemetry on the circuit breaker's state and recovery pipeline.
func (cb *QualityCircuitBreaker) CanaryStatus(customNow ...time.Time) CanaryRecoveryStatus {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	if len(customNow) > 0 && !customNow[0].IsZero() {
		now = customNow[0]
	}

	state := cb.currentStateLocked(now)

	var remaining time.Duration
	if state == CircuitOpen && !cb.lastTripTime.IsZero() {
		elapsed := now.Sub(cb.lastTripTime)
		if elapsed < cb.effectiveCool {
			remaining = cb.effectiveCool - elapsed
		}
	}

	windowStart := now.Add(-cb.config.FlappingWindow)
	recentTrips := 0
	for _, t := range cb.tripHistory {
		if !t.Before(windowStart) {
			recentTrips++
		}
	}

	baseEstablished := cb.baseline != nil && cb.baseline.CompositeScore > 0
	var baseScore float64
	if baseEstablished {
		baseScore = cb.baseline.CompositeScore
	}

	return CanaryRecoveryStatus{
		State:               state,
		CanaryPassCount:     cb.canaryPassCount,
		CanaryThreshold:     cb.config.CanaryConsecutivePassThreshold,
		EffectiveCooldown:   cb.effectiveCool,
		RemainingCooldown:   remaining,
		TripCount:           cb.tripCount,
		RecentTripsInWindow: recentTrips,
		IsFlappingDampened:  cb.effectiveCool > cb.config.CooldownDuration,
		BaselineEstablished: baseEstablished,
		ActiveBaselineScore: baseScore,
	}
}

// CanaryProbeFilter intercepts dispatches during canary recovery phases.
type CanaryProbeFilter struct {
	breaker *QualityCircuitBreaker
}

// NewCanaryProbeFilter constructs a canary probe filter.
func NewCanaryProbeFilter(breaker *QualityCircuitBreaker) (*CanaryProbeFilter, error) {
	if breaker == nil {
		return nil, errfmt.Errorf("quality circuit breaker cannot be nil")
	}
	return &CanaryProbeFilter{breaker: breaker}, nil
}

// CheckDispatch verifies whether the intended dispatch is allowed.
// Regular dispatches are rejected when the breaker is Open or HalfOpen; only canary probes are allowed in HalfOpen.
func (f *CanaryProbeFilter) CheckDispatch(ctx context.Context, isCanary bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	allowed, err := f.breaker.AllowDispatch(isCanary)
	if !allowed {
		return err
	}
	return nil
}
