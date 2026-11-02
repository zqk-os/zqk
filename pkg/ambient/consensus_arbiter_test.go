package ambient

import (
	"errors"
	"testing"
	"time"
)

func TestConsensusArbiter_StaticInvariantsAndFunctionalAcceptance(t *testing.T) {
	t.Run("ConsensusStateEnum", testConsensusStateEnum)
	t.Run("TaskLeaseValidation", testTaskLeaseValidation)
	t.Run("TaskLifecycleTransitions", testTaskLifecycleTransitions)
	t.Run("TaskRevocation", testTaskRevocation)
	t.Run("FIFOLeasing", testFIFOLeasing)
}

func testConsensusStateEnum(t *testing.T) {
	validStates := []ConsensusState{
		ConsensusStatePending,
		ConsensusStateLeased,
		ConsensusStateCommitted,
		ConsensusStateRevoked,
		ConsensusStateExpired,
	}

	for _, s := range validStates {
		if !s.IsValid() {
			t.Fatalf("expected state %q to be valid", s)
		}
		if s.String() != string(s) {
			t.Fatalf("expected stringer %q to equal %q", s.String(), string(s))
		}
	}

	invalidStates := []ConsensusState{"", "unknown", "failed"}
	for _, s := range invalidStates {
		if s.IsValid() {
			t.Fatalf("expected state %q to be invalid", s)
		}
	}
}

func testTaskLeaseValidation(t *testing.T) {
	var nilLease *TaskLease
	if err := nilLease.Validate(); !errors.Is(err, ErrNilTaskLease) {
		t.Fatalf("expected ErrNilTaskLease for nil pointer, got %v", err)
	}

	emptyTaskLease := &TaskLease{State: ConsensusStatePending}
	if err := emptyTaskLease.Validate(); !errors.Is(err, ErrInvalidTaskID) {
		t.Fatalf("expected ErrInvalidTaskID for empty task id, got %v", err)
	}

	invalidStateLease := &TaskLease{TaskID: "task-1", State: ConsensusState("bogus")}
	if err := invalidStateLease.Validate(); !errors.Is(err, ErrInvalidConsensusState) {
		t.Fatalf("expected ErrInvalidConsensusState, got %v", err)
	}

	validLease := &TaskLease{
		TaskID:    "task-1",
		State:     ConsensusStatePending,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := validLease.Validate(); err != nil {
		t.Fatalf("expected nil error for valid lease, got %v", err)
	}
}

func testTaskLifecycleTransitions(t *testing.T) {
	now := time.Now()
	arb := NewConsensusArbiter(WithClock(func() time.Time { return now }))

	lease, err := arb.ProposeTask("task-100", []byte("payload"))
	if err != nil || lease == nil || lease.State != ConsensusStatePending {
		t.Fatalf("unexpected propose result: lease=%+v, err=%v", lease, err)
	}

	leased, err := arb.LeaseTask("task-100", "worker-1", 10*time.Second)
	if err != nil || leased.State != ConsensusStateLeased || leased.WorkerID != "worker-1" {
		t.Fatalf("unexpected lease result: leased=%+v, err=%v", leased, err)
	}

	if _, err := arb.LeaseTask("task-100", "worker-2", 10*time.Second); !errors.Is(err, ErrTaskAlreadyLeased) {
		t.Fatalf("expected ErrTaskAlreadyLeased, got %v", err)
	}

	if err := arb.CommitTask("task-100", "worker-wrong"); !errors.Is(err, ErrUnauthorizedWorker) {
		t.Fatalf("expected ErrUnauthorizedWorker, got %v", err)
	}

	if err := arb.CommitTask("task-100", "worker-1"); err != nil {
		t.Fatalf("unexpected commit error: %v", err)
	}

	if err := arb.CommitTask("task-100", "worker-1"); !errors.Is(err, ErrTaskAlreadyCommitted) {
		t.Fatalf("expected ErrTaskAlreadyCommitted, got %v", err)
	}
}

func testTaskRevocation(t *testing.T) {
	arb := NewConsensusArbiter()

	if err := arb.RevokeTask("non-existent", "test"); !errors.Is(err, ErrLeaseNotFound) {
		t.Fatalf("expected ErrLeaseNotFound, got %v", err)
	}

	_, _ = arb.ProposeTask("task-rev", nil)
	if err := arb.RevokeTask("task-rev", "canceled"); err != nil {
		t.Fatalf("unexpected revoke error: %v", err)
	}

	lease, ok := arb.GetLease("task-rev")
	if !ok || lease.State != ConsensusStateRevoked {
		t.Fatalf("expected lease state revoked, got %+v (ok=%v)", lease, ok)
	}
}

func testFIFOLeasing(t *testing.T) {
	arb := NewConsensusArbiter()

	_, _ = arb.ProposeTask("task-fifo-1", nil)
	_, _ = arb.ProposeTask("task-fifo-2", nil)
	_, _ = arb.ProposeTask("task-fifo-3", nil)

	l1, err1 := arb.LeaseNext("worker-a", 5*time.Second)
	if err1 != nil || l1.TaskID != "task-fifo-1" {
		t.Fatalf("expected task-fifo-1, got %+v (err=%v)", l1, err1)
	}

	l2, err2 := arb.LeaseNext("worker-b", 5*time.Second)
	if err2 != nil || l2.TaskID != "task-fifo-2" {
		t.Fatalf("expected task-fifo-2, got %+v (err=%v)", l2, err2)
	}

	l3, err3 := arb.LeaseNext("worker-c", 5*time.Second)
	if err3 != nil || l3.TaskID != "task-fifo-3" {
		t.Fatalf("expected task-fifo-3, got %+v (err=%v)", l3, err3)
	}

	_, err4 := arb.LeaseNext("worker-d", 5*time.Second)
	if !errors.Is(err4, ErrNoPendingTasks) {
		t.Fatalf("expected ErrNoPendingTasks, got %v", err4)
	}
}
