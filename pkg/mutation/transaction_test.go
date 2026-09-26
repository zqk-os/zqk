package mutation_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/zqk-os/zqk/pkg/mutation"
)

// Satisfies CRIT-ZQL-TXN-ISOLATION-SPEC:
// Formal specification codifying execution state machine transitions (BEGIN, STAGE, PRE_CHECK, COMMIT, ROLLBACK)
// and isolation levels (all_or_nothing, partial_commit, dry_run).
func TestZQL_TransactionIsolationSpec(t *testing.T) {
	ctx := context.Background()

	t.Run("state machine transitions: INITIAL -> BEGIN -> STAGE -> PRE_CHECK -> COMMIT", func(t *testing.T) {
		txn := mutation.NewTransaction("TXN-001", mutation.IsolationAllOrNothing)
		if txn.State() != mutation.StateInitial {
			t.Fatalf("expected state INITIAL, got: %s", txn.State())
		}

		if err := txn.Begin(); err != nil {
			t.Fatalf("failed to begin: %v", err)
		}
		if txn.State() != mutation.StateBegin {
			t.Fatalf("expected state BEGIN, got: %s", txn.State())
		}

		mut := mutation.Mutation{
			Action:     mutation.ActionCreateNode,
			TargetKind: "backlog_item",
			TargetID:   "BLI-TXN-001",
			Fields:     map[string]any{"title": "Test BLI"},
		}
		if err := txn.Stage(mut); err != nil {
			t.Fatalf("failed to stage: %v", err)
		}
		if txn.State() != mutation.StateStage {
			t.Fatalf("expected state STAGE, got: %s", txn.State())
		}

		if err := txn.PreCheck(ctx, nil); err != nil {
			t.Fatalf("pre-check failed: %v", err)
		}
		if txn.State() != mutation.StatePreCheck {
			t.Fatalf("expected state PRE_CHECK, got: %s", txn.State())
		}

		err := txn.Commit(ctx, func(ctx context.Context, idx int, mut *mutation.Mutation) (func(context.Context) error, error) {
			return nil, nil
		})
		if err != nil {
			t.Fatalf("commit failed: %v", err)
		}
		if txn.State() != mutation.StateCommit {
			t.Fatalf("expected state COMMIT, got: %s", txn.State())
		}
	})

	t.Run("state machine transitions: BEGIN -> STAGE -> ROLLBACK", func(t *testing.T) {
		txn := mutation.NewTransaction("TXN-002", mutation.IsolationAllOrNothing)
		_ = txn.Begin()
		_ = txn.Stage(mutation.Mutation{
			Action:     mutation.ActionCreateNode,
			TargetKind: "goal",
			TargetID:   "GOAL-001",
		})

		if err := txn.Rollback(ctx); err != nil {
			t.Fatalf("rollback failed: %v", err)
		}
		if txn.State() != mutation.StateRollback {
			t.Fatalf("expected state ROLLBACK, got: %s", txn.State())
		}
	})

	t.Run("isolation mode: dry_run leaves storage unmutated", func(t *testing.T) {
		engine := mutation.NewTransactionEngine()
		txn := mutation.NewTransaction("TXN-003", mutation.IsolationDryRun)
		_ = txn.Begin()
		_ = txn.Stage(mutation.Mutation{
			Action:     mutation.ActionCreateNode,
			TargetKind: "criteria",
			TargetID:   "CRIT-DRY-RUN",
		})

		if err := engine.ExecuteTransaction(ctx, txn); err != nil {
			t.Fatalf("dry run execution failed: %v", err)
		}

		if engine.Count(ctx) != 0 {
			t.Fatalf("expected storage count 0 in dry_run mode, got: %d", engine.Count(ctx))
		}
		receipts := txn.Receipts()
		if len(receipts) != 1 || receipts[0].Status != mutation.ReceiptStatusDryRunValidated {
			t.Fatalf("expected dry_run_validated receipt status, got: %v", receipts)
		}
	})

	t.Run("isolation mode: partial_commit quarantines failures and commits successes", func(t *testing.T) {
		engine := mutation.NewTransactionEngine()
		txn := mutation.NewTransaction("TXN-004", mutation.IsolationPartialCommit)
		_ = txn.Begin()
		_ = txn.Stage(mutation.Mutation{Action: mutation.ActionCreateNode, TargetKind: "backlog_item", TargetID: "BLI-OK-1"})
		// Trigger error by setting empty target_kind for pre-check
		_ = txn.Stage(mutation.Mutation{Action: mutation.ActionCreateNode, TargetKind: "", TargetID: "BLI-FAIL"})
		_ = txn.Stage(mutation.Mutation{Action: mutation.ActionCreateNode, TargetKind: "backlog_item", TargetID: "BLI-OK-2"})

		if err := engine.ExecuteTransaction(ctx, txn); err != nil {
			t.Fatalf("partial commit execution failed: %v", err)
		}

		receipts := txn.Receipts()
		if len(receipts) != 3 {
			t.Fatalf("expected 3 receipts, got: %d", len(receipts))
		}
		if receipts[1].Status != mutation.ReceiptStatusQuarantined {
			t.Fatalf("expected item 1 quarantined, got: %s", receipts[1].Status)
		}
		if engine.Count(ctx) != 2 {
			t.Fatalf("expected 2 successful writes in storage, got: %d", engine.Count(ctx))
		}
	})
}

// Satisfies CRIT-ZQL-ALL-OR-NOTHING-ROLLBACK-PROOF:
// Execution engine verifies that when an error occurs on record N during all_or_nothing
// transaction processing, all previously staged operations in the transaction batch
// are completely rolled back leaving zero uncommitted mutations.
func TestZQL_AllOrNothingRollbackProof(t *testing.T) {
	ctx := context.Background()
	engine := mutation.NewTransactionEngine()

	// Seed existing node RECORD-004 so attempting to create it again fails on record 4
	seedTxn := mutation.NewTransaction("TXN-SEED", mutation.IsolationAllOrNothing)
	_ = seedTxn.Begin()
	_ = seedTxn.Stage(mutation.Mutation{
		Action:     mutation.ActionCreateNode,
		TargetKind: "backlog_item",
		TargetID:   "RECORD-004",
		Fields:     map[string]any{"seeded": true},
	})
	if err := engine.ExecuteTransaction(ctx, seedTxn); err != nil {
		t.Fatalf("seed transaction failed: %v", err)
	}
	if engine.Count(ctx) != 1 {
		t.Fatalf("expected 1 seeded record, got %d", engine.Count(ctx))
	}

	// Now run all_or_nothing transaction attempting to create RECORD-001..RECORD-005
	// Record 4 will trigger duplicate node error and roll back records 1..3
	txn := mutation.NewTransaction("TXN-ALL-OR-NOTHING", mutation.IsolationAllOrNothing)
	_ = txn.Begin()
	for i := 1; i <= 5; i++ {
		_ = txn.Stage(mutation.Mutation{
			Action:     mutation.ActionCreateNode,
			TargetKind: "backlog_item",
			TargetID:   fmt.Sprintf("RECORD-%03d", i),
			Fields:     map[string]any{"index": i},
		})
	}

	err := engine.ExecuteTransaction(ctx, txn)
	if err == nil {
		t.Fatal("expected ExecuteTransaction to fail on duplicate RECORD-004")
	}

	// Verify rollback state
	if txn.State() != mutation.StateRollback {
		t.Fatalf("expected transaction state to be ROLLBACK, got: %s", txn.State())
	}

	// PROOF: Underlying storage must have ONLY the 1 original seeded record (RECORD-001..003 were rolled back cleanly)
	if engine.Count(ctx) != 1 {
		t.Fatalf("ALL-OR-NOTHING INVARIANT VIOLATION: expected 1 record in storage after rollback, found %d", engine.Count(ctx))
	}

	// Confirm RECORD-001 does not exist in storage
	if _, exists := engine.Get(ctx, "RECORD-001"); exists {
		t.Fatal("RECORD-001 leaked into storage despite rollback!")
	}
}

// Satisfies CRIT-ZQL-DIRTY-READ-CONCURRENCY-NEGATIVE:
// Transaction engine enforces write isolation, verifying that concurrent read operations
// cannot observe uncommitted staging modifications during an in-flight transaction
// until formal commit finalization.
func TestZQL_DirtyReadConcurrencyNegative(t *testing.T) {
	ctx := context.Background()
	engine := mutation.NewTransactionEngine()

	txn := mutation.NewTransaction("TXN-ISOLATED", mutation.IsolationAllOrNothing)
	_ = txn.Begin()

	// Stage an in-flight mutation
	uncommittedID := "BLI-UNCOMMITTED-SECRET"
	_ = txn.Stage(mutation.Mutation{
		Action:     mutation.ActionCreateNode,
		TargetKind: "backlog_item",
		TargetID:   uncommittedID,
		Fields:     map[string]any{"secret": "confidential_data"},
	})

	// 1. Transaction private staged buffer contains the mutation
	staged := txn.StagedMutations()
	if len(staged) != 1 || staged[0].TargetID != uncommittedID {
		t.Fatal("expected staged mutation in transaction buffer")
	}

	// 2. CONCURRENT READ ISOLATION: Engine store must NOT observe uncommitted mutation
	_, exists := engine.Get(ctx, uncommittedID)
	if exists {
		t.Fatal("DIRTY READ VULNERABILITY: outer read observed uncommitted staged mutation prior to commit!")
	}

	// 3. Finalize commit via engine
	if err := engine.ExecuteTransaction(ctx, txn); err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	// 4. Post-commit: Outer read now cleanly observes committed object
	committedObj, postExists := engine.Get(ctx, uncommittedID)
	if !postExists {
		t.Fatal("expected committed object to be readable post-commit")
	}
	if committedObj["secret"] != "confidential_data" {
		t.Errorf("expected committed data to match, got: %v", committedObj["secret"])
	}
}
