package router

import (
	"context"
	"sync"
	"time"
)

// TimeoutEnvelope manages adaptive execution timeout boundaries across model tiers.
type TimeoutEnvelope struct {
	mu            sync.RWMutex
	baseTimeouts  map[ModelTier]time.Duration
	minTimeout    time.Duration
	maxTimeout    time.Duration
	tokenHeadroom time.Duration
	byteHeadroom  time.Duration
	padding       time.Duration
}

// NewTimeoutEnvelope creates a TimeoutEnvelope initialized with default production values.
func NewTimeoutEnvelope() *TimeoutEnvelope {
	return &TimeoutEnvelope{
		baseTimeouts: map[ModelTier]time.Duration{
			TierFast:     DefaultFastTimeout,
			TierStandard: DefaultStandardTimeout,
			TierHeavy:    DefaultHeavyTimeout,
			TierLocal:    DefaultLocalTimeout,
		},
		minTimeout:    DefaultMinTimeout,
		maxTimeout:    DefaultMaxTimeout,
		tokenHeadroom: DefaultTokenHeadroom,
		byteHeadroom:  DefaultByteHeadroom,
		padding:       DefaultTimeoutPadding,
	}
}

// WithBaseTimeout sets the baseline timeout for a specific model tier.
func (e *TimeoutEnvelope) WithBaseTimeout(tier ModelTier, d time.Duration) *TimeoutEnvelope {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.baseTimeouts == nil {
		e.baseTimeouts = make(map[ModelTier]time.Duration)
	}
	e.baseTimeouts[tier] = d
	return e
}

// WithBounds sets the minimum and maximum boundaries for computed timeouts.
func (e *TimeoutEnvelope) WithBounds(minDur, maxDur time.Duration) *TimeoutEnvelope {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.minTimeout = minDur
	e.maxTimeout = maxDur
	return e
}

// WithTokenHeadroom configures dynamic time headroom allocated per requested output token.
func (e *TimeoutEnvelope) WithTokenHeadroom(d time.Duration) *TimeoutEnvelope {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tokenHeadroom = d
	return e
}

// WithByteHeadroom configures dynamic time headroom allocated per prompt byte.
func (e *TimeoutEnvelope) WithByteHeadroom(d time.Duration) *TimeoutEnvelope {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.byteHeadroom = d
	return e
}

// ComputeTimeout calculates the adaptive timeout duration for a given tier and request.
func (e *TimeoutEnvelope) ComputeTimeout(tier ModelTier, req *ModelRequest) time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()

	base, exists := e.baseTimeouts[tier]
	if !exists || base <= 0 {
		base = DefaultStandardTimeout
	}

	computed := base
	if req != nil {
		if req.MaxTokens > 0 && e.tokenHeadroom > 0 {
			computed += time.Duration(req.MaxTokens) * e.tokenHeadroom
		}
		if len(req.Prompt) > 0 && e.byteHeadroom > 0 {
			computed += time.Duration(len(req.Prompt)) * e.byteHeadroom
		}
	}
	computed += e.padding

	if e.minTimeout > 0 && computed < e.minTimeout {
		computed = e.minTimeout
	}
	if e.maxTimeout > 0 && computed > e.maxTimeout {
		computed = e.maxTimeout
	}

	return computed
}

// WithTimeoutContext derives a context bounded by the adaptive timeout envelope.
func (e *TimeoutEnvelope) WithTimeoutContext(ctx context.Context, tier ModelTier, req *ModelRequest) (context.Context, context.CancelFunc, time.Duration) {
	dur := e.ComputeTimeout(tier, req)
	childCtx, cancel := context.WithTimeout(ctx, dur)
	return childCtx, cancel, dur
}
