package agentclaim

import (
	"context"
	"strings"
	"sync"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

type claimMemStore struct {
	storage.ObjectStorageProvider
	mu   sync.Mutex
	objs map[string]map[string]any
}

func newClaimMemStore(tasks ...map[string]any) *claimMemStore {
	m := &claimMemStore{objs: map[string]map[string]any{}}
	for _, task := range tasks {
		id, _ := task[objects.FieldKeyID].(string)
		m.objs[id] = task
	}
	return m
}

func (m *claimMemStore) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objs[id]
	if !ok {
		return nil, errfmt.Errorf("not found")
	}
	cp := make(map[string]any, len(o))
	for k, v := range o {
		cp[k] = v
	}
	return cp, nil
}

func (m *claimMemStore) Update(ctx context.Context, sec *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.objs[id]
	if !ok {
		return errfmt.Errorf("not found")
	}
	for k, v := range updates {
		o[k] = v
	}
	return nil
}

func TestTryClaim_ContentionAndIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_ = objects.GetGlobalLifecycleLoader().EnsureReady(ctx)
	sec := pkgctx.NewSystemSecurityContext()
	store := newClaimMemStore(map[string]any{
		objects.FieldKeyID:   "ATK-1",
		objects.FieldKeyKind: objects.KindAgentTask,
	})

	res, err := TryClaim(ctx, store, sec, "ATK-1", "agent-a")
	if err != nil || !res.Claimed {
		t.Fatalf("first claim: %#v err=%v", res, err)
	}

	res2, err := TryClaim(ctx, store, sec, "ATK-1", "agent-a")
	if err != nil || !res2.Claimed || res2.Reason != "already_held" {
		t.Fatalf("idempotent: %#v err=%v", res2, err)
	}

	_, err = TryClaim(ctx, store, sec, "ATK-1", "agent-b")
	if err == nil {
		t.Fatal("expected contention error")
	}

	rel, err := Release(ctx, store, sec, "ATK-1", "agent-a", false)
	if err != nil || !rel.Released {
		t.Fatalf("release: %#v err=%v", rel, err)
	}

	res3, err := TryClaim(ctx, store, sec, "ATK-1", "agent-b")
	if err != nil || !res3.Claimed {
		t.Fatalf("reclaim after release: %#v err=%v", res3, err)
	}
}

func TestTryClaim_RefusesNonOccupiableKind(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_ = objects.GetGlobalLifecycleLoader().EnsureReady(ctx)
	sec := pkgctx.NewSystemSecurityContext()
	store := newClaimMemStore(map[string]any{
		objects.FieldKeyID:   "GOAL-1",
		objects.FieldKeyKind: objects.KindGoal,
	})

	res, err := TryClaim(ctx, store, sec, "GOAL-1", "agent-a")
	if err == nil || res.Reason != "not_occupiable" {
		t.Fatalf("expected not_occupiable error, got: %#v, err=%v", res, err)
	}

	rel, err := Release(ctx, store, sec, "GOAL-1", "agent-a", false)
	if err == nil || rel.Reason != "not_occupiable" {
		t.Fatalf("expected not_occupiable error on release, got: %#v, err=%v", rel, err)
	}
}

func TestEnforceTrunkTipFreshness_FailClosed(t *testing.T) {
	orig := gitOutput
	t.Cleanup(func() { gitOutput = orig })

	gitOutput = func(root string, args ...string) (string, error) {
		return "", errfmt.Errorf("git command failed injected error")
	}

	err := enforceTrunkTipFreshness(context.Background(), "/tmp")
	if err == nil {
		t.Fatal("expected fail-closed error when git fails")
	}
	if !strings.Contains(err.Error(), "fail-closed gate") {
		t.Fatalf("expected fail-closed gate message, got %v", err)
	}

	gitOutput = func(root string, args ...string) (string, error) {
		if args[0] == "merge-base" {
			return "old-commit", nil
		}
		if args[0] == "rev-parse" && args[1] == "main" {
			return "main-tip", nil
		}
		if args[0] == "rev-parse" && args[1] == "HEAD" {
			return "new-commit", nil
		}
		return "", nil
	}
	err = enforceTrunkTipFreshness(context.Background(), "/tmp")
	if err == nil {
		t.Fatal("expected error when branch is stale")
	}
	if !strings.Contains(err.Error(), "claim requires branching from current trunk tip (main)") {
		t.Fatalf("expected stale branch error, got %v", err)
	}
}

func TestTryClaim_RecordsBranchRef(t *testing.T) {
	orig := gitOutput
	t.Cleanup(func() { gitOutput = orig })

	gitOutput = func(root string, args ...string) (string, error) {
		if args[0] == "merge-base" {
			return "same-commit", nil
		}
		if args[0] == "rev-parse" && args[1] == "main" {
			return "same-commit", nil
		}
		if args[0] == "rev-parse" && len(args) == 2 && args[1] == "HEAD" {
			return "same-commit", nil
		}
		if args[0] == "rev-parse" && len(args) == 3 && args[1] == "--abbrev-ref" && args[2] == "HEAD" {
			return "feature/test-branch", nil
		}
		return "", nil
	}

	ctx := context.Background()
	_ = objects.GetGlobalLifecycleLoader().EnsureReady(ctx)
	sec := pkgctx.NewSystemSecurityContext()
	store := newClaimMemStore(map[string]any{
		objects.FieldKeyID:   "ATK-1",
		objects.FieldKeyKind: objects.KindAgentTask,
	})

	res, err := TryClaim(ctx, store, sec, "ATK-1", "agent-a", ClaimOptions{ProjectRoot: "/tmp"})
	if err != nil || !res.Claimed {
		t.Fatalf("claim failed: %v", err)
	}

	task, _ := store.Read(ctx, sec, "ATK-1")
	if task[objects.FieldKeyBranchName] != "feature/test-branch" {
		t.Fatalf("expected branch_name 'feature/test-branch', got %v", task[objects.FieldKeyBranchName])
	}
}

func TestTryClaim_RefusesConceptual(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_ = objects.GetGlobalLifecycleLoader().EnsureReady(ctx)
	sec := pkgctx.NewSystemSecurityContext()
	store := newClaimMemStore(map[string]any{
		objects.FieldKeyID:     "ATK-1",
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: "conceptual",
	})
	res, err := TryClaim(ctx, store, sec, "ATK-1", "agent-a")
	if err == nil || res.Reason != "not_dispatched" {
		t.Fatalf("proposed claim: %#v err=%v", res, err)
	}
}

func TestTryClaim_ApprovedHopsInProgress(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_ = objects.GetGlobalLifecycleLoader().EnsureReady(ctx)
	sec := pkgctx.NewSystemSecurityContext()
	store := newClaimMemStore(map[string]any{
		objects.FieldKeyID:     "ATK-1",
		objects.FieldKeyKind:   objects.KindAgentTask,
		objects.FieldKeyStatus: objects.ObjectStatusApproved,
	})
	res, err := TryClaim(ctx, store, sec, "ATK-1", "agent-a")
	if err != nil || !res.Claimed {
		t.Fatalf("approved claim: %#v err=%v", res, err)
	}
	task, _ := store.Read(ctx, sec, "ATK-1")
	if task[objects.FieldKeyStatus] != objects.ObjectStatusInProgress {
		t.Fatalf("claim must fire approved→in_progress, got %v", task[objects.FieldKeyStatus])
	}
}

func TestTryClaim_RefusesWhenParentBacklogComplete(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	_ = objects.GetGlobalLifecycleLoader().EnsureReady(ctx)
	sec := pkgctx.NewSystemSecurityContext()
	store := newClaimMemStore(
		map[string]any{
			objects.FieldKeyID:             "ATK-1",
			objects.FieldKeyKind:           objects.KindAgentTask,
			objects.FieldKeyStatus:         objects.ObjectStatusApproved,
			objects.FieldKeyBacklogItemRef: "BLI-1",
		},
		map[string]any{
			objects.FieldKeyID:     "BLI-1",
			objects.FieldKeyKind:   objects.KindBacklogItem,
			objects.FieldKeyStatus: objects.ObjectStatusComplete,
		},
	)
	res, err := TryClaim(ctx, store, sec, "ATK-1", "agent-a")
	if err == nil || res.Reason != "parent_terminal" {
		t.Fatalf("parent complete: %#v err=%v", res, err)
	}
}
