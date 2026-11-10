package eval_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/eval"
)

// CRIT-1794276043050465000-56b64757
// Static Floor Invariant: Regression thresholds and circuit-breaker states (Closed, HalfOpen, Open)
// configured with deterministic default delta thresholds.
func TestBenchmarkRegressionGate_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	// 1. Production defaults verification
	def := eval.DefaultQualityCircuitBreakerConfig()
	assert.Equal(t, 0.05, def.MaxAllowedRegressionDelta, "default regression delta floor must be 5%")
	assert.Equal(t, 5*time.Minute, def.CooldownDuration, "default cooldown duration must be 5 minutes")
	assert.Equal(t, 3, def.CanaryConsecutivePassThreshold, "default canary pass threshold must be 3")
	assert.Equal(t, 3, def.FlappingThreshold, "default flapping threshold must be 3")
	assert.Equal(t, 15*time.Minute, def.FlappingWindow, "default flapping window must be 15 minutes")

	require.NoError(t, def.Validate(), "default configuration must validate successfully")

	// 2. State Enum Invariants
	assert.Equal(t, eval.QualityCircuitBreakerState("Closed"), eval.CircuitClosed)
	assert.Equal(t, eval.QualityCircuitBreakerState("HalfOpen"), eval.CircuitHalfOpen)
	assert.Equal(t, eval.QualityCircuitBreakerState("Open"), eval.CircuitOpen)

	// 3. Configuration validation bounds (Negative static boundary checks)
	invalidConfigs := []struct {
		name string
		cfg  eval.QualityCircuitBreakerConfig
	}{
		{
			name: "negative max delta",
			cfg: func() eval.QualityCircuitBreakerConfig {
				c := def
				c.MaxAllowedRegressionDelta = -0.01
				return c
			}(),
		},
		{
			name: "max delta exceeding 1.0",
			cfg: func() eval.QualityCircuitBreakerConfig {
				c := def
				c.MaxAllowedRegressionDelta = 1.05
				return c
			}(),
		},
		{
			name: "zero cooldown duration",
			cfg: func() eval.QualityCircuitBreakerConfig {
				c := def
				c.CooldownDuration = 0
				return c
			}(),
		},
		{
			name: "zero canary threshold",
			cfg: func() eval.QualityCircuitBreakerConfig {
				c := def
				c.CanaryConsecutivePassThreshold = 0
				return c
			}(),
		},
		{
			name: "zero flapping threshold",
			cfg: func() eval.QualityCircuitBreakerConfig {
				c := def
				c.FlappingThreshold = 0
				return c
			}(),
		},
		{
			name: "zero flapping window",
			cfg: func() eval.QualityCircuitBreakerConfig {
				c := def
				c.FlappingWindow = 0
				return c
			}(),
		},
	}

	for _, tc := range invalidConfigs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			assert.Error(t, err, "invalid config %s must fail validation", tc.name)
		})
	}

	// 4. Initial state invariants on newly constructed breaker
	cb, err := eval.NewQualityCircuitBreaker(def)
	require.NoError(t, err)
	assert.Equal(t, eval.CircuitClosed, cb.State(), "initial state must be CircuitClosed")
	assert.Equal(t, 0, cb.TripCount(), "initial trip count must be 0")
	assert.Nil(t, cb.Baseline(), "initial baseline must be nil before evaluations")

	allowed, err := cb.AllowDispatch(false)
	assert.True(t, allowed, "dispatches must be allowed in initial Closed state")
	assert.NoError(t, err)
}

// CRIT-1794276046925722000-9483cec8
// Operational Dynamic Proof: Circuit breaker trips to Open when benchmark composite score
// drops below regression threshold against baseline, halting non-conforming model dispatch.
func TestBenchmarkRegressionGate_OperationalDynamicProof(t *testing.T) {
	t.Parallel()

	cfg := eval.QualityCircuitBreakerConfig{
		MaxAllowedRegressionDelta:      0.05, // 5% max allowed drop
		CooldownDuration:               1 * time.Minute,
		CanaryConsecutivePassThreshold: 2,
		FlappingThreshold:              5,
		FlappingWindow:                 10 * time.Minute,
	}

	cb, err := eval.NewQualityCircuitBreaker(cfg)
	require.NoError(t, err)

	gate, err := eval.NewBenchmarkRegressionGate(cb)
	require.NoError(t, err)

	now := time.Now()

	// 1. Establish baseline run at composite score 100.0
	baselineRun := &eval.RegressionScorecard{
		RunID:          "run-base-001",
		ModelTier:      "pro",
		CompositeScore: 100.0,
		LatencyP95Ms:   150.0,
		Accuracy:       0.98,
		EvaluatedAt:    now,
	}

	resInit, err := gate.Breaker().RecordEvaluation(baselineRun, now)
	require.NoError(t, err)
	assert.True(t, resInit.Passed)
	assert.Equal(t, eval.CircuitClosed, cb.State())
	assert.Equal(t, 100.0, cb.Baseline().CompositeScore)

	// 2. Candidate with acceptable score (96.0 -> 4.0% drop, under 5.0% threshold)
	candidateGood := &eval.RegressionScorecard{
		RunID:          "run-cand-good",
		ModelTier:      "pro",
		CompositeScore: 96.0,
		EvaluatedAt:    now.Add(1 * time.Minute),
	}

	resGood, err := gate.Breaker().RecordEvaluation(candidateGood, now.Add(1*time.Minute))
	require.NoError(t, err)
	assert.True(t, resGood.Passed, "4% drop must be accepted within 5% threshold")
	assert.False(t, resGood.Tripped)
	assert.Equal(t, eval.CircuitClosed, cb.State())

	allowed, err := cb.AllowDispatch(false)
	assert.True(t, allowed)
	assert.NoError(t, err)

	// 3. Regressed candidate (92.0 -> 8.0% drop, exceeding 5.0% threshold)
	candidateRegressed := &eval.RegressionScorecard{
		RunID:          "run-cand-regressed",
		ModelTier:      "pro",
		CompositeScore: 92.0,
		EvaluatedAt:    now.Add(2 * time.Minute),
	}

	resBad, err := gate.Evaluate(context.Background(), candidateRegressed)
	require.NoError(t, err)
	assert.False(t, resBad.Passed, "8% drop must fail regression evaluation")
	assert.True(t, resBad.Tripped, "regression must trip circuit breaker")
	assert.Equal(t, eval.CircuitOpen, resBad.State)
	assert.Equal(t, eval.CircuitOpen, cb.State(), "state machine must flip to CircuitOpen")
	assert.Equal(t, 1, cb.TripCount())

	// 4. Verify model dispatch is strictly halted in Open state
	allowedOpen, errOpen := cb.AllowDispatch(false)
	assert.False(t, allowedOpen, "model dispatch must be halted when circuit is Open")
	assert.ErrorIs(t, errOpen, eval.ErrCircuitBreakerOpen)

	// Even canary dispatch is halted during active cooldown
	allowedCanary, errCanary := cb.AllowDispatch(true)
	assert.False(t, allowedCanary, "canary dispatch must also be halted during active Open cooldown")
	assert.ErrorIs(t, errCanary, eval.ErrCircuitBreakerOpen)
}

// CRIT-1794276050300376000-6b91b63d
// Negative Adversarial Boundary: Flapping regression dampening, zero/invalid baseline fallback,
// and non-panicking auto-recovery to HalfOpen upon subsequent passing canary runs.
func TestBenchmarkRegressionGate_NegativeAdversarialBoundary(t *testing.T) {
	t.Parallel()

	cfg := eval.QualityCircuitBreakerConfig{
		MaxAllowedRegressionDelta:      0.05,
		CooldownDuration:               100 * time.Millisecond,
		CanaryConsecutivePassThreshold: 2,
		FlappingThreshold:              3, // 3 trips within window doubles cooldown
		FlappingWindow:                 10 * time.Second,
	}

	cb, err := eval.NewQualityCircuitBreaker(cfg)
	require.NoError(t, err)

	filter, err := eval.NewCanaryProbeFilter(cb)
	require.NoError(t, err)

	now := time.Now()

	// 1. Adversarial input validation: nil scorecard & negative scores
	_, errNil := cb.RecordEvaluation(nil)
	assert.Error(t, errNil, "nil scorecard must return error without panicking")

	negCard := &eval.RegressionScorecard{CompositeScore: -10.0}
	_, errNeg := cb.RecordEvaluation(negCard)
	assert.ErrorIs(t, errNeg, eval.ErrInvalidRegressionScore)

	// 2. Zero baseline fallback: initializing with zero/empty scorecard establishing initial baseline
	zeroCard := &eval.RegressionScorecard{CompositeScore: 0.0}
	resZero, err := cb.RecordEvaluation(zeroCard, now)
	require.NoError(t, err)
	assert.True(t, resZero.Passed)

	// Re-establish real baseline at 100.0
	cb.Reset()
	baseCard := &eval.RegressionScorecard{CompositeScore: 100.0}
	_, err = cb.RecordEvaluation(baseCard, now)
	require.NoError(t, err)

	// 3. Flapping Regression Dampening: Trigger repeated rapid trips to test exponential backoff
	assert.Equal(t, 100*time.Millisecond, cb.EffectiveCooldown(), "initial cooldown must match config")

	// Trip 1
	badCard := &eval.RegressionScorecard{CompositeScore: 80.0}
	_, err = cb.RecordEvaluation(badCard, now.Add(1*time.Second))
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, cb.EffectiveCooldown())

	// Trip 2
	_, err = cb.RecordEvaluation(badCard, now.Add(2*time.Second))
	require.NoError(t, err)
	assert.Equal(t, 100*time.Millisecond, cb.EffectiveCooldown())

	// Trip 3: Reaches FlappingThreshold -> effective cooldown must double
	_, err = cb.RecordEvaluation(badCard, now.Add(3*time.Second))
	require.NoError(t, err)
	assert.Equal(t, 200*time.Millisecond, cb.EffectiveCooldown(), "flapping must trigger exponential backoff cooldown")

	status := cb.CanaryStatus(now.Add(3 * time.Second))
	assert.True(t, status.IsFlappingDampened, "status must report flapping dampening active")
	assert.Equal(t, 3, status.RecentTripsInWindow)

	// 4. Cooldown expiration into HalfOpen
	afterCooldown := now.Add(3*time.Second + 250*time.Millisecond)
	statusHalf := cb.CanaryStatus(afterCooldown)
	assert.Equal(t, eval.CircuitHalfOpen, statusHalf.State, "state must automatically transition to HalfOpen after cooldown")

	// In HalfOpen, regular dispatch must be refused, but canary dispatch must be permitted
	errReg := filter.CheckDispatch(context.Background(), false)
	assert.ErrorIs(t, errReg, eval.ErrCircuitBreakerHalfOpenNonCanary)

	errCanary := filter.CheckDispatch(context.Background(), true)
	assert.NoError(t, errCanary, "canary dispatch must be allowed in HalfOpen")

	// 5. Canary Recovery Pipeline: require CanaryConsecutivePassThreshold (2) consecutive passes
	canaryGood := &eval.RegressionScorecard{CompositeScore: 98.0}

	// Canary Pass 1
	resCanary1, err := cb.RecordEvaluation(canaryGood, afterCooldown.Add(10*time.Millisecond))
	require.NoError(t, err)
	assert.True(t, resCanary1.Passed)
	assert.False(t, resCanary1.Recovered)
	assert.Equal(t, eval.CircuitHalfOpen, cb.CanaryStatus(afterCooldown.Add(10*time.Millisecond)).State)

	// Canary Pass 2 (Completes recovery)
	resCanary2, err := cb.RecordEvaluation(canaryGood, afterCooldown.Add(20*time.Millisecond))
	require.NoError(t, err)
	assert.True(t, resCanary2.Passed)
	assert.True(t, resCanary2.Recovered, "second consecutive canary pass must trigger full recovery")
	assert.Equal(t, eval.CircuitClosed, cb.CanaryStatus(afterCooldown.Add(20*time.Millisecond)).State)
	assert.Equal(t, 100*time.Millisecond, cb.EffectiveCooldown(), "cooldown must reset to base value upon full recovery")

	// 6. Upward Ratchet: candidate superior to baseline updates baseline
	superiorCard := &eval.RegressionScorecard{CompositeScore: 110.0}
	resSup, err := cb.RecordEvaluation(superiorCard, afterCooldown.Add(30*time.Millisecond))
	require.NoError(t, err)
	assert.True(t, resSup.Passed)
	assert.Equal(t, 110.0, cb.Baseline().CompositeScore, "baseline must ratchet upward to superior score")
}
