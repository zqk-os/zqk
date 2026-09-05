package kernelcas

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestAllKinds_closedSet(t *testing.T) {
	kinds := AllKinds()
	if len(kinds) != 7 {
		t.Fatalf("expected 7 pipeline kinds, got %d", len(kinds))
	}
	seen := map[string]bool{}
	for _, k := range kinds {
		if seen[k] {
			t.Fatalf("duplicate kind %s", k)
		}
		seen[k] = true
		if len(k) < 7 || k[:7] != "kernel." {
			t.Fatalf("kind %q must start with kernel.", k)
		}
	}
}

func TestRunReconcileIndex_neverRefusesWithoutCommit(t *testing.T) {
	called := false
	err := RunReconcileIndex(context.Background(), nil, &Mutation{
		Kind:   "criteria",
		ID:     "CRIT-TEST",
		Intent: IntentReconcileIndex,
		CommitFn: func(ctx context.Context) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if !called {
		t.Fatal("expected COMMIT to run for reindex_only")
	}
}

func TestRunErase_refusesCriticalWithoutReason(t *testing.T) {
	t.Setenv(zqkenv.TestRoot(), "")
	called := false
	err := RunErase(context.Background(), nil, &Mutation{
		Kind:   "criteria",
		ID:     "CRIT-TEST",
		Intent: IntentEraseLogical,
		CommitFn: func(ctx context.Context) error {
			called = true
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected refuse for critical kind without reason")
	}
	if called {
		t.Fatal("COMMIT must not run when DECIDE refuses")
	}
}

func TestRunErase_allowsNonCritical(t *testing.T) {
	called := false
	err := RunErase(context.Background(), nil, &Mutation{
		Kind:   "audit_event",
		ID:     "AUD-TEST",
		Intent: IntentEraseLogical,
		CommitFn: func(ctx context.Context) error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("erase non-critical: %v", err)
	}
	if !called {
		t.Fatal("expected COMMIT")
	}
}

func TestIsCriticalKind_includesBacklogAndTestCase(t *testing.T) {
	if !IsCriticalKind("backlog_item") || !IsCriticalKind("test_case") || !IsCriticalKind("convergence_session") {
		t.Fatal("expected backlog_item, test_case, convergence_session to be critical")
	}
	if IsCriticalKind("audit_event") {
		t.Fatal("stream kind audit_event must not be critical")
	}
	if IsCriticalKind("scheduler_job") {
		t.Fatal("ephemeral CAS scheduler_job must opt out of kernel_critical")
	}
}
