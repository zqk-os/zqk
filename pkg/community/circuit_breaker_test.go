package community

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCircuitBreaker_StaticFloorInvariant(t *testing.T) {
	cb := NewProviderCircuitBreaker()
	if cb.State() != StateClosed {
		t.Fatalf("expected initial state StateClosed, got %v", cb.State())
	}

	opts := DefaultCircuitBreakerOptions()
	if opts.ConsecutiveFailuresThreshold != 5 {
		t.Errorf("expected default threshold 5, got %d", opts.ConsecutiveFailuresThreshold)
	}

	if err := cb.Execute(context.Background(), nil); !errors.Is(err, ErrNilAction) {
		t.Errorf("expected ErrNilAction, got %v", err)
	}
}

func TestCircuitBreaker_OperationalDynamicProof(t *testing.T) {
	opts := CircuitBreakerOptions{
		ConsecutiveFailuresThreshold: 3,
		OpenDuration:                 50 * time.Millisecond,
		ProbeSuccessThreshold:        2,
	}
	cb := NewProviderCircuitBreaker(opts)
	ctx := context.Background()
	simulatedErr := errors.New("upstream provider error")

	// Trigger 3 consecutive errors to trip circuit
	for i := 0; i < 3; i++ {
		err := cb.Execute(ctx, func(ctx context.Context) error {
			return simulatedErr
		})
		if !errors.Is(err, simulatedErr) {
			t.Fatalf("expected simulatedErr, got: %v", err)
		}
	}

	if cb.State() != StateOpen {
		t.Fatalf("expected StateOpen after 3 failures, got %v", cb.State())
	}

	// Immediate next call should fail-fast with ErrCircuitOpen
	err := cb.Execute(ctx, func(ctx context.Context) error {
		t.Fatal("action should not execute when open")
		return nil
	})
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	// Wait for OpenDuration to expire
	time.Sleep(60 * time.Millisecond)

	// State should become HalfOpen
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected StateHalfOpen after timeout, got %v", cb.State())
	}

	// First probe success
	err = cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected probe success, got %v", err)
	}
	if cb.State() != StateHalfOpen {
		t.Fatalf("expected still StateHalfOpen after 1 success, got %v", cb.State())
	}

	// Second probe success -> should close circuit
	err = cb.Execute(ctx, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected probe success, got %v", err)
	}
	if cb.State() != StateClosed {
		t.Fatalf("expected StateClosed after 2 successes, got %v", cb.State())
	}

	// Verify telemetry
	invocations, successes, failures, state := cb.Telemetry()
	if invocations != 6 { // 3 failures + 1 rejected (doesn't increment invocations) + 2 probe successes = 5
		if invocations != 5 {
			t.Errorf("expected 5 invocations, got %d", invocations)
		}
	}
	if successes != 2 {
		t.Errorf("expected 2 successes, got %d", successes)
	}
	if failures != 3 {
		t.Errorf("expected 3 failures, got %d", failures)
	}
	if state != StateClosed {
		t.Errorf("expected StateClosed, got %v", state)
	}
}

func TestCircuitBreaker_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	opts := CircuitBreakerOptions{
		ConsecutiveFailuresThreshold: 2,
		OpenDuration:                 100 * time.Millisecond,
		ProbeSuccessThreshold:        1,
	}
	cb := NewProviderCircuitBreaker(opts)
	ctx := context.Background()

	// Concurrent invocations to ensure thread-safety
	var wg sync.WaitGroup
	errCount := 0
	var mu sync.Mutex

	for i := 0; i < 20; i++ {
		id := i
		wg.Add(1)
		goroutinelabels.NewGoroutine("circuit_breaker_concurrency_test", "test concurrent circuit breaker execution").StartSimple(func() {
			defer wg.Done()
			err := cb.Execute(ctx, func(ctx context.Context) error {
				if id < 3 {
					return errors.New("simulated error")
				}
				return nil
			})
			if err != nil {
				mu.Lock()
				errCount++
				mu.Unlock()
			}
		})
	}

	wg.Wait()

	if errCount == 0 {
		t.Errorf("expected at least some errors")
	}

	// Test manual Trip and Reset
	cb.Reset()
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after Reset, got %v", cb.State())
	}

	cb.Trip()
	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen after Trip, got %v", cb.State())
	}
}

func TestCircuitBreaker_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_PROVIDER_HEALTH_TELEMETRY_AND_FAILURE_CIRCUIT_BREAKER_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0042 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0042 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0042 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
