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

	// Create an active session
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	activeSession := map[string]any{
		objects.FieldKeyID:     "CONV-123",
		objects.FieldKeyKind:   "convergence_session",
		objects.FieldKeyStatus: "active",
	}
	st := env.Storage.(storagepkg.ObjectStorageProvider)
	err := st.Create(ctx, secCtx, activeSession)
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

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

func TestConvergenceEngine_evaluateActiveConvergenceSessions(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	st := env.Storage.(storagepkg.ObjectStorageProvider)

	// Create active session
	err := st.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:     "CONV-ACTIVE",
		objects.FieldKeyKind:   "convergence_session",
		objects.FieldKeyStatus: "active",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Create inactive session
	err = st.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:     "CONV-INACTIVE",
		objects.FieldKeyKind:   "convergence_session",
		objects.FieldKeyStatus: "draft",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	sched := NewSchedulerWithProjectRoot(st, env.SpecLoader, env.LifecycleLoader, env.TestRoot, nil)

	// Inject MockExecutor to prove entirely in-memory, deterministic evaluation!
	executed := false
	mockExecutor := NewMemoryExecutor(map[string]*MockCmd{
		"bash scripts/cvs_convergence_orchestrate.sh CONV-ACTIVE --no-fail-on-gates": {
			OutputBytes: []byte(`{"rollup_status": "blocked"}`),
			RunFn: func() error {
				executed = true
				return nil
			},
		},
	})
	sched.(*Scheduler).executor = mockExecutor

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
		updatedSession, err = st.Read(ctx, secCtx, "CONV-ACTIVE")
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
		case "CONV-STALE":
			obj[objects.FieldKeyUpdatedAt] = m.staleTime
		case "CONV-FRESH":
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

	// Create stale session
	err := st.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:     "CONV-STALE",
		objects.FieldKeyKind:   "convergence_session",
		objects.FieldKeyStatus: "active",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	// Create fresh session
	err = st.Create(ctx, secCtx, map[string]any{
		objects.FieldKeyID:     "CONV-FRESH",
		objects.FieldKeyKind:   "convergence_session",
		objects.FieldKeyStatus: "active",
	})
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	sched := NewSchedulerWithProjectRoot(st, env.SpecLoader, env.LifecycleLoader, env.TestRoot, nil)

	// Evaluate
	sched.EvaluateStaleConvergenceSessions(ctx)

	// Verify CONV-STALE is escalated
	var staleSession map[string]any
	var status string
	for i := 0; i < 50; i++ {
		staleSession, err = st.Read(ctx, secCtx, "CONV-STALE")
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

	// Verify CONV-FRESH is untouched (status active)
	freshSession, err := st.Read(ctx, secCtx, "CONV-FRESH")
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
		if title, _ := p[objects.FieldKeyTitle].(string); title == "Drift-Control: Session CONV-STALE Timeout" {
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
