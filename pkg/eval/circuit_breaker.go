package eval

import (
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
)

// QualityCircuitBreakerState represents the tri-state of the quality circuit breaker.
type QualityCircuitBreakerState string

const (
	// CircuitClosed represents healthy operation; all dispatches and evaluations proceed.
	CircuitClosed QualityCircuitBreakerState = "Closed"
	// CircuitHalfOpen represents canary recovery probing; only canary dispatches are permitted.
	CircuitHalfOpen QualityCircuitBreakerState = "HalfOpen"
	// CircuitOpen represents tripped state; non-conforming model dispatches are quarantined.
	CircuitOpen QualityCircuitBreakerState = "Open"
)

var (
	// ErrCircuitBreakerOpen is returned when dispatches are halted due to detected regressions.
	ErrCircuitBreakerOpen = errfmt.Errorf("quality circuit breaker open: model dispatch halted due to evaluation regression")
	// ErrCircuitBreakerHalfOpenNonCanary is returned when regular dispatches are rejected during canary probing.
	ErrCircuitBreakerHalfOpenNonCanary = errfmt.Errorf("quality circuit breaker half-open: only canary dispatches permitted")
	// ErrInvalidRegressionScore is returned when an invalid benchmark score is supplied.
	ErrInvalidRegressionScore = errfmt.Errorf("invalid benchmark score: composite score must be non-negative")
)

// QualityCircuitBreakerConfig defines tuning parameters for benchmark regression gating.
type QualityCircuitBreakerConfig struct {
	// MaxAllowedRegressionDelta is the maximum fractional drop allowed from baseline (e.g. 0.05 for 5%).
	MaxAllowedRegressionDelta float64 `json:"max_allowed_regression_delta" yaml:"max_allowed_regression_delta"`
	// CooldownDuration is how long the circuit remains Open before transitioning to HalfOpen.
	CooldownDuration time.Duration `json:"cooldown_duration" yaml:"cooldown_duration"`
	// CanaryConsecutivePassThreshold is the number of passing canary runs required to return to Closed.
	CanaryConsecutivePassThreshold int `json:"canary_consecutive_pass_threshold" yaml:"canary_consecutive_pass_threshold"`
	// FlappingThreshold is the number of trips within FlappingWindow that triggers exponential dampening.
	FlappingThreshold int `json:"flapping_threshold" yaml:"flapping_threshold"`
	// FlappingWindow is the time window in which trips are accumulated.
	FlappingWindow time.Duration `json:"flapping_window" yaml:"flapping_window"`
}

// DefaultQualityCircuitBreakerConfig returns deterministic production defaults.
func DefaultQualityCircuitBreakerConfig() QualityCircuitBreakerConfig {
	return QualityCircuitBreakerConfig{
		MaxAllowedRegressionDelta:      0.05,
		CooldownDuration:               5 * time.Minute,
		CanaryConsecutivePassThreshold: 3,
		FlappingThreshold:              3,
		FlappingWindow:                 15 * time.Minute,
	}
}

// Validate verifies static invariants on the circuit breaker configuration.
func (c *QualityCircuitBreakerConfig) Validate() error {
	if c.MaxAllowedRegressionDelta < 0.0 || c.MaxAllowedRegressionDelta > 1.0 {
		return errfmt.Errorf("max_allowed_regression_delta must be between 0.0 and 1.0 (got %f)", c.MaxAllowedRegressionDelta)
	}
	if c.CooldownDuration <= 0 {
		return errfmt.Errorf("cooldown_duration must be positive (got %v)", c.CooldownDuration)
	}
	if c.CanaryConsecutivePassThreshold <= 0 {
		return errfmt.Errorf("canary_consecutive_pass_threshold must be >= 1 (got %d)", c.CanaryConsecutivePassThreshold)
	}
	if c.FlappingThreshold <= 0 {
		return errfmt.Errorf("flapping_threshold must be >= 1 (got %d)", c.FlappingThreshold)
	}
	if c.FlappingWindow <= 0 {
		return errfmt.Errorf("flapping_window must be positive (got %v)", c.FlappingWindow)
	}
	return nil
}

// RegressionScorecard encapsulates an evaluation benchmark run's composite performance.
type RegressionScorecard struct {
	RunID           string    `json:"run_id" yaml:"run_id"`
	ModelTier       string    `json:"model_tier" yaml:"model_tier"`
	CompositeScore  float64   `json:"composite_score" yaml:"composite_score"`
	LatencyP95Ms    float64   `json:"latency_p95_ms" yaml:"latency_p95_ms"`
	Accuracy        float64   `json:"accuracy" yaml:"accuracy"`
	TokenEfficiency float64   `json:"token_efficiency" yaml:"token_efficiency"`
	EvaluatedAt     time.Time `json:"evaluated_at" yaml:"evaluated_at"`
}

// FromModelScorecard converts a synthesized ModelScorecard into a RegressionScorecard.
func FromModelScorecard(runID string, m ModelScorecard) RegressionScorecard {
	return RegressionScorecard{
		RunID:           runID,
		ModelTier:       m.ModelID,
		CompositeScore:  m.CompositeScore,
		LatencyP95Ms:    m.LatencyP99Ms,
		Accuracy:        m.PassRate,
		TokenEfficiency: m.StabilityIndex,
		EvaluatedAt:     time.Now(),
	}
}

// QualityCircuitBreaker protects model execution pipelines from cascading regressions.
type QualityCircuitBreaker struct {
	mu           sync.RWMutex
	config       QualityCircuitBreakerConfig
	state        QualityCircuitBreakerState
	baseline     *RegressionScorecard
	lastTripTime time.Time
	tripHistory  []time.Time

	canaryPassCount int
	tripCount       int
	effectiveCool   time.Duration
}

// NewQualityCircuitBreaker constructs and validates a new quality circuit breaker.
func NewQualityCircuitBreaker(cfg QualityCircuitBreakerConfig) (*QualityCircuitBreaker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &QualityCircuitBreaker{
		config:        cfg,
		state:         CircuitClosed,
		tripHistory:   make([]time.Time, 0, 16),
		effectiveCool: cfg.CooldownDuration,
	}, nil
}

// State returns the current circuit breaker state, automatically checking cooldown transitions.
func (cb *QualityCircuitBreaker) State() QualityCircuitBreakerState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.currentStateLocked(time.Now())
}

func (cb *QualityCircuitBreaker) currentStateLocked(now time.Time) QualityCircuitBreakerState {
	if cb.state == CircuitOpen {
		if !cb.lastTripTime.IsZero() && now.Sub(cb.lastTripTime) >= cb.effectiveCool {
			cb.state = CircuitHalfOpen
			cb.canaryPassCount = 0
		}
	}
	return cb.state
}

// AllowDispatch checks whether a model dispatch is permitted based on circuit state.
func (cb *QualityCircuitBreaker) AllowDispatch(isCanary bool) (bool, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentStateLocked(time.Now())
	switch state {
	case CircuitClosed:
		return true, nil
	case CircuitHalfOpen:
		if isCanary {
			return true, nil
		}
		return false, ErrCircuitBreakerHalfOpenNonCanary
	case CircuitOpen:
		return false, ErrCircuitBreakerOpen
	default:
		return false, ErrCircuitBreakerOpen
	}
}

// Baseline returns the current active benchmark baseline scorecard.
func (cb *QualityCircuitBreaker) Baseline() *RegressionScorecard {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	if cb.baseline == nil {
		return nil
	}
	cp := *cb.baseline
	return &cp
}

// SetBaseline explicitly updates the benchmark baseline scorecard.
func (cb *QualityCircuitBreaker) SetBaseline(b *RegressionScorecard) error {
	if b == nil {
		return errfmt.Errorf("baseline scorecard cannot be nil")
	}
	if b.CompositeScore < 0 {
		return ErrInvalidRegressionScore
	}
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cp := *b
	cb.baseline = &cp
	return nil
}

// TripCount returns the cumulative number of times the circuit has tripped.
func (cb *QualityCircuitBreaker) TripCount() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.tripCount
}
