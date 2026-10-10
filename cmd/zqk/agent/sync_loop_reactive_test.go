package agent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func createSyncLoopTestTask(t *testing.T, store storage.ObjectStorageProvider, taskID, title, effort string) *pkgctx.SecurityContext {
	ctx := storage.WithSyncCreateForKind(context.Background(), objects.KindAgentTask)
	ctx = storage.WithSkipWriteBehind(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	task := map[string]any{
		objects.FieldKeyID:                 taskID,
		objects.FieldKeyKind:               objects.KindAgentTask,
		objects.FieldKeyTitle:              title,
		objects.FieldKeyStatus:             objects.ObjectStatusInProgress,
		objects.FieldKeySchemaVersion:      "2.0.0",
		objects.FieldKeyAssigneePersonaRef: objects.ConstPersonaOrchestratorAlpha,
		objects.FieldKeyEstimatedEffort:    effort,
	}
	storage.CreateCASVisible(t, store, ctx, secCtx, task, objects.ObjectStatusInProgress)
	return secCtx
}

func dispatchTestCallbackEvent(taskID string, timestamp time.Time) int {
	return lifecycle.GlobalJobWakerRegistry.Dispatch(&lifecycle.LifecycleEvent{
		EventType: lifecycle.EventTypeSchedulerCallback,
		ID:        taskID,
		ToStatus:  "completed",
		Ts:        timestamp,
	})
}

func setupBinaryCandidate(t *testing.T) {
	if wd, err := fileutil.Getwd(); err == nil {
		if modRoot, err := paths.ModuleRootFromPath(wd); err == nil {
			candidate := filepath.Join(modRoot, "bin", "zqk")
			if info, err := fileutil.Stat(candidate); err == nil && !info.IsDir() {
				t.Setenv(zqkenv.Bin().Name(), candidate)
			}
		}
	}
}

func TestSyncLoop_ReactiveInstantWake(t *testing.T) {
	root, store := setupSyncLoopTestProject(t)
	setupBinaryCandidate(t)

	taskID := "ATK-reactive-instant-wake"
	secCtx := createSyncLoopTestTask(t, store, taskID, "Reactive Instant Wake Test", "30m")
	require.NotNil(t, secCtx)

	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskID})

	loopCtx, cancelLoop := context.WithTimeout(storage.WithSkipWriteBehind(context.Background()), 15*time.Second)
	defer cancelLoop()

	errChan := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_reactive_sync_loop", "run reactive sync loop").StartSimple(func() {
		errChan <- cmd.ExecuteContext(loopCtx)
	})

	// Wait briefly to ensure the sync-loop enters its select block and registers the waker
	time.Sleep(300 * time.Millisecond)

	// Mutate task to terminal status in storage so when the loop wakes, it terminates immediately
	updateCtx := pkgctx.WithBypassCache(context.Background())
	err := store.Update(updateCtx, secCtx, taskID, map[string]any{
		objects.FieldKeyStatus: objects.ObjectStatusError,
	})
	require.NoError(t, err)

	// Flush CAS index to ensure visibility
	writeQueue := caspkg.GetListingIndexWriteQueueForProjectRoot(root)
	if writeQueue != nil {
		flushErr := writeQueue.FlushAll(1 * time.Second)
		require.NoError(t, flushErr)
	}

	// Dispatch the waker event and measure response latency
	t0 := time.Now()
	dispatched := dispatchTestCallbackEvent(taskID, t0)
	require.GreaterOrEqual(t, dispatched, 1, "expected waker event to be dispatched to active sync-loop listener")

	select {
	case loopErr := <-errChan:
		elapsed := time.Since(t0)
		require.NoError(t, loopErr)
		t.Logf("Reactive wake latency: %v", elapsed)
		require.Less(t, elapsed, 2500*time.Millisecond, "sync-loop did not wake up reactively within fallback polling window")
	case <-time.After(15 * time.Second):
		t.Fatal("timed out waiting for reactive sync loop exit")
	}

	// Clean defer unregistration verification: after loop returns, registry must have 0 listeners
	afterDispatch := dispatchTestCallbackEvent(taskID, time.Time{})
	require.Equal(t, 0, afterDispatch, "expected waker channel to be unregistered after sync loop exits")
}

func TestSyncLoop_WakerChannelDeliveryLatency(t *testing.T) {
	taskID := "ATK-latency-test"
	wakerCh, unregister := lifecycle.GlobalJobWakerRegistry.Register(taskID)
	defer unregister()

	t0 := time.Now()
	dispatched := dispatchTestCallbackEvent(taskID, t0)
	require.Equal(t, 1, dispatched)

	select {
	case ev := <-wakerCh:
		elapsed := time.Since(t0)
		require.Equal(t, taskID, ev.ID)
		t.Logf("Waker delivery latency: %v", elapsed)
		require.Less(t, elapsed, 50*time.Millisecond, "waker event delivery exceeded 50ms requirement")
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for waker event delivery")
	}
}

func TestSyncLoop_ContextCancellationCleanExit(t *testing.T) {
	_, store := setupSyncLoopTestProject(t)

	taskID := "ATK-cancel-clean-exit"
	cancelSecCtx := createSyncLoopTestTask(t, store, taskID, "Cancel Clean Exit Test", "15m")
	require.NotNil(t, cancelSecCtx)

	cmd := NewSyncLoopCmd()
	cmd.SetArgs([]string{taskID})

	loopCtx, cancelLoop := context.WithCancel(storage.WithSkipWriteBehind(context.Background()))

	errChan := make(chan error, 1)
	goroutinelabels.NewGoroutine("test_cancel_sync_loop", "run cancellable sync loop").StartSimple(func() {
		errChan <- cmd.ExecuteContext(loopCtx)
	})

	time.Sleep(300 * time.Millisecond)
	cancelLoop()

	select {
	case err := <-errChan:
		require.Error(t, err)
		require.Contains(t, err.Error(), "orchestrator timeout")
	case <-time.After(15 * time.Second):
		t.Fatal("sync loop failed to exit on context cancellation")
	}

	// Verify waker was cleaned up
	remaining := dispatchTestCallbackEvent(taskID, time.Time{})
	require.Equal(t, 0, remaining, "expected no lingering waker listeners after cancellation")
}
