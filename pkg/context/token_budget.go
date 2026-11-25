// Plane: core
// Package context — token budget for hot→cold episodic eviction (stranger paging v0.2).
//
// TRACK: https://github.com/zqk-os/zqk/pull/1215 — cold episodic + token-budget eviction.
package context

import (
	"errors"
	"sync/atomic"
)

// Token budget defaults match CONTEXT_PAGING_DESIGN.md soft/hard guidance.
const (
	DefaultTokenSoftLimit = 80_000
	DefaultTokenHardLimit = 100_000
)

var (
	// ErrInvalidTokenBudgetLimits is returned when soft/hard limits are incoherent.
	ErrInvalidTokenBudgetLimits = errors.New("context: soft limit must be > 0 and <= hard limit")
)

// TokenBudget tracks estimated prompt/context token usage and signals eviction.
type TokenBudget struct {
	softLimit int64
	hardLimit int64
	current   atomic.Int64
}

// NewTokenBudget returns a budget with the given soft and hard limits.
func NewTokenBudget(softLimit, hardLimit int) (*TokenBudget, error) {
	if softLimit <= 0 || hardLimit < softLimit {
		return nil, ErrInvalidTokenBudgetLimits
	}
	return &TokenBudget{
		softLimit: int64(softLimit),
		hardLimit: int64(hardLimit),
	}, nil
}

// DefaultTokenBudget returns soft=80k / hard=100k.
func DefaultTokenBudget() *TokenBudget {
	b, err := NewTokenBudget(DefaultTokenSoftLimit, DefaultTokenHardLimit)
	if err != nil {
		panic(err)
	}
	return b
}

// SoftLimit returns the soft eviction threshold.
func (b *TokenBudget) SoftLimit() int {
	if b == nil {
		return 0
	}
	return int(b.softLimit)
}

// HardLimit returns the hard eviction threshold.
func (b *TokenBudget) HardLimit() int {
	if b == nil {
		return 0
	}
	return int(b.hardLimit)
}

// Current returns the current estimated token use.
func (b *TokenBudget) Current() int64 {
	if b == nil {
		return 0
	}
	return b.current.Load()
}

// Add increases current use by n tokens (n may be negative to release).
func (b *TokenBudget) Add(n int64) int64 {
	if b == nil {
		return 0
	}
	for {
		cur := b.current.Load()
		next := cur + n
		if next < 0 {
			next = 0
		}
		if b.current.CompareAndSwap(cur, next) {
			return next
		}
	}
}

// SetCurrent replaces the current use (clamped to >= 0).
func (b *TokenBudget) SetCurrent(n int64) {
	if b == nil {
		return
	}
	if n < 0 {
		n = 0
	}
	b.current.Store(n)
}

// AtSoftLimit reports whether current use meets or exceeds the soft limit.
func (b *TokenBudget) AtSoftLimit() bool {
	if b == nil {
		return false
	}
	return b.current.Load() >= b.softLimit
}

// AtHardLimit reports whether current use meets or exceeds the hard limit.
func (b *TokenBudget) AtHardLimit() bool {
	if b == nil {
		return false
	}
	return b.current.Load() >= b.hardLimit
}

// ShouldEvict is true when soft limit is reached (start hot→cold paging).
func (b *TokenBudget) ShouldEvict() bool {
	return b.AtSoftLimit()
}
