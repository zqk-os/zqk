package ambient

import (
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/validation/qa"
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

	if _, err := arb.ProposeTask("task-rev", nil); err != nil {
		t.Fatalf("unexpected propose error: %v", err)
	}
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

	if _, err := arb.ProposeTask("task-fifo-1", nil); err != nil {
		t.Fatalf("propose 1 failed: %v", err)
	}
	if _, err := arb.ProposeTask("task-fifo-2", nil); err != nil {
		t.Fatalf("propose 2 failed: %v", err)
	}
	if _, err := arb.ProposeTask("task-fifo-3", nil); err != nil {
		t.Fatalf("propose 3 failed: %v", err)
	}

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

func TestConsensusArbiter_BoundaryAndErrorHandling(t *testing.T) {
	verifyLeaseTimeoutRevocation(t)
	verifyRenewLeaseValidation(t)
	verifyContextCancellation(t)
	verifyNilReceiverSafety(t)
	verifyInvalidInputs(t)
}

func verifyLeaseTimeoutRevocation(t *testing.T) {
	t.Run("LeaseTimeoutRevocation", func(t *testing.T) {
		fastReapInterval := 10 * time.Millisecond
		arbiter := NewConsensusArbiter(WithReapInterval(fastReapInterval))
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := arbiter.Start(ctx); err != nil {
			t.Fatalf("failed to start arbiter: %v", err)
		}
		defer func() {
			if err := arbiter.Stop(); err != nil {
				t.Fatalf("failed to stop arbiter: %v", err)
			}
		}()

		shortDuration := 30 * time.Millisecond
		lease, err := arbiter.AcquireLease("task-reap-1", "worker-alpha", shortDuration)
		if err != nil {
			t.Fatalf("failed to acquire lease: %v", err)
		}
		if lease.State != ConsensusStateLeased {
			t.Fatalf("expected state leased, got %s", lease.State)
		}

		time.Sleep(70 * time.Millisecond)

		expiredLease, ok := arbiter.GetLease("task-reap-1")
		if !ok {
			t.Fatalf("failed to get reaped lease")
		}
		if expiredLease.State != ConsensusStateExpired {
			t.Fatalf("expected state expired, got %s", expiredLease.State)
		}
		if expiredLease.WorkerID != "" {
			t.Fatalf("expected revoked worker ownership, got %s", expiredLease.WorkerID)
		}

		newLease, err := arbiter.AcquireLease("task-reap-1", "worker-beta", 1*time.Second)
		if err != nil {
			t.Fatalf("competing worker failed to acquire released task: %v", err)
		}
		if newLease.WorkerID != "worker-beta" {
			t.Fatalf("expected worker-beta, got %s", newLease.WorkerID)
		}
	})
}

func verifyRenewLeaseValidation(t *testing.T) {
	t.Run("RenewLeaseValidation", func(t *testing.T) {
		arbiter := NewConsensusArbiter(WithReapInterval(100 * time.Millisecond))
		lease, err := arbiter.AcquireLease("task-renew-1", "worker-prime", 300*time.Millisecond)
		if err != nil {
			t.Fatalf("initial acquire failed: %v", err)
		}

		_, err = arbiter.RenewLease("task-renew-1", "worker-intruder", 100*time.Millisecond)
		if !errors.Is(err, ErrUnauthorizedWorker) {
			t.Fatalf("expected ErrUnauthorizedWorker for intruder, got %v", err)
		}

		_, err = arbiter.AcquireLease("task-renew-1", "worker-intruder", 100*time.Millisecond)
		if !errors.Is(err, ErrTaskAlreadyLeased) {
			t.Fatalf("expected ErrTaskAlreadyLeased for competing worker, got %v", err)
		}

		renewed, err := arbiter.RenewLease("task-renew-1", "worker-prime", 300*time.Millisecond)
		if err != nil {
			t.Fatalf("authorized renewal failed: %v", err)
		}
		if !renewed.ExpiresAt.After(lease.ExpiresAt) {
			t.Fatalf("expected extended expiration, got %v <= %v", renewed.ExpiresAt, lease.ExpiresAt)
		}

		err = arbiter.CommitTask("task-renew-1", "worker-intruder")
		if !errors.Is(err, ErrUnauthorizedWorker) {
			t.Fatalf("expected ErrUnauthorizedWorker on commit, got %v", err)
		}

		err = arbiter.CommitTask("task-renew-1", "worker-prime")
		if err != nil {
			t.Fatalf("authorized commit failed: %v", err)
		}
	})
}

func verifyContextCancellation(t *testing.T) {
	t.Run("ContextCancellation", func(t *testing.T) {
		arbiter := NewConsensusArbiter(WithReapInterval(10 * time.Millisecond))
		ctx, cancel := context.WithCancel(context.Background())

		if err := arbiter.Start(ctx); err != nil {
			t.Fatalf("failed to start arbiter: %v", err)
		}

		cancel()
		time.Sleep(30 * time.Millisecond)

		if err := arbiter.Stop(); err != nil {
			t.Fatalf("failed to stop after cancellation: %v", err)
		}
	})
}

func verifyNilReceiverSafety(t *testing.T) {
	t.Run("NilReceiverSafety", func(t *testing.T) {
		var nilArbiter *AmbientConsensusArbiter
		ctx := context.Background()

		if err := nilArbiter.Start(ctx); !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on Start, got %v", err)
		}
		if err := nilArbiter.Stop(); !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on Stop, got %v", err)
		}
		if _, err := nilArbiter.AcquireLease("t1", "w1", time.Second); !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on AcquireLease, got %v", err)
		}
		if _, err := nilArbiter.LeaseTask("t1", "w1", time.Second); !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on LeaseTask, got %v", err)
		}
		if _, err := nilArbiter.LeaseNext("w1", time.Second); !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on LeaseNext, got %v", err)
		}
		if _, err := nilArbiter.RenewLease("t1", "w1", time.Second); !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on RenewLease, got %v", err)
		}
		if err := nilArbiter.CommitTask("t1", "w1"); !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on CommitTask, got %v", err)
		}
		if err := nilArbiter.RevokeTask("t1", "reason"); !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on RevokeTask, got %v", err)
		}
		if l, ok := nilArbiter.GetLease("t1"); l != nil || ok {
			t.Fatalf("expected nil, false on GetLease, got %v, %v", l, ok)
		}
		if _, err := nilArbiter.LookupLease("t1"); !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on LookupLease, got %v", err)
		}
		if reaped := nilArbiter.ReapExpiredLeases(); reaped != 0 {
			t.Fatalf("expected 0 reaped on nil receiver, got %d", reaped)
		}
		if err := nilArbiter.Close(); !errors.Is(err, ErrNilReceiver) {
			t.Fatalf("expected ErrNilReceiver on Close, got %v", err)
		}
	})
}

func verifyInvalidInputs(t *testing.T) {
	t.Run("InvalidInputs", func(t *testing.T) {
		arbiter := NewConsensusArbiter()

		if _, err := arbiter.AcquireLease("", "worker-1", time.Second); !errors.Is(err, ErrInvalidTaskID) {
			t.Fatalf("expected ErrInvalidTaskID, got %v", err)
		}
		if _, err := arbiter.AcquireLease("task-1", "", time.Second); !errors.Is(err, ErrInvalidWorkerID) {
			t.Fatalf("expected ErrInvalidWorkerID, got %v", err)
		}
		if _, err := arbiter.AcquireLease("task-1", "worker-1", 0); !errors.Is(err, ErrInvalidLeaseDuration) {
			t.Fatalf("expected ErrInvalidLeaseDuration, got %v", err)
		}
		if _, err := arbiter.AcquireLease("task-1", "worker-1", -time.Second); !errors.Is(err, ErrInvalidLeaseDuration) {
			t.Fatalf("expected ErrInvalidLeaseDuration for negative duration, got %v", err)
		}

		if _, err := arbiter.RenewLease("", "worker-1", time.Second); !errors.Is(err, ErrInvalidTaskID) {
			t.Fatalf("expected ErrInvalidTaskID on renew, got %v", err)
		}
		if _, err := arbiter.RenewLease("task-1", "", time.Second); !errors.Is(err, ErrInvalidWorkerID) {
			t.Fatalf("expected ErrInvalidWorkerID on renew, got %v", err)
		}
		if _, err := arbiter.RenewLease("task-1", "worker-1", 0); !errors.Is(err, ErrInvalidLeaseDuration) {
			t.Fatalf("expected ErrInvalidLeaseDuration on renew, got %v", err)
		}

		if err := arbiter.CommitTask("", "worker-1"); !errors.Is(err, ErrInvalidTaskID) {
			t.Fatalf("expected ErrInvalidTaskID on commit, got %v", err)
		}
		if err := arbiter.CommitTask("task-1", ""); !errors.Is(err, ErrInvalidWorkerID) {
			t.Fatalf("expected ErrInvalidWorkerID on commit, got %v", err)
		}
		if _, err := arbiter.LookupLease(""); !errors.Is(err, ErrInvalidTaskID) {
			t.Fatalf("expected ErrInvalidTaskID on LookupLease, got %v", err)
		}
		if _, err := arbiter.LookupLease("nonexistent"); !errors.Is(err, ErrLeaseNotFound) {
			t.Fatalf("expected ErrLeaseNotFound on LookupLease, got %v", err)
		}
	})
}

// TestConsensusArbiter_ConcurrentRaceResilience satisfies the 60-worker concurrency mandate.
func TestConsensusArbiter_ConcurrentRaceResilience(t *testing.T) {
	arbiter := NewConsensusArbiter(WithDefaultLeaseTTL(50 * time.Millisecond))
	concurrency := 60
	iterations := 25
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		workerID := fmt.Sprintf("race-worker-%d", i)
		wg.Add(1)

		goroutinelabels.NewGoroutine(
			fmt.Sprintf("test_consensus_worker_%d", i),
			"concurrent consensus arbiter stress worker",
		).StartSimple(func() {
			defer wg.Done()
			executeConcurrentWorkerLoop(t, arbiter, workerID, iterations)
		})
	}

	wg.Wait()

	totalTasks := len(arbiter.ListTasks())
	if totalTasks < concurrency {
		t.Fatalf("expected at least %d tasks tracked, got %d", concurrency, totalTasks)
	}
}

func executeWorkerAction(t *testing.T, arbiter *AmbientConsensusArbiter, taskID string, workerID string, actionMod int) {
	switch actionMod {
	case 0:
		if _, renewErr := arbiter.RenewLease(taskID, workerID, 50*time.Millisecond); renewErr != nil && !errors.Is(renewErr, ErrLeaseExpired) {
			t.Errorf("unexpected renew error: %v", renewErr)
		}
		if commitErr := arbiter.CommitTask(taskID, workerID); commitErr != nil && !errors.Is(commitErr, ErrLeaseExpired) {
			t.Errorf("unexpected commit error: %v", commitErr)
		}
	case 1:
		if commitErr := arbiter.CommitTask(taskID, workerID); commitErr != nil && !errors.Is(commitErr, ErrLeaseExpired) {
			t.Errorf("unexpected fast commit error: %v", commitErr)
		}
	default:
		if revokeErr := arbiter.RevokeTask(taskID, "preempted"); revokeErr != nil && !errors.Is(revokeErr, ErrTaskAlreadyCommitted) {
			t.Errorf("unexpected revoke error: %v", revokeErr)
		}
	}
}

func executeConcurrentWorkerLoop(t *testing.T, arbiter *AmbientConsensusArbiter, workerID string, iterations int) {
	for j := 0; j < iterations; j++ {
		taskID := fmt.Sprintf("task-%s-%d", workerID, j)
		if _, err := arbiter.ProposeTask(taskID, []byte("concurrent-payload")); err != nil && !errors.Is(err, ErrTaskAlreadyLeased) {
			t.Errorf("unexpected propose error: %v", err)
		}

		lease, err := arbiter.LeaseTask(taskID, workerID, 30*time.Millisecond)
		if err == nil && lease != nil {
			executeWorkerAction(t, arbiter, taskID, workerID, j%3)
		}

		p := arbiter.ProposedCount()
		l := arbiter.LeasedCount()
		c := arbiter.CommittedCount()
		r := arbiter.RevokedCount()
		if p < 0 || l < 0 || c < 0 || r < 0 {
			t.Errorf("invalid negative counts reported")
		}
	}
}

// TestConsensusArbiter_ASTAudit verifies zero naked goroutines, function line length limits,
// and quality audit compliance.
func TestConsensusArbiter_ASTAudit(t *testing.T) {
	auditor := qa.NewASTAuditor()
	targetFiles := []string{"consensus_arbiter.go", "consensus_arbiter_test.go"}

	for _, file := range targetFiles {
		violations, err := auditor.AuditFile(file)
		if err != nil {
			t.Fatalf("failed to audit %s: %v", file, err)
		}
		for _, v := range violations {
			if v.Severity == "high" || v.Severity == "medium" {
				t.Errorf("AST violation [%s] in %s at %s: %s", v.Severity, file, v.Pos, v.Message)
			}
		}

		verifyNoNakedGoroutinesInFile(t, file)
		verifyFunctionLineLengthsInFile(t, file)
	}
}

func parseSourceASTForArbiter(t *testing.T, filename string) (*token.FileSet, *ast.File) {
	t.Helper()
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, filename, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse %s: %v", filename, err)
	}
	return fset, node
}

func verifyNoNakedGoroutinesInFile(t *testing.T, filename string) {
	fset, node := parseSourceASTForArbiter(t, filename)
	ast.Inspect(node, func(n ast.Node) bool {
		goStmt, ok := n.(*ast.GoStmt)
		if !ok {
			return true
		}
		if _, isFuncLit := goStmt.Call.Fun.(*ast.FuncLit); isFuncLit {
			pos := fset.Position(goStmt.Pos())
			t.Errorf("POL-CONCURRENCY violation: raw naked goroutine found in %s at %s", filename, pos)
		}
		return true
	})
}

func verifyFunctionLineLengthsInFile(t *testing.T, filename string) {
	fset, node := parseSourceASTForArbiter(t, filename)
	ast.Inspect(node, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}
		startPos := fset.Position(fn.Body.Lbrace)
		endPos := fset.Position(fn.Body.Rbrace)
		lineCount := endPos.Line - startPos.Line + 1
		if lineCount > 100 {
			t.Errorf("Function %s in %s exceeds 100 lines limit (has %d lines)", fn.Name.Name, filename, lineCount)
		}
		return true
	})
}
