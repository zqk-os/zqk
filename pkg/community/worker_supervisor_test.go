package community

import (
	"errors"
	"testing"
	"time"
)

// TestWorkerSupervisor_StaticFloorInvariant verifies CRIT-1794507040806701000-0f45dba4.
func TestWorkerSupervisor_StaticFloorInvariant(t *testing.T) {
	sup := NewWorkerSupervisor()

	validSpec := WorkerProcessSpec{
		WorkerID:         "seat-worker-1",
		BinaryPath:       "/bin/zqk-seat",
		HeartbeatTimeout: 5 * time.Second,
		MaxCrashLimit:    3,
		BaseBackoff:      500 * time.Millisecond,
		MaxBackoff:       8 * time.Second,
	}

	if err := sup.RegisterWorker(validSpec); err != nil {
		t.Fatalf("expected RegisterWorker to succeed, got: %v", err)
	}

	status, err := sup.GetWorkerStatus("seat-worker-1")
	if err != nil {
		t.Fatalf("expected GetWorkerStatus to succeed, got: %v", err)
	}
	if status.Status != WorkerStatusHealthy {
		t.Errorf("expected initial status healthy, got: %s", status.Status)
	}
	if status.CrashCount != 0 {
		t.Errorf("expected 0 initial crashes, got: %d", status.CrashCount)
	}
}

// TestWorkerSupervisor_OperationalDynamicProof verifies CRIT-1794507050974965000-5dd81a46.
func TestWorkerSupervisor_OperationalDynamicProof(t *testing.T) {
	sup := NewWorkerSupervisor()
	baseTime := time.Date(2026, 11, 12, 10, 0, 0, 0, time.UTC)

	spec := WorkerProcessSpec{
		WorkerID:         "seat-worker-2",
		BinaryPath:       "/bin/zqk-seat",
		HeartbeatTimeout: 2 * time.Second,
		MaxCrashLimit:    4,
		BaseBackoff:      100 * time.Millisecond,
		MaxBackoff:       1 * time.Second,
	}
	_ = sup.RegisterWorker(spec)

	// Set heartbeat at baseTime
	w := sup.workers["seat-worker-2"]
	w.LastHeartbeat = baseTime

	// 1s elapsed: still healthy
	degraded := sup.EvaluateHealth(baseTime.Add(1 * time.Second))
	if len(degraded) != 0 {
		t.Errorf("expected 0 degraded workers, got: %v", degraded)
	}

	// 3s elapsed (> 2s timeout): degraded
	degraded = sup.EvaluateHealth(baseTime.Add(3 * time.Second))
	if len(degraded) != 1 || degraded[0] != "seat-worker-2" {
		t.Errorf("expected seat-worker-2 to be degraded, got: %v", degraded)
	}

	// Report crash 1: backoff = 100ms
	backoff1, err := sup.ReportCrash("seat-worker-2", baseTime.Add(3*time.Second))
	if err != nil {
		t.Fatalf("unexpected error reporting crash 1: %v", err)
	}
	if backoff1 != 100*time.Millisecond {
		t.Errorf("expected 100ms backoff, got: %v", backoff1)
	}

	// Report crash 2: backoff = 200ms
	backoff2, err := sup.ReportCrash("seat-worker-2", baseTime.Add(4*time.Second))
	if err != nil {
		t.Fatalf("unexpected error reporting crash 2: %v", err)
	}
	if backoff2 != 200*time.Millisecond {
		t.Errorf("expected 200ms backoff, got: %v", backoff2)
	}

	// Record heartbeat heals worker back to healthy and resets crash count
	err = sup.RecordHeartbeat("seat-worker-2")
	if err != nil {
		t.Fatalf("expected RecordHeartbeat to succeed, got: %v", err)
	}
	status, _ := sup.GetWorkerStatus("seat-worker-2")
	if status.Status != WorkerStatusHealthy || status.CrashCount != 0 {
		t.Errorf("expected healed worker with 0 crash count, got status=%s, crashes=%d", status.Status, status.CrashCount)
	}
}

// TestWorkerSupervisor_NegativeAdversarialBoundary verifies CRIT-1794507063046324000-f57b5ece.
func TestWorkerSupervisor_NegativeAdversarialBoundary(t *testing.T) {
	sup := NewWorkerSupervisor()

	// 1. Invalid Spec Rejections
	blankIDSpec := WorkerProcessSpec{
		WorkerID:         "  ",
		BinaryPath:       "/bin/zqk-seat",
		HeartbeatTimeout: 2 * time.Second,
		MaxCrashLimit:    3,
		BaseBackoff:      100 * time.Millisecond,
		MaxBackoff:       1 * time.Second,
	}
	err := sup.RegisterWorker(blankIDSpec)
	if !errors.Is(err, ErrBlankWorkerID) {
		t.Errorf("expected ErrBlankWorkerID, got: %v", err)
	}

	zeroLimitSpec := WorkerProcessSpec{
		WorkerID:         "worker-bad",
		BinaryPath:       "/bin/zqk-seat",
		HeartbeatTimeout: 2 * time.Second,
		MaxCrashLimit:    0,
		BaseBackoff:      100 * time.Millisecond,
		MaxBackoff:       1 * time.Second,
	}
	err = sup.RegisterWorker(zeroLimitSpec)
	if !errors.Is(err, ErrInvalidSpec) {
		t.Errorf("expected ErrInvalidSpec for zero limit, got: %v", err)
	}

	// 2. Unknown Worker Lookup
	_, err = sup.GetWorkerStatus("non-existent")
	if !errors.Is(err, ErrWorkerNotFound) {
		t.Errorf("expected ErrWorkerNotFound, got: %v", err)
	}

	// 3. Crash Loop Circuit Breaker Quarantine
	crashSpec := WorkerProcessSpec{
		WorkerID:         "flaky-worker",
		BinaryPath:       "/bin/zqk-seat",
		HeartbeatTimeout: 2 * time.Second,
		MaxCrashLimit:    2,
		BaseBackoff:      50 * time.Millisecond,
		MaxBackoff:       200 * time.Millisecond,
	}
	_ = sup.RegisterWorker(crashSpec)
	now := time.Now()

	// Crash 1: Allowed, restarts
	_, err = sup.ReportCrash("flaky-worker", now)
	if err != nil {
		t.Fatalf("crash 1 should not error: %v", err)
	}

	// Crash 2: Reaches MaxCrashLimit (2) -> Quarantined
	_, err = sup.ReportCrash("flaky-worker", now)
	if !errors.Is(err, ErrWorkerQuarantined) {
		t.Errorf("expected ErrWorkerQuarantined on limit breach, got: %v", err)
	}

	status, _ := sup.GetWorkerStatus("flaky-worker")
	if status.Status != WorkerStatusQuarantined {
		t.Errorf("expected worker to be quarantined, got: %s", status.Status)
	}

	// Subsequent heartbeat on quarantined worker rejected
	err = sup.RecordHeartbeat("flaky-worker")
	if !errors.Is(err, ErrWorkerQuarantined) {
		t.Errorf("expected ErrWorkerQuarantined on heartbeat for quarantined worker, got: %v", err)
	}
}
