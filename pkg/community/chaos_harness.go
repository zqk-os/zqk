package community

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrChaosInjected        = errors.New("chaos: simulated fault injected")
	ErrChaosTimeout         = errors.New("chaos: simulated latency timeout")
	ErrInvalidProbability   = errors.New("chaos: error probability must be between 0.0 and 1.0")
	ErrChaosInvalidDuration = errors.New("chaos: delay duration cannot be negative")
)

// ChaosPolicy defines failure simulation parameters for testing resilience.
type ChaosPolicy struct {
	ErrorProbability float64       `json:"error_probability"`
	DelayDuration    time.Duration `json:"delay_duration"`
	FaultType        string        `json:"fault_type"`
}

// Validate checks invariant constraints on the policy.
func (p *ChaosPolicy) Validate() error {
	if p.ErrorProbability < 0.0 || p.ErrorProbability > 1.0 {
		return ErrInvalidProbability
	}
	if p.DelayDuration < 0 {
		return ErrChaosInvalidDuration
	}
	return nil
}

// ChaosHarness executes operations under controlled fault conditions.
type ChaosHarness struct {
	mu            sync.RWMutex
	policy        ChaosPolicy
	injectedCount int64
	successCount  int64
	stepCount     int64
}

// NewChaosHarness initializes a new chaos harness with policy.
func NewChaosHarness(policy ChaosPolicy) (*ChaosHarness, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &ChaosHarness{
		policy: policy,
	}, nil
}

// SetPolicy updates the active chaos policy dynamically.
func (h *ChaosHarness) SetPolicy(policy ChaosPolicy) error {
	if err := policy.Validate(); err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.policy = policy
	return nil
}

// Execute wraps an operation and applies configured fault injection.
func (h *ChaosHarness) Execute(ctx context.Context, op func(ctx context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	h.mu.Lock()
	h.stepCount++
	currentStep := h.stepCount
	policy := h.policy
	h.mu.Unlock()

	// Deterministic fault calculation based on step count and probability
	shouldInject := false
	if policy.ErrorProbability >= 1.0 {
		shouldInject = true
	} else if policy.ErrorProbability > 0.0 {
		// modulo 100 simulation
		threshold := int64(policy.ErrorProbability * 100)
		if (currentStep % 100) < threshold {
			shouldInject = true
		}
	}

	if policy.DelayDuration > 0 {
		select {
		case <-time.After(policy.DelayDuration):
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if shouldInject {
		h.mu.Lock()
		h.injectedCount++
		h.mu.Unlock()

		switch policy.FaultType {
		case "timeout":
			return fmt.Errorf("%w: timeout simulated", ErrChaosTimeout)
		default:
			return fmt.Errorf("%w: generic fault simulated", ErrChaosInjected)
		}
	}

	err := op(ctx)
	if err == nil {
		h.mu.Lock()
		h.successCount++
		h.mu.Unlock()
	}
	return err
}

// Stats returns the injection and execution telemetry.
func (h *ChaosHarness) Stats() (injected int64, succeeded int64, total int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.injectedCount, h.successCount, h.stepCount
}
