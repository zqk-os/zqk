package storage

import (
	"context"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
)

func TestStorageExtended_AuditMetrics(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	t.Run("DisabledMetricsRecording_EarlyReturns", func(t *testing.T) {
		// When recording is disabled
		metricsrecording.EnterHotPathNoPersist()
		defer metricsrecording.LeaveHotPathNoPersist()

		collector := NewAuditMetricsCollector(&mockStorageWave12{})
		collector.RecordAuditEventCreation(ctx, "test_event", true, time.Millisecond, true)
		collector.RecordAuditEventValidation(true)
		collector.RecordAuditEventDuplicate()
		collector.RecordAuditEventMerged()

		now := time.Now()
		id, err := collector.CollectMetrics(ctx, secCtx, now.Add(-time.Hour), now)
		assert.NoError(t, err)
		assert.Empty(t, id)

		id2, err := collector.CollectAndReset(ctx, secCtx, now.Add(-time.Hour), now)
		assert.NoError(t, err)
		assert.Empty(t, id2)
	})

	t.Run("EnabledMetricsRecording_And_EventEmitter", func(t *testing.T) {
		metricsrecording.EnterAllowRecording()
		defer metricsrecording.LeaveAllowRecording()

		mockStorage := &mockStorageWave12{}
		collector := NewAuditMetricsCollector(mockStorage)
		require.NotNil(t, collector)

		// Event emitter
		var emittedCount atomic.Int32
		emitter := func(event map[string]any) error {
			emittedCount.Add(1)
			return nil
		}
		collector.SetEventEmitter(emitter, true)
		assert.True(t, collector.ShouldEmitEvents())

		collector.emitEvent(map[string]any{"type": "audit_metric_event"})
		time.Sleep(50 * time.Millisecond)
		assert.GreaterOrEqual(t, emittedCount.Load(), int32(1))

		collector.SetEmitEvents(false)
		assert.False(t, collector.ShouldEmitEvents())

		// RecordAuditEventCreation: CAS + Success
		collector.RecordAuditEventCreation(ctx, "create_event", true, 10*time.Millisecond, true)
		// RecordAuditEventCreation: Non-CAS + Failure
		collector.RecordAuditEventCreation(ctx, "create_event", false, 20*time.Millisecond, false)

		// RecordAuditEventValidation: success & failure
		collector.RecordAuditEventValidation(true)
		collector.RecordAuditEventValidation(false)

		// Duplicate & Merged
		collector.RecordAuditEventDuplicate()
		collector.RecordAuditEventMerged()

		// GetSnapshot
		snapshot := collector.GetSnapshot()
		assert.Equal(t, int64(2), snapshot.EventsCreated)
		assert.Equal(t, int64(1), snapshot.EventsCreatedCAS)
		assert.Equal(t, int64(1), snapshot.EventsCreatedID)
		assert.Equal(t, int64(1), snapshot.EventsFailed)
		assert.Equal(t, int64(2), snapshot.EventsValidated)
		assert.Equal(t, int64(1), snapshot.EventsSkipped)
		assert.Equal(t, int64(1), snapshot.EventsDuplicated)
		assert.Equal(t, int64(1), snapshot.EventsMerged)
		assert.Equal(t, int64(1), snapshot.SuccessCount())
		assert.Equal(t, 50.0, snapshot.FailureRate())

		// CollectMetrics
		now := time.Now().UTC()
		wStart := now.Add(-time.Hour)
		wEnd := now

		metricID, err := collector.CollectMetrics(ctx, secCtx, wStart, wEnd)
		require.NoError(t, err)
		assert.NotEmpty(t, metricID)

		// CollectAndReset
		resetID, err := collector.CollectAndReset(ctx, secCtx, wStart, wEnd)
		require.NoError(t, err)
		assert.NotEmpty(t, resetID)

		// Snapshot after reset should have zero created events
		afterReset := collector.GetSnapshot()
		assert.Equal(t, int64(0), afterReset.EventsCreated)

		// Nil storage should return error
		nilStorageCollector := NewAuditMetricsCollector(nil)
		_, err = nilStorageCollector.CollectMetrics(ctx, secCtx, wStart, wEnd)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "storage not configured")
	})
}

func TestStorageExtended_AuditEventBufferLifecycle(t *testing.T) {
	ctx := context.Background()
	tempDir, err := os.MkdirTemp("", "audit_buffer_test_*")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	secCtx := pkgctx.NewSystemSecurityContext()
	buf := NewAuditEventBuffer(tempDir, secCtx, nil)
	require.NotNil(t, buf)

	t.Run("QueueShutdownHandler_Implementation", func(t *testing.T) {
		assert.Equal(t, ValueAuditEventBuffer, buf.GetName())
		assert.False(t, buf.IsCritical())
		assert.True(t, buf.IsDrained())
		assert.Equal(t, int64(0), buf.GetPendingCount())

		err := buf.Drain(ctx)
		require.NoError(t, err)

		err = buf.InitiateShutdown()
		require.NoError(t, err)
	})

	t.Run("GetBufferStats_and_GetAuditEventBufferStats", func(t *testing.T) {
		stats := buf.GetBufferStats()
		assert.NotNil(t, stats)
		assert.Contains(t, stats, "total_events")
		assert.Contains(t, stats, "enabled")

		added, flushed := buf.GetAuditEventBufferStats()
		assert.Equal(t, int64(0), added)
		assert.Equal(t, int64(0), flushed)
	})

	t.Run("SetTSDBProvider_and_SetFlushProgressChannel", func(t *testing.T) {
		buf.SetTSDBProvider(nil)

		ch := make(chan FlushProgress, 5)
		buf.SetFlushProgressChannel(ch)
	})

	t.Run("TearDownGlobalAuditBufferForTestProjectRoot", func(t *testing.T) {
		secCtx := pkgctx.NewSystemSecurityContext()
		resetDir, err := os.MkdirTemp("", "audit_buffer_reset_*")
		require.NoError(t, err)
		defer os.RemoveAll(resetDir)

		// No-op when global buffer root doesn't match
		err = TearDownGlobalAuditBufferForTestProjectRoot("/nonexistent/matching/path", resetDir, secCtx)
		assert.NoError(t, err)

		// Set global buffer to match
		globalBuffer := GetGlobalAuditEventBuffer()
		if globalBuffer != nil {
			curRoot := globalBuffer.GetProjectRoot()
			if curRoot != "" {
				err = TearDownGlobalAuditBufferForTestProjectRoot(curRoot, resetDir, secCtx)
				assert.NoError(t, err)
			}
		}
	})
}

func TestStorageExtended_AuditEventsOps(t *testing.T) {
	secCtx := pkgctx.NewSystemSecurityContext()

	t.Run("CreateCacheRefreshAuditEvent", func(t *testing.T) {
		// Empty project root -> early return nil
		err := CreateCacheRefreshAuditEvent("", secCtx, nil, nil)
		assert.NoError(t, err)

		// Valid project root
		testRoot, _, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
		err = CreateCacheRefreshAuditEvent(testRoot, secCtx, []string{"--force", "--all"}, map[string]any{
			"cache_mode": "strict",
			"entries":    150,
		})
		assert.NoError(t, err)
	})

	t.Run("CreateLintBypassAuditEvent", func(t *testing.T) {
		// Empty project root -> early return nil
		err := CreateLintBypassAuditEvent("", "dev-user", "dev@example.com", "commit message", nil)
		assert.NoError(t, err)

		// Valid project root with short and long commit message, Go and non-Go staged files
		testRoot, _, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
		stagedFiles := []string{
			"pkg/storage/file1.go",
			"pkg/storage/file2.go",
			"docs/readme.md",
		}

		// Short commit message
		err = CreateLintBypassAuditEvent(testRoot, "alice", "alice@example.com", "fix lint issue", stagedFiles)
		assert.NoError(t, err)

		// Long commit message (> 100 characters)
		longMsg := strings.Repeat("Extremely detailed long commit message explaining why lint check was bypassed. ", 3)
		err = CreateLintBypassAuditEvent(testRoot, "", "", longMsg, stagedFiles)
		assert.NoError(t, err)
	})
}
