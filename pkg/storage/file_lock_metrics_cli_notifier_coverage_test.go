package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// mockProgressEventEmitter records progress events
type mockProgressEventEmitter struct {
	progressCalls []string
	statusCalls   []string
	errorCalls    []string
	completeCalls []string
}

func (m *mockProgressEventEmitter) EmitProgress(ctx context.Context, operationID, operationType string, progress, total int, message string, fields map[string]any, emitAudit bool) error {
	m.progressCalls = append(m.progressCalls, operationID)
	return nil
}

func (m *mockProgressEventEmitter) EmitStatusChange(ctx context.Context, operationID, operationType, oldStatus, newStatus, message string, fields map[string]any) error {
	m.statusCalls = append(m.statusCalls, operationID)
	return nil
}

func (m *mockProgressEventEmitter) EmitError(ctx context.Context, operationID, operationType string, err error, message string, fields map[string]any) error {
	m.errorCalls = append(m.errorCalls, operationID)
	return nil
}

func (m *mockProgressEventEmitter) EmitCompletion(ctx context.Context, operationID, operationType string, duration time.Duration, message string, fields map[string]any) error {
	m.completeCalls = append(m.completeCalls, operationID)
	return nil
}

// mockSchedulerWave11 implements storage.SchedulerInterface
type mockSchedulerWave11 struct {
	triggeredJobs       []string
	triggeredEvents     []string
	triggeredLifecycles []string
	failEvent           bool
}

func (m *mockSchedulerWave11) TriggerJob(ctx context.Context, jobID string) error {
	m.triggeredJobs = append(m.triggeredJobs, jobID)
	return nil
}

func (m *mockSchedulerWave11) TriggerJobByEvent(ctx context.Context, eventType, eventKind string, eventData map[string]any) error {
	if m.failEvent {
		return errors.New("event trigger failed")
	}
	m.triggeredEvents = append(m.triggeredEvents, eventType+":"+eventKind)
	return nil
}

func (m *mockSchedulerWave11) TriggerJobByLifecycle(ctx context.Context, kind, fromState, toState string, objectData map[string]any) error {
	m.triggeredLifecycles = append(m.triggeredLifecycles, kind+":"+fromState+"->"+toState)
	return nil
}

// TestStorageExtended_Wave11_FileLockMetricsAsync tests file_lock_metrics_async.go
func TestStorageExtended_FileLockMetricsAsync(t *testing.T) {
	mockStorage := newMockStorageProviderWave8()
	collector := storagepkg.NewFileLockMetricsAsyncCollector(mockStorage)
	require.NotNil(t, collector)

	// Test GetAsyncCollectorStats
	enqueued, dropped := collector.GetAsyncCollectorStats()
	assert.Equal(t, int64(0), enqueued)
	assert.Equal(t, int64(0), dropped)

	// Global collector singleton
	globalCol := storagepkg.GetFileLockMetricsAsyncCollector(mockStorage)
	assert.NotNil(t, globalCol)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	wStart := time.Now().Add(-time.Hour)
	wEnd := time.Now()

	// Enqueue batch
	called := false
	err := collector.CollectMetricsAsync(ctx, secCtx, wStart, wEnd, func(id string, err error) {
		called = true
	})
	assert.NoError(t, err)

	enqueued, _ = collector.GetAsyncCollectorStats()
	assert.Equal(t, int64(1), enqueued)

	// Disable & Enable
	collector.Disable()
	// When disabled, CollectMetricsAsync returns immediately without queueing
	err = collector.CollectMetricsAsync(ctx, secCtx, wStart, wEnd, nil)
	assert.NoError(t, err)

	collector.Enable()

	// Stop collector
	collector.Stop()
	// Idempotent Stop call
	collector.Stop()

	// Nil stats
	var nilCol *storagepkg.FileLockMetricsAsyncCollector
	enq, drp := nilCol.GetAsyncCollectorStats()
	assert.Equal(t, int64(0), enq)
	assert.Equal(t, int64(0), drp)
	_ = called
}

// TestStorageExtended_Wave11_SchedulerIntegration tests scheduler_integration.go
func TestStorageExtended_SchedulerIntegration(t *testing.T) {
	mockStorage := newMockStorageProviderWave8()
	mockSched := &mockSchedulerWave11{}

	sjm := storagepkg.NewSchedulerJobManager(mockStorage, mockSched)
	require.NotNil(t, sjm)

	ctx := context.Background()

	// 1. ScheduleCacheInvalidation via event
	err := sjm.ScheduleCacheInvalidation(ctx, []string{"GOAL-001"}, "testing")
	assert.NoError(t, err)
	assert.Len(t, mockSched.triggeredEvents, 1)

	// 2. ScheduleCacheInvalidation with failing event trigger -> fallback to direct
	mockSched.failEvent = true
	err = sjm.ScheduleCacheInvalidation(ctx, []string{"GOAL-001"}, "testing fallback")
	assert.NoError(t, err)

	// 3. ScheduleCacheInvalidation without scheduler
	sjmNoSched := storagepkg.GetSchedulerJobManager(mockStorage)
	assert.NotNil(t, sjmNoSched)
	err = sjmNoSched.ScheduleCacheInvalidation(ctx, []string{"GOAL-002"}, "direct execution")
	assert.NoError(t, err)

	// 4. ScheduleCascadeUpdate via event
	mockSched.failEvent = false
	err = sjm.ScheduleCascadeUpdate(ctx, "GOAL-001", "goal", "nullify", []string{"BLI-001"})
	assert.NoError(t, err)

	// 5. ScheduleCascadeUpdate with failing event trigger -> fallback to direct
	mockSched.failEvent = true
	err = sjm.ScheduleCascadeUpdate(ctx, "GOAL-001", "goal", "nullify", []string{"BLI-001"})
	assert.NoError(t, err)

	// 6. ScheduleCascadeUpdate direct (delete and nullify) without scheduler
	err = sjmNoSched.ScheduleCascadeUpdate(ctx, "GOAL-001", "goal", "delete", []string{"BLI-001"})
	assert.NoError(t, err)

	err = sjmNoSched.ScheduleCascadeUpdate(ctx, "GOAL-001", "goal", "unknown_type", []string{"BLI-001"})
	assert.Error(t, err)

	// 7. ScheduleOperation
	mockSched.failEvent = false
	err = sjm.ScheduleOperation(ctx, "create", "GOAL-003", "goal", map[string]any{"title": "Op Goal"})
	assert.NoError(t, err)

	// ScheduleOperation without scheduler returns error
	err = sjmNoSched.ScheduleOperation(ctx, "create", "GOAL-003", "goal", nil)
	assert.Error(t, err)
}

// TestStorageExtended_Wave11_CLINotifier tests cli_notifier.go
func TestStorageExtended_CLINotifier(t *testing.T) {
	// Event emitter required
	_, err := storagepkg.NewCLINotifier(false, false, nil)
	assert.Error(t, err)

	emitter := &mockProgressEventEmitter{}
	notifier, err := storagepkg.NewCLINotifier(true, false, emitter)
	require.NoError(t, err)
	require.NotNil(t, notifier)

	op := &storagepkg.Operation{
		ID:       "OP-100",
		ObjectID: "GOAL-100",
		Type:     storagepkg.OpCreate,
	}

	// 1. NotifyProgress
	err = notifier.NotifyProgress(op, 25, "Processing 25%")
	assert.NoError(t, err)
	assert.Len(t, emitter.progressCalls, 1)

	// GetProgress
	progMap := notifier.GetProgress()
	assert.Contains(t, progMap, "OP-100")
	assert.Equal(t, 25, progMap["OP-100"].Progress)

	// 2. NotifyStatus
	err = notifier.NotifyStatus(op, storagepkg.StatusPending, storagepkg.StatusRunning)
	assert.NoError(t, err)
	assert.Len(t, emitter.statusCalls, 1)

	// 3. NotifyError
	err = notifier.NotifyError(op, errors.New("test error"))
	assert.NoError(t, err)
	assert.Len(t, emitter.errorCalls, 1)

	// 4. NotifyCompletion
	err = notifier.NotifyCompletion(op)
	assert.NoError(t, err)
	assert.Len(t, emitter.completeCalls, 1)

	// Quiet mode: skips logging and emissions
	quietNotifier, err := storagepkg.NewCLINotifier(false, true, emitter)
	require.NoError(t, err)
	assert.NoError(t, quietNotifier.NotifyProgress(op, 50, "Quiet progress"))
	assert.NoError(t, quietNotifier.NotifyStatus(op, storagepkg.StatusRunning, storagepkg.StatusComplete))
	assert.NoError(t, quietNotifier.NotifyError(op, errors.New("quiet err")))
	assert.NoError(t, quietNotifier.NotifyCompletion(op))
}
