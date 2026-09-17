package circuitbreaker

import (
	"context"
	"testing"
	"time"
)

func TestTPMRateLimiter_Wait(t *testing.T) {
	// 600 TPM means 10 tokens per second
	rl := NewTPMRateLimiter(600)

	// Should return immediately as bucket starts full
	ctx := context.Background()
	if err := rl.Wait(ctx, 600); err != nil {
		t.Fatalf("Failed to wait: %v", err)
	}

	// Next request for 10 tokens should take about 1 second
	start := time.Now()
	if err := rl.Wait(ctx, 10); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 500*time.Millisecond {
		t.Fatalf("expected to wait roughly 1 second, but only waited %v", elapsed)
	}
}

func TestTPMRateLimiter_Cancel(t *testing.T) {
	rl := NewTPMRateLimiter(600)

	ctx := context.Background()
	_ = rl.Wait(ctx, 600) // Empty the bucket

	cancelCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	// Ask for 100 tokens, which would take 10 seconds. Context will cancel in 0.1s.
	err := rl.Wait(cancelCtx, 100)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
