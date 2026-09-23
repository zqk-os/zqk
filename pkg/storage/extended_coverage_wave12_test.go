package storage

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
)

type mockSnapshotWave12 struct {
	totalOps int64
}

func (m *mockSnapshotWave12) GetTotalOperations() int64 {
	return m.totalOps
}

type mockStorageWave12 struct {
	NoopObjectStorage
	mu          sync.Mutex
	createFunc  func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error
	readFunc    func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error)
	updateFunc  func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error
	deleteFunc  func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error
	createCalls []map[string]any
	readCalls   []string
	updateCalls []string
	deleteCalls []string
}

func (m *mockStorageWave12) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	m.createCalls = append(m.createCalls, obj)
	fn := m.createFunc
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, secCtx, obj)
	}
	if _, ok := obj[objects.FieldKeyID]; !ok {
		obj[objects.FieldKeyID] = "MOCK-ID-123"
	}
	return nil
}

func (m *mockStorageWave12) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	m.mu.Lock()
	m.readCalls = append(m.readCalls, id)
	fn := m.readFunc
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, secCtx, id)
	}
	return map[string]any{
		objects.FieldKeyID:        id,
		objects.FieldKeyKind:      "backlog_item",
		objects.FieldKeyUpdatedAt: "2026-01-01T00:00:00Z",
	}, nil
}

func (m *mockStorageWave12) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	m.mu.Lock()
	m.updateCalls = append(m.updateCalls, id)
	fn := m.updateFunc
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, secCtx, id, updates)
	}
	return nil
}

func (m *mockStorageWave12) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
	m.mu.Lock()
	m.deleteCalls = append(m.deleteCalls, id)
	fn := m.deleteFunc
	m.mu.Unlock()
	if fn != nil {
		return fn(ctx, secCtx, id, cascade)
	}
	return nil
}

func TestStorageExtended_Wave12_MetricsFramework(t *testing.T) {
	t.Run("BaseMetricsCollector_SuccessAndReset", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		var resetCalled bool
		cfg := MetricBuilderConfig{
			Kind:        MetricKindFileLockMetric,
			TitlePrefix: "Test File Lock",
			MetricType:  StorageMetricTypePerformance,
			Source:      FileLockMetricSource,
			Tags:        []string{StorageMetricTagFileLock},
			IDPrefix:    "FLM",
			GetSnapshot: func() MetricsSnapshot {
				return &mockSnapshotWave12{totalOps: 42}
			},
			ResetMetrics: func() {
				resetCalled = true
			},
			BuildMetricObject: func(builder any, snapshot MetricsSnapshot, windowStart, windowEnd time.Time) error {
				b, ok := builder.(*bldr_instance_v1.FileLockMetricInstanceBuilder)
				if !ok {
					return errors.New("unexpected builder type")
				}
				b.TotalAcquisitions(int(snapshot.GetTotalOperations()))
				return nil
			},
		}

		collector := NewBaseMetricsCollector(mockStorage, cfg)
		require.NotNil(t, collector)

		ctx := context.Background()
		secCtx := pkgctx.NewSystemSecurityContext()
		now := time.Now().UTC()
		wStart := now.Add(-10 * time.Minute)
		wEnd := now

		metricID, err := collector.CollectMetrics(ctx, secCtx, wStart, wEnd)
		require.NoError(t, err)
		assert.NotEmpty(t, metricID)

		// Test CollectAndReset
		resetCalled = false
		metricID2, err := collector.CollectAndReset(ctx, secCtx, wStart, wEnd)
		require.NoError(t, err)
		assert.NotEmpty(t, metricID2)
		assert.True(t, resetCalled)
	})

	t.Run("BaseMetricsCollector_Errors", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		ctx := context.Background()
		secCtx := pkgctx.NewSystemSecurityContext()
		now := time.Now().UTC()

		// Unknown schema kind in registry
		cfgUnknownKind := MetricBuilderConfig{
			Kind: "non_existent_metric_kind_xyz",
			GetSnapshot: func() MetricsSnapshot {
				return &mockSnapshotWave12{totalOps: 1}
			},
		}
		c1 := NewBaseMetricsCollector(mockStorage, cfgUnknownKind)
		_, err := c1.CollectMetrics(ctx, secCtx, now, now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to get latest schema version")

		// Kind exists in registry (e.g. "backlog_item") but createMetricBuilder returns nil
		cfgNoBuilder := MetricBuilderConfig{
			Kind: objects.KindBacklogItem,
			GetSnapshot: func() MetricsSnapshot {
				return &mockSnapshotWave12{totalOps: 1}
			},
		}
		c2 := NewBaseMetricsCollector(mockStorage, cfgNoBuilder)
		_, err = c2.CollectMetrics(ctx, secCtx, now, now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported metric kind")

		// BuildMetricObject fails
		cfgBuildErr := MetricBuilderConfig{
			Kind: MetricKindFileLockMetric,
			GetSnapshot: func() MetricsSnapshot {
				return &mockSnapshotWave12{totalOps: 1}
			},
			BuildMetricObject: func(builder any, snapshot MetricsSnapshot, windowStart, windowEnd time.Time) error {
				return errors.New("build error injected")
			},
		}
		c3 := NewBaseMetricsCollector(mockStorage, cfgBuildErr)
		_, err = c3.CollectMetrics(ctx, secCtx, now, now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to build metric object")

		// Storage Create fails
		mockFailStorage := &mockStorageWave12{
			createFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
				return errors.New("disk write failure")
			},
		}
		cfgCreateErr := MetricBuilderConfig{
			Kind: MetricKindFileLockMetric,
			GetSnapshot: func() MetricsSnapshot {
				return &mockSnapshotWave12{totalOps: 1}
			},
			BuildMetricObject: func(builder any, snapshot MetricsSnapshot, windowStart, windowEnd time.Time) error {
				return nil
			},
		}
		c4 := NewBaseMetricsCollector(mockFailStorage, cfgCreateErr)
		_, err = c4.CollectMetrics(ctx, secCtx, now, now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create metric")

		// Storage Create succeeds but removes ID
		mockNoIDStorage := &mockStorageWave12{
			createFunc: func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
				delete(obj, objects.FieldKeyID)
				return nil
			},
		}
		c5 := NewBaseMetricsCollector(mockNoIDStorage, cfgCreateErr)
		_, err = c5.CollectMetrics(ctx, secCtx, now, now)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "metric ID not set")
	})

	t.Run("createMetricBuilder_and_Helpers", func(t *testing.T) {
		// FileLockMetric
		b1 := createMetricBuilder(MetricKindFileLockMetric, "1.0.0")
		assert.NotNil(t, b1)

		// CommandMetric
		b2 := createMetricBuilder(MetricKindCommandMetric, "1.0.0")
		assert.NotNil(t, b2)

		// Unknown kind
		b3 := createMetricBuilder("unknown_metric_foo", "1.0.0")
		assert.Nil(t, b3)

		// setCommonMetricFields on various builders
		setCommonMetricFields(b1, "FLM-1", "Lock Metric", "system", "file_lock", []string{"t1"}, 10, "start", "end", "created")
		setCommonMetricFields(b2, "CM-1", "Command Metric", "system", "cmd", []string{"t2"}, 20, "start", "end", "created")
		setCommonMetricFields("not_a_builder", "ID", "Title", "type", "src", nil, 0, "", "", "")

		// buildMetricInstance
		inst, err := buildMetricInstance(b1)
		require.NoError(t, err)
		assert.NotNil(t, inst)

		_, err = buildMetricInstance("not_buildable")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported builder type")
	})

	t.Run("AsyncMetricsCollector", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		cfg := MetricBuilderConfig{
			Kind: MetricKindFileLockMetric,
			GetSnapshot: func() MetricsSnapshot {
				return &mockSnapshotWave12{totalOps: 5}
			},
			ResetMetrics: func() {},
			BuildMetricObject: func(builder any, snapshot MetricsSnapshot, windowStart, windowEnd time.Time) error {
				return nil
			},
		}
		baseCollector := NewBaseMetricsCollector(mockStorage, cfg)

		// Default buffer size when <= 0
		async0 := NewAsyncMetricsCollector(baseCollector, 0)
		assert.Equal(t, 100, async0.bufferSize)
		async0.Stop()

		// Custom buffer size
		async := NewAsyncMetricsCollector(baseCollector, 10)
		require.NotNil(t, async)

		// CollectAsync when enabled
		var cbExecuted atomic.Bool
		var receivedID string
		ctx := context.Background()
		secCtx := pkgctx.NewSystemSecurityContext()
		now := time.Now()

		err := async.CollectMetricsAsync(ctx, secCtx, now.Add(-time.Minute), now, func(metricID string, err error) {
			receivedID = metricID
			cbExecuted.Store(true)
		})
		require.NoError(t, err)

		// Disable
		async.Disable()
		err = async.CollectMetricsAsync(ctx, secCtx, now, now, func(id string, err error) {})
		require.NoError(t, err)

		// Re-enable
		async.Enable()

		// Stop is idempotent
		async.Stop()
		async.Stop()

		assert.True(t, cbExecuted.Load())
		assert.NotEmpty(t, receivedID)
	})
}

type mockConflictResolverWave12 struct {
	strategy ResolutionStrategy
	err      error
}

func (m *mockConflictResolverWave12) ResolveConflict(ctx context.Context, op *Operation, conflict *Conflict) (ResolutionStrategy, error) {
	return m.strategy, m.err
}

func TestStorageExtended_Wave12_OperationExecutor(t *testing.T) {
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	t.Run("Lifecycle_Constructors_Stats_ProjectRoot", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		queue := NewOperationQueue(nil)

		exec1 := NewOperationExecutor(ctx, mockStorage, queue, 2, nil)
		require.NotNil(t, exec1)
		exec1.SetProjectRoot("/tmp/test-project-root")
		assert.Equal(t, "/tmp/test-project-root", exec1.projectRoot)

		err := exec1.Start()
		require.NoError(t, err)

		executed, failed := exec1.GetOperationExecutorStats()
		assert.Equal(t, int64(0), executed)
		assert.Equal(t, int64(0), failed)

		status := exec1.GetConsistencyStatus()
		assert.NotNil(t, status)

		exec1.Stop()

		// Custom config
		retryCfg := &RetryConfig{
			MaxAttempts:   2,
			InitialDelay:  1 * time.Millisecond,
			MaxDelay:      10 * time.Millisecond,
			BackoffFactor: 1.5,
		}
		exec2 := NewOperationExecutorWithConfig(ctx, mockStorage, queue, 1, nil, 5*time.Second, retryCfg)
		require.NotNil(t, exec2)
		exec2.Stop()
	})

	t.Run("executeOperationOnce_UnknownType", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		queue := NewOperationQueue(nil)
		exec := NewOperationExecutor(ctx, mockStorage, queue, 1, nil)
		defer exec.Stop()

		op := &Operation{
			Type:       OperationType("bogus_operation"),
			ObjectID:   "ITEM-001",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
		}
		err := exec.executeOperationOnce(ctx, op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown operation type")
	})

	t.Run("executeCreateWithCache", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		queue := NewOperationQueue(nil)
		exec := NewOperationExecutor(ctx, mockStorage, queue, 1, nil)
		defer exec.Stop()

		op := &Operation{
			Type:       OperationCreate,
			ObjectID:   "ITEM-NEW",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
			Data: map[string]any{
				objects.FieldKeyID:   "ITEM-NEW",
				objects.FieldKeyKind: "backlog_item",
			},
		}

		// Read returns object (already exists)
		err := exec.executeCreateWithCache(ctx, op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "object already exists")

		// Read returns other error
		mockStorage.readFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, errors.New("disk failure on read")
		}
		err = exec.executeCreateWithCache(ctx, op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to check if object exists")

		// Read returns ErrObjectNotFound -> proceed to create
		mockStorage.readFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, ErrObjectNotFound
		}
		err = exec.executeCreateWithCache(ctx, op)
		require.NoError(t, err)

		// Create fails
		mockStorage.createFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
			return errors.New("disk write error")
		}
		err = exec.executeCreateWithCache(ctx, op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create object")
	})

	t.Run("executeCreate_Legacy_And_ConflictResolver", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		queue := NewOperationQueue(nil)

		// Object already exists, no resolver
		exec := NewOperationExecutor(ctx, mockStorage, queue, 1, nil)
		op := &Operation{
			Type:       OperationCreate,
			ObjectID:   "ITEM-LEGACY",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
			Data: map[string]any{
				objects.FieldKeyID: "ITEM-LEGACY",
			},
		}
		err := exec.executeCreate(op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "object already exists")

		// Resolver with error
		resolverErr := &mockConflictResolverWave12{err: errors.New("resolver failure")}
		execResErr := NewOperationExecutor(ctx, mockStorage, queue, 1, resolverErr)
		err = execResErr.executeCreate(op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolver failure")

		// Resolver StrategySkip
		resolverSkip := &mockConflictResolverWave12{strategy: StrategySkip}
		execSkip := NewOperationExecutor(ctx, mockStorage, queue, 1, resolverSkip)
		err = execSkip.executeCreate(op)
		require.NoError(t, err)

		// Resolver StrategyReject
		resolverReject := &mockConflictResolverWave12{strategy: StrategyReject}
		execReject := NewOperationExecutor(ctx, mockStorage, queue, 1, resolverReject)
		err = execReject.executeCreate(op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "object already exists")

		// Resolver StrategyMerge
		resolverMerge := &mockConflictResolverWave12{strategy: StrategyMerge}
		execMerge := NewOperationExecutor(ctx, mockStorage, queue, 1, resolverMerge)
		err = execMerge.executeCreate(op)
		require.NoError(t, err)

		// Resolver default strategy
		resolverDefault := &mockConflictResolverWave12{strategy: "unknown"}
		execDefault := NewOperationExecutor(ctx, mockStorage, queue, 1, resolverDefault)
		err = execDefault.executeCreate(op)
		require.Error(t, err)

		// Not found -> creates
		mockStorage.readFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, ErrObjectNotFound
		}
		err = exec.executeCreate(op)
		require.NoError(t, err)

		// Create fails
		mockStorage.createFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
			return errors.New("cannot create")
		}
		err = exec.executeCreate(op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create object")
	})

	t.Run("executeUpdate_And_VersionConflict", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		queue := NewOperationQueue(nil)
		exec := NewOperationExecutor(ctx, mockStorage, queue, 1, nil)

		op := &Operation{
			Type:       OperationUpdate,
			ObjectID:   "ITEM-UPD",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
			Updates: map[string]any{
				"title": "New Title",
			},
		}

		// Read fails
		mockStorage.readFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, errors.New("read failed")
		}
		err := exec.executeUpdate(op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read object")

		// Version conflict with no resolver
		mockStorage.readFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return map[string]any{
				objects.FieldKeyID:        id,
				objects.FieldKeyUpdatedAt: "2026-02-01T00:00:00Z",
			}, nil
		}
		op.Metadata = map[string]string{
			opMetadataExpectedUpdated: "2026-01-01T00:00:00Z",
		}
		err = exec.executeUpdate(op)
		require.ErrorIs(t, err, ErrVersionConflict)

		// Version conflict with StrategyReject
		resolverReject := &mockConflictResolverWave12{strategy: StrategyReject}
		execReject := NewOperationExecutor(ctx, mockStorage, queue, 1, resolverReject)
		err = execReject.executeUpdate(op)
		require.ErrorIs(t, err, ErrVersionConflict)

		// Version conflict with StrategyRetry
		resolverRetry := &mockConflictResolverWave12{strategy: StrategyRetry}
		execRetry := NewOperationExecutor(ctx, mockStorage, queue, 1, resolverRetry)
		err = execRetry.executeUpdate(op)
		require.NoError(t, err)

		// Version conflict resolver returns error
		op.Metadata = map[string]string{
			opMetadataExpectedUpdated: "2026-01-01T00:00:00Z",
		}
		resolverErr := &mockConflictResolverWave12{err: errors.New("resolver error")}
		execResErr := NewOperationExecutor(ctx, mockStorage, queue, 1, resolverErr)
		err = execResErr.executeUpdate(op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "resolver error")

		// Success path (matching expected_updated_at or no metadata)
		op.Metadata = nil
		err = exec.executeUpdate(op)
		require.NoError(t, err)

		// Storage Update fails
		mockStorage.updateFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
			return errors.New("update write failure")
		}
		err = exec.executeUpdate(op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update object")
	})

	t.Run("executeDelete_And_Idempotence", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		queue := NewOperationQueue(nil)
		exec := NewOperationExecutor(ctx, mockStorage, queue, 1, nil)

		op := &Operation{
			Type:       OperationDelete,
			ObjectID:   "ITEM-DEL",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
			Metadata: map[string]string{
				opMetadataCascade: opMetadataBoolTrue,
			},
		}

		// Read returns ErrObjectNotFound -> idempotent, returns nil
		mockStorage.readFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, ErrObjectNotFound
		}
		err := exec.executeDelete(op)
		require.NoError(t, err)

		// Read returns other error
		mockStorage.readFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
			return nil, errors.New("disk read error")
		}
		err = exec.executeDelete(op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read object")

		// Read succeeds -> Delete succeeds
		mockStorage.readFunc = nil
		err = exec.executeDelete(op)
		require.NoError(t, err)

		// Storage Delete fails
		mockStorage.deleteFunc = func(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, cascade bool) error {
			return errors.New("delete error")
		}
		err = exec.executeDelete(op)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to delete object")
	})

	t.Run("executeCascadeNullify_and_SetNull", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		queue := NewOperationQueue(nil)
		exec := NewOperationExecutor(ctx, mockStorage, queue, 1, nil)

		opNullify := &Operation{Type: OperationCascadeNullify}
		err := exec.executeCascadeNullify(opNullify)
		require.NoError(t, err)

		opSetNull := &Operation{Type: OperationCascadeSetNull}
		err = exec.executeCascadeSetNull(opSetNull)
		require.NoError(t, err)
	})

	t.Run("executeOperationWithTimeout_Success_and_Failure", func(t *testing.T) {
		mockStorage := &mockStorageWave12{}
		queue := NewOperationQueue(nil)
		exec := NewOperationExecutor(ctx, mockStorage, queue, 1, nil)

		// Nullify operation succeeds
		opSuccess := &Operation{
			Type:       OperationCascadeNullify,
			ObjectID:   "ITEM-SUCCESS",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
		}
		err := exec.executeOperationWithTimeout(opSuccess)
		require.NoError(t, err)
		execCount, failCount := exec.GetOperationExecutorStats()
		assert.Equal(t, int64(1), execCount)
		assert.Equal(t, int64(0), failCount)

		// Bogus operation fails
		opFail := &Operation{
			Type:       OperationType("bogus"),
			ObjectID:   "ITEM-FAIL",
			ObjectKind: "backlog_item",
			Context:    ctx,
			SecCtx:     secCtx,
		}
		err = exec.executeOperationWithTimeout(opFail)
		require.Error(t, err)
		execCount, failCount = exec.GetOperationExecutorStats()
		assert.Equal(t, int64(1), execCount)
		assert.Equal(t, int64(1), failCount)
	})
}
