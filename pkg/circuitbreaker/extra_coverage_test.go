// BLI-STARTER-COMMUNITY-027 / PRI-STARTER-COMMUNITY-027 coverage elevation
package circuitbreaker

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestCircuitBreaker_OpensAfterThreeFailures(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker()
	if err := cb.AllowRequest(); err != nil {
		t.Fatalf("closed: %v", err)
	}
	cb.RecordFailure()
	cb.RecordFailure()
	if err := cb.AllowRequest(); err != nil {
		t.Fatalf("two failures still closed: %v", err)
	}
	cb.RecordFailure()
	if err := cb.AllowRequest(); err == nil || !strings.Contains(err.Error(), "circuit_open") {
		t.Fatalf("expected open, got %v", err)
	}
}

func TestCircuitBreaker_HalfOpenThenSuccessCloses(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker()
	for i := 0; i < 3; i++ {
		cb.RecordFailure()
	}
	cb.mu.Lock()
	cb.lastFailure = time.Now().Add(-6 * time.Second)
	cb.mu.Unlock()
	if err := cb.AllowRequest(); err != nil {
		t.Fatalf("should enter half-open: %v", err)
	}
	if err := cb.AllowRequest(); err != nil {
		t.Fatalf("first half-open probe must pass: %v", err)
	}
	if err := cb.AllowRequest(); err == nil || !strings.Contains(err.Error(), "circuit_half_open_probing") {
		t.Fatalf("second probe should wait: %v", err)
	}
	cb.RecordSuccess()
	if err := cb.AllowRequest(); err != nil {
		t.Fatalf("success must close: %v", err)
	}
}

func TestCircuitBreaker_UnknownState(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker()
	cb.mu.Lock()
	cb.state = "broken"
	cb.mu.Unlock()
	if err := cb.AllowRequest(); err == nil || !strings.Contains(err.Error(), "unknown_circuit_state") {
		t.Fatalf("got %v", err)
	}
}

func TestCircuitBreaker_FailureWindowDecay(t *testing.T) {
	t.Parallel()
	cb := NewCircuitBreaker()
	cb.RecordFailure()
	cb.mu.Lock()
	cb.lastFailure = time.Now().Add(-31 * time.Second)
	cb.mu.Unlock()
	cb.RecordFailure()
	cb.RecordFailure()
	if err := cb.AllowRequest(); err != nil {
		t.Fatalf("decayed window should not open yet: %v", err)
	}
}

func TestTPMRateLimiter_CapsOversizedRequest(t *testing.T) {
	t.Parallel()
	rl := NewTPMRateLimiter(10)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := rl.Wait(ctx, 10_000); err != nil {
		t.Fatalf("oversized request should cap to tpm: %v", err)
	}
}

func TestExtractPackagePathFromRunWrapperCommand_Flags(t *testing.T) {
	t.Parallel()
	got := ExtractPackagePathFromRunWrapperCommand("go", []string{"test", "-count=1", "./pkg/storage"})
	if got != "pkg/storage" {
		t.Fatalf("got %q", got)
	}
	if ExtractPackagePathFromRunWrapperCommand("go", nil) != "" {
		t.Fatal("empty args")
	}
	shell := ExtractPackagePathFromRunWrapperCommand("/bin/sh", []string{"-c", "go test ./pkg/storage -run '^Foo$'"})
	if shell != "pkg/storage" {
		t.Fatalf("shell: %q", shell)
	}
	if ExtractPackagePathFromRunWrapperCommand("go", []string{"./..."}) != "" {
		t.Fatal("./... is not a package path")
	}
}

func TestConcurrencyLimiter_SetLimitAndAcquire(t *testing.T) {
	t.Parallel()
	l := NewConcurrencyLimiter(nil, 50*time.Millisecond).(*MapConcurrencyLimiter)
	l.SetLimit("pkg/foo", 1)
	ctx := context.Background()
	if err := l.Acquire(ctx, "pkg/foo"); err != nil {
		t.Fatal(err)
	}
	limit, inUse := l.Snapshot("pkg/foo")
	if limit != 1 || inUse != 1 {
		t.Fatalf("snapshot %d %d", limit, inUse)
	}
	l.Release("pkg/foo")
	_, inUse = l.Snapshot("pkg/foo")
	if inUse != 0 {
		t.Fatalf("after release inUse=%d", inUse)
	}
	if err := l.Acquire(ctx, ""); err != nil {
		t.Fatal(err)
	}
	l.Release("")
	if lim, use := l.Snapshot(""); lim != 0 || use != 0 {
		t.Fatalf("empty snapshot %d %d", lim, use)
	}
}

func TestConcurrencyLimiter_MergeLimitsCapsAt32(t *testing.T) {
	t.Parallel()
	l := NewConcurrencyLimiter(nil, time.Second).(*MapConcurrencyLimiter)
	l.MergeLimits(nil)
	l.MergeLimits(map[string]int{"./pkg/bar": 99, " ": 1})
	if l.limits["pkg/bar"] != 32 {
		t.Fatalf("cap at 32, got %d", l.limits["pkg/bar"])
	}
	if l.ShouldLimit("pkg/bar") != true {
		t.Fatal("ShouldLimit")
	}
	l.MergeLimits(map[string]int{"pkg/bar": 0})
	if l.ShouldLimit("pkg/bar") {
		t.Fatal("zero removes limit")
	}
}
