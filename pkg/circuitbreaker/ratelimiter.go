package circuitbreaker

import (
	"sync"
	"time"
)

const (
	minRequestLimit    = 1
	defaultWindow      = time.Minute
	initialWindowCount = 1
)

// FixedWindowLimiter limits requests per key using a fixed time window (e.g. 1 minute).
// Each key has an independent counter that resets at the start of each window.
type FixedWindowLimiter struct {
	mu       sync.Mutex
	limit    int           // max requests per window
	window   time.Duration // window duration (e.g. 1*time.Minute)
	counters map[string]*windowCount
	nowFunc  func() time.Time // for tests
}

type windowCount struct {
	count int
	start time.Time
}

// NewFixedWindowLimiter creates a limiter allowing limit requests per window per key.
// Example: NewFixedWindowLimiter(60, time.Minute) = 60 requests per minute per key.
func NewFixedWindowLimiter(limit int, window time.Duration) *FixedWindowLimiter {
	if limit <= 0 {
		limit = minRequestLimit
	}
	if window <= 0 {
		window = defaultWindow
	}
	return &FixedWindowLimiter{
		limit:    limit,
		window:   window,
		counters: make(map[string]*windowCount),
		nowFunc:  time.Now,
	}
}

// Allow returns true if the key is under the limit, false if rate limit exceeded.
func (f *FixedWindowLimiter) Allow(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()

	now := f.nowFunc()
	wc, ok := f.counters[key]
	if !ok || now.Sub(wc.start) >= f.window {
		f.counters[key] = &windowCount{count: initialWindowCount, start: now}
		return true
	}
	if wc.count >= f.limit {
		return false
	}
	wc.count++
	return true
}

// SetNowFunc sets the function used to get current time (for tests).
func (f *FixedWindowLimiter) SetNowFunc(fn func() time.Time) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nowFunc = fn
}
