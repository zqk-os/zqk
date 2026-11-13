package community

import (
	"errors"
	"testing"
)

// TestRunwayGovernor_StaticFloorInvariant verifies CRIT-1794507522641473000-2ff16f28.
func TestRunwayGovernor_StaticFloorInvariant(t *testing.T) {
	policy := RunwayCapacityPolicy{
		MinWorkers:           2,
		MaxWorkers:           10,
		TargetTasksPerWorker: 5,
		MinRunwayTokens:      1000,
		MaxQueueDepth:        50,
	}

	gov, err := NewRunwayGovernor(policy, 10000)
	if err != nil {
		t.Fatalf("expected NewRunwayGovernor to succeed, got: %v", err)
	}

	workers, queueDepth, tokens := gov.GetMetrics()
	if workers != 2 {
		t.Errorf("expected initial workers to match MinWorkers (2), got %d", workers)
	}
	if queueDepth != 0 {
		t.Errorf("expected initial queue depth 0, got %d", queueDepth)
	}
	if tokens != 10000 {
		t.Errorf("expected initial tokens 10000, got %d", tokens)
	}
}

// TestRunwayGovernor_OperationalDynamicProof verifies CRIT-1794507530620200000-cfc497e9.
func TestRunwayGovernor_OperationalDynamicProof(t *testing.T) {
	policy := RunwayCapacityPolicy{
		MinWorkers:           2,
		MaxWorkers:           8,
		TargetTasksPerWorker: 4,
		MinRunwayTokens:      500,
		MaxQueueDepth:        40,
	}

	gov, _ := NewRunwayGovernor(policy, 5000)

	// Admit 12 tasks (each costing 50 tokens) -> 12 tasks / 4 target = 3 workers desired
	for i := 0; i < 12; i++ {
		err := gov.AdmitTask(50)
		if err != nil {
			t.Fatalf("expected task %d to be admitted, got: %v", i, err)
		}
	}

	workers := gov.EvaluateScaling()
	if workers != 3 {
		t.Errorf("expected 3 workers for 12 tasks, got %d", workers)
	}

	// Admit 16 more tasks (total 28 tasks) -> 28 / 4 = 7 workers desired
	for i := 0; i < 16; i++ {
		_ = gov.AdmitTask(50)
	}
	workers = gov.EvaluateScaling()
	if workers != 7 {
		t.Errorf("expected 7 workers for 28 tasks, got %d", workers)
	}

	// Complete 20 tasks
	for i := 0; i < 20; i++ {
		gov.CompleteTask()
	}

	// Now 8 tasks remaining -> 8 / 4 = 2 workers
	workers = gov.EvaluateScaling()
	if workers != 2 {
		t.Errorf("expected 2 workers for 8 remaining tasks, got %d", workers)
	}
}

// TestRunwayGovernor_NegativeAdversarialBoundary verifies CRIT-1794507538893561000-072261f0.
func TestRunwayGovernor_NegativeAdversarialBoundary(t *testing.T) {
	// 1. Invalid Policy Rejections
	badPolicy := RunwayCapacityPolicy{
		MinWorkers: 5,
		MaxWorkers: 2, // Max < Min
	}
	_, err := NewRunwayGovernor(badPolicy, 1000)
	if !errors.Is(err, ErrInvalidPolicy) {
		t.Errorf("expected ErrInvalidPolicy when MaxWorkers < MinWorkers, got: %v", err)
	}

	policy := RunwayCapacityPolicy{
		MinWorkers:           1,
		MaxWorkers:           4,
		TargetTasksPerWorker: 2,
		MinRunwayTokens:      200,
		MaxQueueDepth:        5,
	}

	// 2. Negative Token Cost Rejection
	gov, _ := NewRunwayGovernor(policy, 1000)
	err = gov.AdmitTask(-10)
	if !errors.Is(err, ErrNegativeCost) {
		t.Errorf("expected ErrNegativeCost for negative estimated cost, got: %v", err)
	}

	// 3. Queue Saturation Load Shedding
	for i := 0; i < 5; i++ {
		_ = gov.AdmitTask(50)
	}
	// 6th task exceeds MaxQueueDepth (5) -> ErrAdmissionShed
	err = gov.AdmitTask(50)
	if !errors.Is(err, ErrAdmissionShed) {
		t.Errorf("expected ErrAdmissionShed when queue reaches max capacity, got: %v", err)
	}

	// 4. Runway Token Exhaustion
	govLowRunway, _ := NewRunwayGovernor(policy, 250) // Starting with 250 tokens, min reserve = 200
	// 1st task costs 40 -> 210 remaining (>= 200 min reserve) -> succeeds
	err = govLowRunway.AdmitTask(40)
	if err != nil {
		t.Fatalf("expected admission to succeed with remaining tokens, got: %v", err)
	}
	// 2nd task costs 20 -> 210 - 20 = 190 (< 200 min reserve) -> ErrRunwayExhausted
	err = govLowRunway.AdmitTask(20)
	if !errors.Is(err, ErrRunwayExhausted) {
		t.Errorf("expected ErrRunwayExhausted when breaching min token reserve, got: %v", err)
	}
}
