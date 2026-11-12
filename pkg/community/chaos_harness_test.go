package community

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestChaosHarness_StaticFloorInvariant(t *testing.T) {
	policy := ChaosPolicy{
		ErrorProbability: 0.25,
		DelayDuration:    10 * time.Millisecond,
		FaultType:        "timeout",
	}

	if err := policy.Validate(); err != nil {
		t.Fatalf("expected valid policy, got: %v", err)
	}

	harness, err := NewChaosHarness(policy)
	if err != nil {
		t.Fatalf("expected harness creation to succeed, got: %v", err)
	}

	injected, succeeded, total := harness.Stats()
	if total != 0 || injected != 0 || succeeded != 0 {
		t.Errorf("expected clean initial stats, got: %d, %d, %d", injected, succeeded, total)
	}
}

func TestChaosHarness_OperationalDynamicProof(t *testing.T) {
	ctx := context.Background()

	// 100% injection policy
	alwaysFailPolicy := ChaosPolicy{
		ErrorProbability: 1.0,
		FaultType:        "generic",
	}
	harness, err := NewChaosHarness(alwaysFailPolicy)
	if err != nil {
		t.Fatalf("failed to create harness: %v", err)
	}

	err = harness.Execute(ctx, func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrChaosInjected) {
		t.Fatalf("expected ErrChaosInjected, got: %v", err)
	}

	// 0% injection policy (all pass)
	passPolicy := ChaosPolicy{
		ErrorProbability: 0.0,
	}
	if err := harness.SetPolicy(passPolicy); err != nil {
		t.Fatalf("failed to update policy: %v", err)
	}

	executed := false
	err = harness.Execute(ctx, func(ctx context.Context) error {
		executed = true
		return nil
	})
	if err != nil {
		t.Fatalf("expected success with 0%% probability, got: %v", err)
	}
	if !executed {
		t.Errorf("expected op to be executed")
	}

	injected, succeeded, total := harness.Stats()
	if injected != 1 || succeeded != 1 || total != 2 {
		t.Errorf("expected stats 1, 1, 2; got: %d, %d, %d", injected, succeeded, total)
	}
}

func TestChaosHarness_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	// Case 1: Probability < 0.0
	badPolicy1 := ChaosPolicy{ErrorProbability: -0.1}
	if err := badPolicy1.Validate(); !errors.Is(err, ErrInvalidProbability) {
		t.Errorf("expected ErrInvalidProbability for negative probability, got: %v", err)
	}

	// Case 2: Probability > 1.0
	badPolicy2 := ChaosPolicy{ErrorProbability: 1.5}
	if err := badPolicy2.Validate(); !errors.Is(err, ErrInvalidProbability) {
		t.Errorf("expected ErrInvalidProbability for >1.0 probability, got: %v", err)
	}

	// Case 3: Negative delay duration
	badPolicy3 := ChaosPolicy{DelayDuration: -5 * time.Second}
	if err := badPolicy3.Validate(); !errors.Is(err, ErrChaosInvalidDuration) {
		t.Errorf("expected ErrChaosInvalidDuration for negative duration, got: %v", err)
	}

	// Case 4: Timeout fault simulation
	timeoutPolicy := ChaosPolicy{
		ErrorProbability: 1.0,
		FaultType:        "timeout",
	}
	harness, err := NewChaosHarness(timeoutPolicy)
	if err != nil {
		t.Fatalf("failed to create harness: %v", err)
	}

	err = harness.Execute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrChaosTimeout) {
		t.Errorf("expected ErrChaosTimeout, got: %v", err)
	}
}

func TestChaosHarness_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_AUTONOMOUS_PROVIDER_FAULT_INJECTION_AND_CHAOS_RESILIENCE_HARNESS_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0056 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0056 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0056 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
