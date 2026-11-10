package convergence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// Invariant 1: Static Floor Invariant (CRIT-1794278721649883000-3691707c)
func TestPolicyRule_StaticFloorInvariants(t *testing.T) {
	testNilAndEmptyFields(t)
	testInvalidEnums(t)
	testBoundsAndThresholds(t)
	testValidRule(t)
}

func testNilAndEmptyFields(t *testing.T) {
	if err := ValidateRule(nil); err == nil {
		t.Fatal("expected error for nil rule")
	}
	rEmptyID := &PolicyRule{Name: "Name", Impact: ImpactHigh, Action: ActionRetry, Weight: 50, MaxRetries: 3, BackoffBaseMs: 100}
	if err := ValidateRule(rEmptyID); err == nil {
		t.Fatal("expected error for empty ID")
	}
	rEmptyName := &PolicyRule{ID: "R1", Name: " ", Impact: ImpactHigh, Action: ActionRetry, Weight: 50, MaxRetries: 3, BackoffBaseMs: 100}
	if err := ValidateRule(rEmptyName); err == nil {
		t.Fatal("expected error for empty Name")
	}
}

func testInvalidEnums(t *testing.T) {
	rBadImpact := &PolicyRule{ID: "R1", Name: "N", Impact: "catastrophic", Action: ActionRetry, Weight: 50, MaxRetries: 3, BackoffBaseMs: 100}
	if err := ValidateRule(rBadImpact); err == nil {
		t.Fatal("expected error for invalid impact")
	}
	rBadAction := &PolicyRule{ID: "R1", Name: "N", Impact: ImpactHigh, Action: "reboot", Weight: 50, MaxRetries: 3, BackoffBaseMs: 100}
	if err := ValidateRule(rBadAction); err == nil {
		t.Fatal("expected error for invalid action")
	}
}

func testBoundsAndThresholds(t *testing.T) {
	rZeroWeight := &PolicyRule{ID: "R1", Name: "N", Impact: ImpactHigh, Action: ActionRetry, Weight: 0, MaxRetries: 3, BackoffBaseMs: 100}
	if err := ValidateRule(rZeroWeight); err == nil {
		t.Fatal("expected error for zero weight")
	}
	rHighWeight := &PolicyRule{ID: "R1", Name: "N", Impact: ImpactHigh, Action: ActionRetry, Weight: 101, MaxRetries: 3, BackoffBaseMs: 100}
	if err := ValidateRule(rHighWeight); err == nil {
		t.Fatal("expected error for weight > 100")
	}
	rNegRetries := &PolicyRule{ID: "R1", Name: "N", Impact: ImpactHigh, Action: ActionRetry, Weight: 50, MaxRetries: -1, BackoffBaseMs: 100}
	if err := ValidateRule(rNegRetries); err == nil {
		t.Fatal("expected error for negative retries")
	}
	rNegBackoff := &PolicyRule{ID: "R1", Name: "N", Impact: ImpactHigh, Action: ActionRetry, Weight: 50, MaxRetries: 3, BackoffBaseMs: 0}
	if err := ValidateRule(rNegBackoff); err == nil {
		t.Fatal("expected error for non-positive backoff")
	}
}

func testValidRule(t *testing.T) {
	rValid := &PolicyRule{
		ID:            "RULE-VALID",
		Name:          "Valid Test Rule",
		TargetFilter:  "test-failure",
		Impact:        ImpactHigh,
		Action:        ActionRetry,
		Weight:        80,
		MaxRetries:    3,
		BackoffBaseMs: 250,
		Enabled:       true,
	}
	if err := ValidateRule(rValid); err != nil {
		t.Fatalf("unexpected error for valid rule: %v", err)
	}
}

// Invariant 2: Operational Dynamic Proof (CRIT-1794278727036494000-24e2d2fd)
func TestPolicyOrchestrator_OperationalDynamicProof(t *testing.T) {
	evaluator := NewPolicyEvaluator()
	registerDynamicTestRules(t, evaluator)

	orchestrator := NewRemediationOrchestrator(evaluator)
	ctx := context.Background()

	testTimeoutDispatch(t, ctx, orchestrator)
	testCachePurgeDispatch(t, ctx, orchestrator)
}

func registerDynamicTestRules(t *testing.T, pe *PolicyEvaluator) {
	t.Helper()
	lowRule := &PolicyRule{
		ID: "RULE-LOW-CATCHALL", Name: "Low Weight Catchall", TargetFilter: "*",
		Impact: ImpactLow, Action: ActionTaskIsolation, Weight: 10, MaxRetries: 2, BackoffBaseMs: 100, Enabled: true,
	}
	highRule := &PolicyRule{
		ID: "RULE-HIGH-TIMEOUT", Name: "High Weight Timeout Rule", TargetFilter: "timeout, timed out",
		Impact: ImpactCritical, Action: ActionRetry, Weight: 90, MaxRetries: 3, BackoffBaseMs: 500, Enabled: true,
	}
	cacheRule := &PolicyRule{
		ID: "RULE-CACHE-CORRUPT", Name: "Cache Corrupt Purge", TargetFilter: "cache",
		Impact: ImpactHigh, Action: ActionCachePurge, Weight: 75, MaxRetries: 1, BackoffBaseMs: 200, Enabled: true,
	}
	for _, r := range []*PolicyRule{lowRule, highRule, cacheRule} {
		if err := pe.RegisterRule(r); err != nil {
			t.Fatalf("failed to register rule %s: %v", r.ID, err)
		}
	}
}

func testTimeoutDispatch(t *testing.T, ctx context.Context, ro *RemediationOrchestrator) {
	t.Helper()
	pTimeout := &TelemetryPayload{
		SessionID: "sess-100", JobID: "job-unit-tests", StepName: "go test execution",
		ExitCode: 1, ErrorMsg: "command timed out after 10m",
	}
	r1, err := ro.DispatchRemediation(ctx, pTimeout)
	if err != nil {
		t.Fatalf("unexpected dispatch error: %v", err)
	}
	if r1.RuleID != "RULE-HIGH-TIMEOUT" || r1.Action != ActionRetry || r1.Attempt != 1 {
		t.Errorf("unexpected receipt 1: %+v", r1)
	}
	if r1.BackoffDelay != 500*time.Millisecond {
		t.Errorf("expected backoff 500ms, got %v", r1.BackoffDelay)
	}

	r2, err := ro.DispatchRemediation(ctx, pTimeout)
	if err != nil {
		t.Fatalf("unexpected dispatch error on attempt 2: %v", err)
	}
	if r2.Attempt != 2 || r2.BackoffDelay != 1000*time.Millisecond {
		t.Errorf("expected attempt 2 with 1000ms backoff, got: %+v", r2)
	}
}

func testCachePurgeDispatch(t *testing.T, ctx context.Context, ro *RemediationOrchestrator) {
	t.Helper()
	pCache := &TelemetryPayload{
		SessionID: "sess-100", JobID: "job-cache-check", StepName: "build cache verify",
		ExitCode: 2, ErrorMsg: "checksum mismatch in build cache entry",
	}
	rCache, err := ro.DispatchRemediation(ctx, pCache)
	if err != nil {
		t.Fatalf("unexpected dispatch error for cache: %v", err)
	}
	if rCache.RuleID != "RULE-CACHE-CORRUPT" || rCache.Action != ActionCachePurge {
		t.Errorf("unexpected cache receipt: %+v", rCache)
	}
}

// Invariant 3: Negative Adversarial Boundary (CRIT-1794278732561365000-892ac816)
func TestPolicyOrchestrator_NegativeAdversarialBoundaries(t *testing.T) {
	evaluator := NewPolicyEvaluator()
	rule := &PolicyRule{
		ID: "RULE-BOUNDED-RETRY", Name: "Bounded Retry", TargetFilter: "fail",
		Impact: ImpactMedium, Action: ActionRetry, Weight: 50, MaxRetries: 2, BackoffBaseMs: 100, Enabled: true,
	}
	if err := evaluator.RegisterRule(rule); err != nil {
		t.Fatalf("failed to register rule: %v", err)
	}
	if err := evaluator.RegisterRule(rule); !errors.Is(err, ErrRuleAlreadyExists) {
		t.Fatalf("expected ErrRuleAlreadyExists, got %v", err)
	}

	orchestrator := NewRemediationOrchestrator(evaluator)
	ctx := context.Background()

	testCorruptedPayloads(t, ctx, orchestrator)
	testCyclicTriggerLoop(t, ctx, orchestrator)
	testConcurrentDispatch(t, ctx)
}

func testCorruptedPayloads(t *testing.T, ctx context.Context, ro *RemediationOrchestrator) {
	t.Helper()
	tests := []*TelemetryPayload{
		nil,
		{SessionID: "", JobID: "j1", ExitCode: 1},
		{SessionID: "s1", JobID: "", ExitCode: 1},
		{SessionID: "s1", JobID: "j1", ExitCode: -5},
	}
	for idx, p := range tests {
		if _, err := ro.DispatchRemediation(ctx, p); !errors.Is(err, ErrCorruptedTelemetry) {
			t.Fatalf("test %d: expected ErrCorruptedTelemetry, got %v", idx, err)
		}
	}
}

func testCyclicTriggerLoop(t *testing.T, ctx context.Context, ro *RemediationOrchestrator) {
	t.Helper()
	p := &TelemetryPayload{SessionID: "sess-cyclic", JobID: "job-loop", StepName: "step", ExitCode: 1, ErrorMsg: "fail"}
	if _, err := ro.DispatchRemediation(ctx, p); err != nil {
		t.Fatalf("attempt 1 error: %v", err)
	}
	if _, err := ro.DispatchRemediation(ctx, p); err != nil {
		t.Fatalf("attempt 2 error: %v", err)
	}
	if _, err := ro.DispatchRemediation(ctx, p); !errors.Is(err, ErrCyclicRemediationLoop) {
		t.Fatalf("attempt 3: expected ErrCyclicRemediationLoop, got %v", err)
	}
	ro.ResetSession("sess-cyclic")
	if _, err := ro.DispatchRemediation(ctx, p); err != nil {
		t.Fatalf("attempt after reset error: %v", err)
	}
}

func testConcurrentDispatch(t *testing.T, ctx context.Context) {
	t.Helper()
	pe := NewPolicyEvaluator()
	r := &PolicyRule{ID: "R-CONC", Name: "Conc", TargetFilter: "*", Impact: ImpactLow, Action: ActionRetry, Weight: 10, MaxRetries: 10, BackoffBaseMs: 10, Enabled: true}
	if err := pe.RegisterRule(r); err != nil {
		t.Fatalf("register rule failed: %v", err)
	}
	ro := NewRemediationOrchestrator(pe)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("convergence_concurrent_test", "concurrent telemetry dispatch").StartSimple(func() {
			defer wg.Done()
			p := &TelemetryPayload{SessionID: "sess-concurrent", JobID: "job-concurrent", StepName: "step", ExitCode: 1}
			if _, err := ro.DispatchRemediation(ctx, p); err != nil && !errors.Is(err, ErrCyclicRemediationLoop) {
				t.Errorf("dispatch remediation error: %v", err)
			}
		})
	}
	wg.Wait()
}
