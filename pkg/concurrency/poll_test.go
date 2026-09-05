package concurrency_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
)

func TestPollUntil_SuccessImmediate(t *testing.T) {
	t.Parallel()
	called := false
	ok := concurrency.PollUntil(context.Background(), 5*time.Millisecond, func() bool {
		called = true
		return true
	})
	if !ok || !called {
		t.Fatalf("expected immediate success, got ok=%v called=%v", ok, called)
	}
}

func TestPollUntil_SuccessAfterRetries(t *testing.T) {
	t.Parallel()
	var count int32
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ok := concurrency.PollUntil(ctx, 5*time.Millisecond, func() bool {
		return atomic.AddInt32(&count, 1) >= 3
	})
	if !ok {
		t.Fatal("expected polling to succeed after retries")
	}
}

func TestPollUntil_Timeout(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	ok := concurrency.PollUntil(ctx, 5*time.Millisecond, func() bool {
		return false
	})
	if ok {
		t.Fatal("expected polling to timeout and return false")
	}
}

func TestPollTimeout_Helper(t *testing.T) {
	t.Parallel()
	var flag atomic.Bool
	time.AfterFunc(20*time.Millisecond, func() {
		flag.Store(true)
	})

	ok := concurrency.PollTimeout(200*time.Millisecond, 5*time.Millisecond, func() bool {
		return flag.Load()
	})
	if !ok {
		t.Fatal("expected PollTimeout to succeed")
	}
}
