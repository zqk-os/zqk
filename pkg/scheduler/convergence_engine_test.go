package scheduler

import (
	"context"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
)

func TestConvergenceEngine_StartStop(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	st := env.Storage.(storagepkg.ObjectStorageProvider)
	mustCreateActiveConvergenceSession(t, st, ctx, secCtx, "CVS-123")

	sched := NewSchedulerWithProjectRoot(st, env.SpecLoader, env.LifecycleLoader, env.TestRoot, nil)

	// Start engine and immediately stop it via context cancellation
	engineCtx, cancel := context.WithCancel(ctx)

	stopEngine := sched.StartConvergenceEngine(engineCtx)
	if stopEngine == nil {
		t.Fatal("expected stopEngine to not be nil")
	}

	// Wait a tiny bit then cancel
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Wait for goroutine to stop
	stopEngine(context.Background())
}

// mustCreateActiveConvergenceSession creates a CVS at origin (draft) then promotes to active.
// Create with status=active is rewritten to draft (origin); draft is preliminary and List-invisible
// until promoted into CAS.
func mustCreateActiveConvergenceSession(t *testing.T, st storagepkg.ObjectStorageProvider, ctx context.Context, secCtx *pkgctx.SecurityContext, id string) {
	t.Helper()
	err := st.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:               id,
		objects.FieldKeyKind:             objects.KindConvergenceSession,
		objects.FieldKeyStatus:           objects.ObjectStatusDraft,
		objects.FieldKeyTitle:            "test session " + id,
		objects.FieldKeyOutcomeCharacter: "pending",
		objects.FieldKeyCurrentPhase:     "c1_scope",
		objects.FieldKeyHypothesis:       "engine fixture hypothesis",
		objects.FieldKeyDesiredEndState:  "engine fixture end state",
	})
	if err != nil {
		t.Fatalf("create %s: %v", id, err)
	}
	updCtx := pkgctx.WithLifecycleBreakGlass(ctx, "convergence_engine_test fixture promote")
	if err := st.Update(updCtx, secCtx, id, map[string]any{objects.FieldKeyStatus: objects.ObjectStatusActive}); err != nil {
		t.Fatalf("promote %s to active: %v", id, err)
	}
	got, err := st.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	if status, _ := got[objects.FieldKeyStatus].(string); status != "active" {
		t.Fatalf("%s status=%q want active", id, status)
	}
}

func TestConvergenceEngine_evaluateActiveConvergenceSessions(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	st := env.Storage.(storagepkg.ObjectStorageProvider)

	mustCreateActiveConvergenceSession(t, st, ctx, secCtx, "CVS-ACTIVE")

	// Create inactive session (stays draft / List-invisible — evaluate must ignore it)
	err := st.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:     "CVS-INACTIVE",
		objects.FieldKeyKind:   objects.KindConvergenceSession,
		objects.FieldKeyStatus: objects.ObjectStatusDraft,
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	sched := NewSchedulerWithProjectRoot(st, env.SpecLoader, env.LifecycleLoader, env.TestRoot, nil)

	// Confirm the active session is listable (draft-plane / lifecycle traps would skip evaluate).
	listed, listErr := st.List(ctx, secCtx, nil, storagepkg.ListFilter{Kind: objects.KindConvergenceSession})
	if listErr != nil {
		t.Fatalf("list convergence_session: %v", listErr)
	}
	var sawActive bool
	for _, obj := range listed.Objects {
		if id, _ := obj[objects.FieldKeyID].(string); id == "CVS-ACTIVE" {
			status, _ := obj[objects.FieldKeyStatus].(string)
			if status == "active" {
				sawActive = true
			} else {
				t.Fatalf("CVS-ACTIVE status=%q want active (evaluate would skip)", status)
			}
		}
	}
	if !sawActive {
		t.Fatalf("CVS-ACTIVE not returned by List (got %d session(s)); evaluate would no-op", len(listed.Objects))
	}

	// Inject MockExecutor to prove entirely in-memory, deterministic evaluation!
	executed := false
	sched.(*Scheduler).executor = &MockExecutor{
		CommandContextFn: func(_ context.Context, name string, args ...string) Cmd {
			full := name
			for _, a := range args {
				full += " " + a
			}
			if name == "bash" && len(args) >= 3 && args[0] == "scripts/cvs_convergence_orchestrate.sh" && args[1] == "CVS-ACTIVE" {
				executed = true
				return &MockCmd{OutputBytes: []byte(`{"rollup_status": "blocked"}`)}
			}
			t.Fatalf("unexpected command: %s", full)
			return &MockCmd{}
		},
	}

	// Evaluate. The MockExecutor will intercept the bash call.
	sched.EvaluateActiveConvergenceSessions(ctx)

	if !executed {
		t.Fatal("expected MockExecutor to intercept the convergence orchestrate call")
	}

	// Verify debouncing: the session status should be updated to 'paused' since it returned 'blocked'
	// Use polling to account for write-behind worker processing the update asynchronously
	var updatedSession map[string]any
	var status string
	for i := 0; i < 50; i++ {
		updatedSession, err = st.Read(ctx, secCtx, "CVS-ACTIVE")
		if err != nil {
			t.Fatalf("failed to get updated session: %v", err)
		}
		status, _ = updatedSession[objects.FieldKeyStatus].(string)
		if status == "paused" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Logf("Updated session: %+v", updatedSession)
	if status != "paused" {
		t.Errorf("expected session status to be 'paused', got %q", status)
	}
}

type mockStaleStorage struct {
	storagepkg.ObjectStorageProvider
	staleTime string
	freshTime string
}

func (m *mockStaleStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storagepkg.StorageContext, filter storagepkg.ListFilter) (*storagepkg.QueryResult, error) {
	result, err := m.ObjectStorageProvider.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return result, err
	}

	// Override timestamps for testing
	for _, obj := range result.Objects {
		id, _ := obj[objects.FieldKeyID].(string)
		switch id {
		case "CVS-STALE":
			obj[objects.FieldKeyUpdatedAt] = m.staleTime
		case "CVS-FRESH":
			obj[objects.FieldKeyUpdatedAt] = m.freshTime
		}
	}
	return result, nil
}

func TestConvergenceEngine_evaluateStaleConvergenceSessions(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	baseSt := env.Storage.(storagepkg.ObjectStorageProvider)

	now := time.Now()
	staleTime := now.Add(-2 * time.Hour).Format(time.RFC3339)
	freshTime := now.Add(-30 * time.Minute).Format(time.RFC3339)

	st := &mockStaleStorage{
		ObjectStorageProvider: baseSt,
		staleTime:             staleTime,
		freshTime:             freshTime,
	}

	mustCreateActiveConvergenceSession(t, st, ctx, secCtx, "CVS-STALE")
	mustCreateActiveConvergenceSession(t, st, ctx, secCtx, "CVS-FRESH")
	var err error

	sched := NewSchedulerWithProjectRoot(st, env.SpecLoader, env.LifecycleLoader, env.TestRoot, nil)

	// Evaluate
	sched.EvaluateStaleConvergenceSessions(ctx)

	// Verify CVS-STALE is escalated
	var staleSession map[string]any
	var status string
	for i := 0; i < 50; i++ {
		staleSession, err = st.Read(ctx, secCtx, "CVS-STALE")
		if err != nil {
			t.Fatalf("failed to read stale session: %v", err)
		}
		status, _ = staleSession[objects.FieldKeyStatus].(string)
		if status == "escalated" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if status != "escalated" {
		t.Errorf("expected stale session status to be 'escalated', got %q", status)
	}

	// Verify CVS-FRESH is untouched (status active)
	freshSession, err := st.Read(ctx, secCtx, "CVS-FRESH")
	if err != nil {
		t.Fatalf("failed to read fresh session: %v", err)
	}
	if freshStatus, _ := freshSession[objects.FieldKeyStatus].(string); freshStatus != "active" {
		t.Errorf("expected fresh session status to be 'active', got %q", freshStatus)
	}

	// Verify priority_plan was created
	plans, err := st.List(ctx, secCtx, nil, storagepkg.ListFilter{
		Kind: objects.KindPriorityPlan,
	})
	if err != nil {
		t.Fatalf("failed to list priority plans: %v", err)
	}

	foundPlan := false
	for _, p := range plans.Objects {
		if title, _ := p[objects.FieldKeyTitle].(string); title == "Drift-Control: Session CVS-STALE Timeout" {
			foundPlan = true
			if cat, _ := p[objects.FieldKeyCategory].(string); cat != "Re-Alignment" {
				t.Errorf("expected category 'Re-Alignment', got %q", cat)
			}
			if s, _ := p[objects.FieldKeyStatus].(string); s != "active" {
				t.Errorf("expected status 'active', got %q", s)
			}
		}
	}
	if !foundPlan {
		t.Errorf("expected priority_plan to be created for stale session")
	}
}
