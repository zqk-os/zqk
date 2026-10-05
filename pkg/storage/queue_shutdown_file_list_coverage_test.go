package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// mockQueueHandler implements QueueShutdownHandler for testing QueueShutdownCoordinator
type mockQueueHandler struct {
	name         string
	isCritical   bool
	pending      int64
	initErr      error
	drainErr     error
	drainDelay   time.Duration
	shutdownDone bool
}

func (m *mockQueueHandler) InitiateShutdown() error {
	m.shutdownDone = true
	return m.initErr
}

func (m *mockQueueHandler) Drain(ctx context.Context) error {
	if m.drainDelay > 0 {
		select {
		case <-time.After(m.drainDelay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	m.pending = 0
	return m.drainErr
}

func (m *mockQueueHandler) IsDrained() bool {
	return m.pending == 0
}

func (m *mockQueueHandler) GetPendingCount() int64 {
	return m.pending
}

func (m *mockQueueHandler) GetName() string {
	return m.name
}

func (m *mockQueueHandler) IsCritical() bool {
	return m.isCritical
}

func TestStorageExtended_QueueShutdownCoordinator(t *testing.T) {
	ctx := context.Background()

	// 1. Config and global coordinator
	cfg := DefaultShutdownConfig()
	assert.NotNil(t, cfg)
	assert.True(t, cfg.ForceShutdown)
	assert.True(t, cfg.LogIncomplete)
	assert.Equal(t, 30*time.Second, cfg.Timeout)

	globalCoord := GetGlobalShutdownCoordinator()
	assert.NotNil(t, globalCoord)

	// 2. Event callback
	var eventCount atomic.Int32
	SetQueueShutdownEventCallback(func(
		ctx context.Context,
		queueName string,
		eventType string,
		pendingCount int64,
		isCritical bool,
		duration time.Duration,
		err error,
	) {
		eventCount.Add(1)
	})
	assert.NotNil(t, getQueueShutdownEventCallback())

	// 3. Isolated coordinator instance with testOverride=true
	coord := &QueueShutdownCoordinator{
		shutdownComplete: make(chan struct{}),
		queues:           make([]QueueShutdownHandler, 0),
		config: &ShutdownConfig{
			Timeout:       200 * time.Millisecond,
			ForceShutdown: true,
			LogIncomplete: true,
			CheckInterval: 10 * time.Millisecond,
		},
		logger:       logging.NewEventLogger(pkgctx.NewSystemContext()),
		testOverride: true,
	}

	assert.False(t, coord.IsShutdownInitiated())

	// Register queues
	q1 := &mockQueueHandler{name: "critical-wal", isCritical: true, pending: 5}
	q2 := &mockQueueHandler{name: "noncritical-orphan", isCritical: false, pending: 2}
	qErr := &mockQueueHandler{name: "failing-queue", isCritical: false, pending: 1, initErr: errors.New("init failed")}

	coord.RegisterQueue(q1)
	coord.RegisterQueue(q2)
	coord.RegisterQueue(qErr)

	// 4. InitiateShutdown
	err := coord.InitiateShutdown()
	assert.NoError(t, err)
	assert.True(t, coord.IsShutdownInitiated())
	// Idempotent call
	assert.NoError(t, coord.InitiateShutdown())

	// 5. DrainAll
	err = coord.DrainAll(ctx)
	assert.NoError(t, err)
	assert.True(t, q1.IsDrained())
	assert.True(t, q2.IsDrained())

	// Reset event callback
	SetQueueShutdownEventCallback(nil)
	assert.Nil(t, getQueueShutdownEventCallback())
}

func TestStorageExtended_FileListCollection(t *testing.T) {
	ctx := context.Background()
	testRoot, fos, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	// 1. extractTimeRangeFromFilters
	filtersEmpty := map[string]any{}
	assert.Nil(t, fos.extractTimeRangeFromFilters(filtersEmpty))

	startStr := "2026-08-01T00:00:00Z"
	endStr := "2026-09-30T23:59:59Z"
	filtersRange := map[string]any{
		objects.FieldKeyCreatedAt: map[string]any{
			"$gte": startStr,
			"$lte": endStr,
		},
	}
	tr := fos.extractTimeRangeFromFilters(filtersRange)
	require.NotNil(t, tr)
	assert.Equal(t, 2026, tr.start.Year())
	assert.Equal(t, time.August, tr.start.Month())
	assert.Equal(t, 2026, tr.end.Year())
	assert.Equal(t, time.September, tr.end.Month())

	// Only $gte
	filtersGTE := map[string]any{
		objects.FieldKeyCreatedAt: map[string]any{
			"$gte": startStr,
		},
	}
	assert.NotNil(t, fos.extractTimeRangeFromFilters(filtersGTE))

	// Invalid date
	filtersInvalid := map[string]any{
		objects.FieldKeyCreatedAt: map[string]any{
			"$gte": "not-a-valid-date",
		},
	}
	assert.Nil(t, fos.extractTimeRangeFromFilters(filtersInvalid))

	// 2. walkBucketedStorageByDateRange and hasDateSubdirectories
	kindDir := filepath.Join(testRoot, "bucketed_test_kind")
	month1 := filepath.Join(kindDir, "2026-08")
	month2 := filepath.Join(kindDir, "2026-09")
	month3 := filepath.Join(kindDir, "2026-10")
	require.NoError(t, os.MkdirAll(month1, 0755))
	require.NoError(t, os.MkdirAll(month2, 0755))
	require.NoError(t, os.MkdirAll(month3, 0755))

	f1 := filepath.Join(month1, "item1.yaml")
	f2 := filepath.Join(month2, "item2.yml")
	f3 := filepath.Join(month3, "item3.yaml")
	require.NoError(t, os.WriteFile(f1, []byte("title: 1"), 0600))
	require.NoError(t, os.WriteFile(f2, []byte("title: 2"), 0600))
	require.NoError(t, os.WriteFile(f3, []byte("title: 3"), 0600))

	assert.True(t, fos.hasDateSubdirectories(kindDir))

	nonDateDir := filepath.Join(testRoot, "nondate_test_kind")
	require.NoError(t, os.MkdirAll(filepath.Join(nonDateDir, "other_sub"), 0755))
	assert.False(t, fos.hasDateSubdirectories(nonDateDir))

	// Walk bucketed storage within August to September (should find f1 and f2, not f3)
	startTime, _ := time.Parse(time.RFC3339, startStr)
	endTime, _ := time.Parse(time.RFC3339, endStr)
	foundFiles := fos.walkBucketedStorageByDateRange(ctx, kindDir, startTime, endTime)
	assert.Len(t, foundFiles, 2)

	// Context cancellation check
	cancelCtx, cancel := context.WithCancel(ctx)
	cancel()
	cancelledFiles := fos.walkBucketedStorageByDateRange(cancelCtx, kindDir, startTime, endTime)
	assert.Empty(t, cancelledFiles)

	// 3. collectFilePathsWithStrategy
	// Stream-backed kinds return nil, nil immediately
	streamFiles, err := fos.collectFilePathsWithStrategy(ctx, objects.KindAuditAggregationMetric, kindDir, nil, nil, nil)
	assert.NoError(t, err)
	assert.Nil(t, streamFiles)

	// Non-stream kind without strategy uses hasDateSubdirectories
	pathsWithRange, err := fos.collectFilePathsWithStrategy(ctx, "bucketed_test_kind", kindDir, nil, tr, nil)
	assert.NoError(t, err)
	assert.Len(t, pathsWithRange, 2)

	// Non-stream kind without time range walks all subdirectories
	pathsAll, err := fos.collectFilePathsWithStrategy(ctx, "bucketed_test_kind", kindDir, nil, nil, nil)
	assert.NoError(t, err)
	assert.Len(t, pathsAll, 3)

	// With PathBasedBucketStrategy
	strat := &PathBasedBucketStrategy{BaseDir: kindDir}
	pathsStrat, err := fos.collectFilePathsWithStrategy(ctx, "bucketed_test_kind", kindDir, strat, tr, nil)
	assert.NoError(t, err)
	assert.NotEmpty(t, pathsStrat)
}
