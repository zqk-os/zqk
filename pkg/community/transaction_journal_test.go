package community

import (
	"context"
	"errors"
	"testing"
)

func TestTransactionJournal_CommitSuccess(t *testing.T) {
	coord := NewTransactionCoordinator(10)
	tx := coord.Begin("provider-storage")

	var executionOrder []string
	tx.AddStep("step-1", "create object", func(ctx context.Context) error {
		executionOrder = append(executionOrder, "step-1")
		return nil
	}, func(ctx context.Context) error {
		executionOrder = append(executionOrder, "step-1-compensate")
		return nil
	})

	tx.AddStep("step-2", "update index", func(ctx context.Context) error {
		executionOrder = append(executionOrder, "step-2")
		return nil
	}, func(ctx context.Context) error {
		executionOrder = append(executionOrder, "step-2-compensate")
		return nil
	})

	err := tx.ExecuteAndCommit(context.Background())
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if tx.Phase() != PhaseCommitted {
		t.Fatalf("expected phase committed, got %v", tx.Phase())
	}

	if len(executionOrder) != 2 || executionOrder[0] != "step-1" || executionOrder[1] != "step-2" {
		t.Fatalf("unexpected execution order: %v", executionOrder)
	}

	history := coord.History()
	if len(history) != 1 || history[0].Phase != PhaseCommitted {
		t.Fatalf("expected 1 committed history record, got %+v", history)
	}
}

func TestTransactionJournal_AutoRollbackOnFailure(t *testing.T) {
	coord := NewTransactionCoordinator(10)
	tx := coord.Begin("provider-sync")

	var compensationOrder []string
	tx.AddStep("step-1", "stage file", func(ctx context.Context) error {
		return nil
	}, func(ctx context.Context) error {
		compensationOrder = append(compensationOrder, "step-1-rollback")
		return nil
	})

	tx.AddStep("step-2", "allocate resource", func(ctx context.Context) error {
		return errors.New("insufficient quota")
	}, func(ctx context.Context) error {
		compensationOrder = append(compensationOrder, "step-2-rollback")
		return nil
	})

	err := tx.ExecuteAndCommit(context.Background())
	if err == nil {
		t.Fatal("expected execution error, got nil")
	}

	if tx.Phase() != PhaseRolledBack {
		t.Fatalf("expected phase rolled_back, got %v", tx.Phase())
	}

	// Step 1 was executed, step 2 failed. Only step 1's compensation should be invoked.
	if len(compensationOrder) != 1 || compensationOrder[0] != "step-1-rollback" {
		t.Fatalf("expected ['step-1-rollback'], got %v", compensationOrder)
	}

	history := coord.History()
	if len(history) != 1 || history[0].Phase != PhaseRolledBack {
		t.Fatalf("expected 1 rolled_back history record, got %+v", history)
	}
}

func TestTransactionJournal_ManualRollback(t *testing.T) {
	coord := NewTransactionCoordinator(10)
	tx := coord.Begin("provider-network")

	var compensated bool
	tx.AddStep("step-1", "open socket", func(ctx context.Context) error {
		return nil
	}, func(ctx context.Context) error {
		compensated = true
		return nil
	})

	// Add step and execute manually without commit
	if err := tx.steps[0].Execute(context.Background()); err != nil {
		t.Fatalf("exec error: %v", err)
	}
	tx.executedIdx = 1

	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback error: %v", err)
	}

	if !compensated {
		t.Fatal("expected compensation to be invoked")
	}
	if tx.Phase() != PhaseRolledBack {
		t.Fatalf("expected PhaseRolledBack, got %v", tx.Phase())
	}
}
