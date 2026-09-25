package verification_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/kernel/verification"
)

// TST-KERNEL-EXECUTION-ORGANIZER-SUITE: Composite Execution Organizer Test Harness
// Verifies:
// 1. Static Floor: CRIT-EXEC-ORG-TOPOLOGY-METADATA
// 2. Operational Proof: CRIT-EXEC-ORG-OPERATIONAL-DISPATCH
// 3. Negative Invariant: CRIT-EXEC-ORG-ADVERSARIAL-ISOLATION-SAFETY

// TestExecutionOrganizerCriteriaRatioAndSchema asserts that test cases must bind
// multiple criteria (CRIT:TST >= 2:1) and rejects 1:1 single-verification stubs.
func TestExecutionOrganizerCriteriaRatioAndSchema(t *testing.T) {
	eng := verification.NewEngine()

	t.Run("Rejects 1:1 Single Verification Test Cases", func(t *testing.T) {
		singleCriteriaOrganizer := &verification.ExecutionOrganizer{
			TestCaseID: "TST-SINGLE-STUB",
			Mode:       verification.TopologySequential,
			Stages: []verification.VerificationStage{
				{
					ID:       "CRIT-SINGLE-001",
					Category: verification.CategoryStaticFloor,
					Verify:   func(ctx context.Context) error { return nil },
				},
			},
		}

		err := eng.ValidateOrganizer(singleCriteriaOrganizer)
		if err == nil {
			t.Fatal("expected error on 1:1 single criteria test case, got nil")
		}
		if !errors.Is(err, verification.ErrInsufficientCriteriaRatio) {
			t.Errorf("expected ErrInsufficientCriteriaRatio, got %v", err)
		}
	})

	t.Run("Accepts Three-Fold Proof Criteria Ratio (3:1)", func(t *testing.T) {
		threeFoldOrganizer := &verification.ExecutionOrganizer{
			TestCaseID: "TST-THREE-FOLD-COMPLIANT",
			Mode:       verification.TopologyHybridDAG,
			Stages: []verification.VerificationStage{
				{ID: "CRIT-001", Category: verification.CategoryStaticFloor},
				{ID: "CRIT-002", Category: verification.CategoryOperationalProof},
				{ID: "CRIT-003", Category: verification.CategoryNegativeInvariant},
			},
		}

		err := eng.ValidateOrganizer(threeFoldOrganizer)
		if err != nil {
			t.Fatalf("expected 3:1 organizer to validate successfully, got: %v", err)
		}
	})
}

// TestExecutionOrganizerDispatchPipelines verifies multi-stage verification dispatch across
// sequential, concurrent, and hybrid DAG execution topologies.
func TestExecutionOrganizerDispatchPipelines(t *testing.T) {
	eng := verification.NewEngine()
	ctx := context.Background()

	t.Run("Concurrent Execution Runs Stages in Parallel", func(t *testing.T) {
		var activeCount int32
		var maxConcurrent int32

		makeStage := func(id string) verification.VerificationStage {
			return verification.VerificationStage{
				ID:       id,
				Category: verification.CategoryOperationalProof,
				Verify: func(ctx context.Context) error {
					curr := atomic.AddInt32(&activeCount, 1)
					defer atomic.AddInt32(&activeCount, -1)

					// Track peak concurrency
					for {
						oldMax := atomic.LoadInt32(&maxConcurrent)
						if curr <= oldMax || atomic.CompareAndSwapInt32(&maxConcurrent, oldMax, curr) {
							break
						}
					}
					time.Sleep(20 * time.Millisecond)
					return nil
				},
			}
		}

		org := &verification.ExecutionOrganizer{
			TestCaseID: "TST-CONCURRENT-VERIFICATION",
			Mode:       verification.TopologyConcurrent,
			Stages: []verification.VerificationStage{
				makeStage("CRIT-CONC-01"),
				makeStage("CRIT-CONC-02"),
				makeStage("CRIT-CONC-03"),
			},
		}

		report, err := eng.Execute(ctx, org)
		if err != nil {
			t.Fatalf("unexpected execution error: %v", err)
		}
		if !report.Passed {
			t.Errorf("expected report.Passed == true")
		}

		peak := atomic.LoadInt32(&maxConcurrent)
		if peak < 2 {
			t.Errorf("expected concurrent execution (peak >= 2), observed peak = %d", peak)
		}
	})

	t.Run("Hybrid DAG Enforces Barrier Synchronization", func(t *testing.T) {
		var executionOrder []string
		var mu atomic.Value

		org := &verification.ExecutionOrganizer{
			TestCaseID: "TST-HYBRID-BARRIER",
			Mode:       verification.TopologyHybridDAG,
			Stages: []verification.VerificationStage{
				{
					ID:       "STAGE-A-INITIALIZE",
					Category: verification.CategoryStaticFloor,
					Verify: func(ctx context.Context) error {
						executionOrder = append(executionOrder, "A")
						return nil
					},
				},
				{
					ID:        "STAGE-B-DEPENDENT",
					Category:  verification.CategoryOperationalProof,
					DependsOn: []string{"STAGE-A-INITIALIZE"},
					Verify: func(ctx context.Context) error {
						executionOrder = append(executionOrder, "B")
						return nil
					},
				},
			},
		}
		_ = mu

		report, err := eng.Execute(ctx, org)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !report.Passed {
			t.Errorf("expected report to pass")
		}

		if len(executionOrder) != 2 || executionOrder[0] != "A" || executionOrder[1] != "B" {
			t.Errorf("expected barrier order [A, B], got %v", executionOrder)
		}
	})
}

// TestExecutionOrganizerFaultIsolation verifies that panics and failures in one branch
// do not crash the runner or contaminate sibling concurrent stages.
func TestExecutionOrganizerFaultIsolation(t *testing.T) {
	eng := verification.NewEngine()
	ctx := context.Background()

	t.Run("Catches Panic Safely and Reports Atomic Diagnostic", func(t *testing.T) {
		var siblingCompleted int32

		org := &verification.ExecutionOrganizer{
			TestCaseID: "TST-PANIC-ISOLATION",
			Mode:       verification.TopologyConcurrent,
			Stages: []verification.VerificationStage{
				{
					ID:       "CRIT-PANIC-RUNNER",
					Category: verification.CategoryNegativeInvariant,
					Verify: func(ctx context.Context) error {
						panic("simulated critical crash or memory corruption in test harness")
					},
				},
				{
					ID:       "CRIT-SIBLING-HEALTHY",
					Category: verification.CategoryOperationalProof,
					Verify: func(ctx context.Context) error {
						time.Sleep(15 * time.Millisecond)
						atomic.StoreInt32(&siblingCompleted, 1)
						return nil
					},
				},
			},
		}

		report, err := eng.Execute(ctx, org)
		if err != nil {
			t.Fatalf("unexpected error returned by engine: %v", err)
		}

		// Overall report must fail because one stage panicked
		if report.Passed {
			t.Errorf("expected overall report to fail due to stage panic")
		}

		if report.PanicsCaught != 1 {
			t.Errorf("expected 1 caught panic, got %d", report.PanicsCaught)
		}

		panicStage := report.StageResults["CRIT-PANIC-RUNNER"]
		if panicStage == nil || !panicStage.Panicked {
			t.Errorf("expected panicStage to record Panicked == true")
		}

		// Assert sibling completed despite panic
		if atomic.LoadInt32(&siblingCompleted) != 1 {
			t.Errorf("expected sibling stage to complete independently")
		}

		siblingStage := report.StageResults["CRIT-SIBLING-HEALTHY"]
		if siblingStage == nil || !siblingStage.Passed {
			t.Errorf("expected sibling stage to record Passed == true")
		}
	})
}
