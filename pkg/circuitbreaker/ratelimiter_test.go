package circuitbreaker

import (
	"testing"
	"time"
)

func TestNewFixedWindowLimiter(t *testing.T) {
	t.Parallel()
	limiter := NewFixedWindowLimiter(2, time.Minute)
	if limiter == nil {
		t.Fatal("expected non-nil limiter")
	}
}

func TestFixedWindowLimiter_Allow_underLimit(t *testing.T) {
	t.Parallel()
	limiter := NewFixedWindowLimiter(3, time.Minute)
	key := "user1"

	if !limiter.Allow(key) {
		t.Error("first request should be allowed")
	}
	if !limiter.Allow(key) {
		t.Error("second request should be allowed")
	}
	if !limiter.Allow(key) {
		t.Error("third request should be allowed")
	}
	if limiter.Allow(key) {
		t.Error("fourth request should be denied")
	}
}

func TestFixedWindowLimiter_Allow_perKey(t *testing.T) {
	t.Parallel()
	limiter := NewFixedWindowLimiter(1, time.Minute)

	if !limiter.Allow("a") {
		t.Error("a: first should be allowed")
	}
	if limiter.Allow("a") {
		t.Error("a: second should be denied")
	}
	if !limiter.Allow("b") {
		t.Error("b: first should be allowed (different key)")
	}
}

func TestFixedWindowLimiter_Allow_windowReset(t *testing.T) {
	t.Parallel()
	now := time.Now()
	limiter := NewFixedWindowLimiter(2, time.Minute)
	limiter.SetNowFunc(func() time.Time { return now })

	key := "user1"
	if !limiter.Allow(key) {
		t.Error("first should be allowed")
	}
	if !limiter.Allow(key) {
		t.Error("second should be allowed")
	}
	if limiter.Allow(key) {
		t.Error("third should be denied")
	}

	// Advance past window
	limiter.SetNowFunc(func() time.Time { return now.Add(time.Minute + time.Second) })
	if !limiter.Allow(key) {
		t.Error("after window reset, should be allowed again")
	}
}

func TestFixedWindowLimiter_zeroLimit_usesOne(t *testing.T) {
	t.Parallel()
	limiter := NewFixedWindowLimiter(0, time.Minute)
	if !limiter.Allow("x") {
		t.Error("zero limit should default to 1, first request allowed")
	}
	if limiter.Allow("x") {
		t.Error("second request should be denied")
	}
}
