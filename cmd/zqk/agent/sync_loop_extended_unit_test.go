package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func initGitRepoInDir(t *testing.T, dir string) string {
	t.Helper()
	cmd := execwrap.Command("git", "init")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = execwrap.Command("git", "config", "user.name", "Test Agent")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = execwrap.Command("git", "config", "user.email", "agent@test.local")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	cmd = execwrap.Command("git", "commit", "--allow-empty", "-m", "initial commit")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())

	out, err := gitWorktreeOutput(context.Background(), dir, "rev-parse", "HEAD")
	require.NoError(t, err)
	return out
}

func TestSyncLoop_InvalidInputs(t *testing.T) {
	_, store := setupSyncLoopTestProject(t)
	_ = store

	// 1. Missing task ID argument
	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{})
	err := cmd.ExecuteContext(context.Background())
	assert.Error(t, err)

	// 2. Non-existent task ID
	cmd = NewSyncLoopCmd()
	cmd.SetArgs([]string{"ATK-NONEXISTENT-999"})
	err = cmd.ExecuteContext(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read task")

	// 3. Non-agent-task kind (e.g. persona)
	cmd = NewSyncLoopCmd()
	cmd.SetArgs([]string{objects.ConstPersonaOrchestratorAlpha})
	err = cmd.ExecuteContext(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "can only execute agent_task objects")
}

func TestSyncLoop_AlreadyImplemented(t *testing.T) {
	root, store := setupSyncLoopTestProject(t)
	commitHash := initGitRepoInDir(t, root)

	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-ALREADY-DONE"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Already Completed Task",
		objects.FieldKeyStatus:             objects.ObjectStatusImplemented,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyCommitHash:         commitHash,
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusImplemented)

	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskID})
	errChan := make(chan error, 1)
	goroutinelabels.NewGoroutine("sync_loop_test", "runSyncLoop already done").StartSimple(func() {
		errChan <- cmd.ExecuteContext(context.Background())
	})

	select {
	case loopErr := <-errChan:
		require.NoError(t, loopErr)
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for runSyncLoop on completed task")
	}

	writeQueue := caspkg.GetListingIndexWriteQueueForProjectRoot(root)
	if writeQueue != nil {
		_ = writeQueue.FlushAll(1 * time.Second)
	}
	time.Sleep(50 * time.Millisecond)
}

func TestSyncLoop_NoStepsTransition(t *testing.T) {
	_, store := setupSyncLoopTestProject(t)

	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-NO-STEPS-TASK"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "No Steps Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyTaskSteps:          []any{},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusInProgress)

	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskID})
	errChan := make(chan error, 1)
	goroutinelabels.NewGoroutine("sync_loop_test", "runSyncLoop no steps").StartSimple(func() {
		errChan <- cmd.ExecuteContext(context.Background())
	})

	select {
	case loopErr := <-errChan:
		require.NoError(t, loopErr)
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for runSyncLoop to complete empty steps task")
	}

	bypassCtx := pkgctx.WithBypassCache(context.Background())
	updated, err := store.Read(bypassCtx, secCtx, taskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusImplemented, updated[objects.FieldKeyStatus])
}

func TestSyncLoop_StagnationAbort(t *testing.T) {
	t.Setenv(zqkenv.AgentSyncMaxStagnantTicks().Name(), "1")

	_, store := setupSyncLoopTestProject(t)

	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-STAGNANT-TASK"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Stagnant Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyTitle:       "Step Pending",
				objects.FieldKeyStatus:      objects.ObjectStatusPending,
				objects.FieldKeyDescription: "Must implement something",
			},
		},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusInProgress)

	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskID})
	errChan := make(chan error, 1)
	goroutinelabels.NewGoroutine("sync_loop_test", "runSyncLoop stagnant").StartSimple(func() {
		errChan <- cmd.ExecuteContext(context.Background())
	})

	select {
	case loopErr := <-errChan:
		require.NoError(t, loopErr)
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for runSyncLoop to finish")
	}

	bypassCtx := pkgctx.WithBypassCache(context.Background())
	updated, err := store.Read(bypassCtx, secCtx, taskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusFailed, updated[objects.FieldKeyStatus])
}

func TestSyncLoop_StepVerification_Passed(t *testing.T) {
	root, store := setupSyncLoopTestProject(t)
	commitHash := initGitRepoInDir(t, root)

	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-VERIFY-PASS-TASK"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Verification Pass Task",
		objects.FieldKeyStatus:             objects.ObjectStatusPendingVerification,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyCommitHash:         commitHash,
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyName:        "Step1",
				objects.FieldKeyTitle:       "Step 1",
				objects.FieldKeyStatus:      objects.ObjectStatusPendingVerification,
				"verification_strategy":     "command_exit_code",
				objects.FieldKeyCommand:     "echo ok",
				objects.FieldKeyDescription: "Exit code verification",
			},
		},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusPendingVerification)

	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskID})
	errChan := make(chan error, 1)
	goroutinelabels.NewGoroutine("sync_loop_test", "runSyncLoop verify pass").StartSimple(func() {
		errChan <- cmd.ExecuteContext(context.Background())
	})

	select {
	case loopErr := <-errChan:
		require.NoError(t, loopErr)
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for runSyncLoop to verify and implement task")
	}

	bypassCtx := pkgctx.WithBypassCache(context.Background())
	updated, err := store.Read(bypassCtx, secCtx, taskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusImplemented, updated[objects.FieldKeyStatus])

	writeQueue := caspkg.GetListingIndexWriteQueueForProjectRoot(root)
	if writeQueue != nil {
		_ = writeQueue.FlushAll(1 * time.Second)
	}
	time.Sleep(50 * time.Millisecond)
}

func TestSyncLoop_StepVerification_FailedAndMaxAttempts(t *testing.T) {
	t.Setenv(zqkenv.AgentMaxVerificationAttempts().Name(), "1")

	root, store := setupSyncLoopTestProject(t)
	commitHash := initGitRepoInDir(t, root)

	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-VERIFY-FAIL-TASK"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Verification Fail Task",
		objects.FieldKeyStatus:             objects.ObjectStatusPendingVerification,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyCommitHash:         commitHash,
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyName:        "FailStep",
				objects.FieldKeyTitle:       "Fail Step",
				objects.FieldKeyStatus:      objects.ObjectStatusPendingVerification,
				"verification_strategy":     "command_exit_code",
				objects.FieldKeyCommand:     "false",
				objects.FieldKeyDescription: "Exit code verification that fails",
			},
		},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusPendingVerification)

	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskID})
	errChan := make(chan error, 1)
	goroutinelabels.NewGoroutine("sync_loop_test", "runSyncLoop verify fail").StartSimple(func() {
		errChan <- cmd.ExecuteContext(context.Background())
	})

	select {
	case loopErr := <-errChan:
		require.NoError(t, loopErr)
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for runSyncLoop to fail verification and complete task")
	}

	bypassCtx := pkgctx.WithBypassCache(context.Background())
	updated, err := store.Read(bypassCtx, secCtx, taskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusError, updated[objects.FieldKeyStatus])

	writeQueue := caspkg.GetListingIndexWriteQueueForProjectRoot(root)
	if writeQueue != nil {
		_ = writeQueue.FlushAll(1 * time.Second)
	}
	time.Sleep(50 * time.Millisecond)
}

func TestSyncLoop_MaxLoopLimit(t *testing.T) {
	t.Setenv(zqkenv.AgentSyncMaxLoops().Name(), "1")

	_, store := setupSyncLoopTestProject(t)

	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	taskID := "ATK-MAX-LOOP-TASK"
	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              "Max Loop Task",
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    "1h",
		objects.FieldKeyTaskSteps: []any{
			map[string]any{
				objects.FieldKeyTitle:       "Step Pending",
				objects.FieldKeyStatus:      objects.ObjectStatusPending,
				objects.FieldKeyDescription: "Must implement something",
			},
		},
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusInProgress)

	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskID})
	errChan := make(chan error, 1)
	goroutinelabels.NewGoroutine("sync_loop_test", "runSyncLoop max loops").StartSimple(func() {
		errChan <- cmd.ExecuteContext(context.Background())
	})

	select {
	case loopErr := <-errChan:
		assert.ErrorContains(t, loopErr, "max sync loop limit reached")
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for runSyncLoop to hit max loops")
	}

	bypassCtx := pkgctx.WithBypassCache(context.Background())
	updated, err := store.Read(bypassCtx, secCtx, taskID)
	require.NoError(t, err)
	assert.Equal(t, objects.ObjectStatusFailed, updated[objects.FieldKeyStatus])
}
