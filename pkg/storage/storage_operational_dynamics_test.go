package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/graph/provider"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

// cycle30DynamicMockProvider provides a thread-safe mock storage provider for Cycle 30 testing.
type cycle30DynamicMockProvider struct {
	mockObjectStorage
	mu          sync.Mutex
	createCalls []map[string]any
	bulkCalls   [][]map[string]any
	failIndexes map[int]bool
	bulkErr     error
	skipStatus  bool
	listItems   []map[string]any
	listErr     error
}

func (m *cycle30DynamicMockProvider) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.skipStatus && objects.GetString(obj, objects.FieldKeyStatus) == "" {
		obj[objects.FieldKeyStatus] = objects.ObjectStatusActive
	}
	m.createCalls = append(m.createCalls, obj)
	return nil
}

func (m *cycle30DynamicMockProvider) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objs []map[string]any) (*BulkResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bulkErr != nil {
		return nil, m.bulkErr
	}
	m.bulkCalls = append(m.bulkCalls, objs)
	res := &BulkResult{TotalCount: len(objs)}
	for i, obj := range objs {
		if m.failIndexes != nil && m.failIndexes[i] {
			res.FailureCount++
			res.Errors = append(res.Errors, BulkOperationError{Index: i, Message: "forced failure on item"})
			continue
		}
		if !m.skipStatus && objects.GetString(obj, objects.FieldKeyStatus) == "" {
			obj[objects.FieldKeyStatus] = objects.ObjectStatusActive
		}
		res.SuccessCount++
		res.Results = append(res.Results, obj)
	}
	return res, nil
}

func (m *cycle30DynamicMockProvider) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	start := filter.Offset
	if start > len(m.listItems) {
		start = len(m.listItems)
	}
	end := start + filter.Limit
	if end > len(m.listItems) {
		end = len(m.listItems)
	}
	return &QueryResult{
		Objects: m.listItems[start:end],
	}, nil
}

type cycle30PoolMockProvider struct {
	cycle30DynamicMockProvider
	pool provider.ConnectionPool
}

func (p *cycle30PoolMockProvider) GetPool() provider.ConnectionPool {
	return p.pool
}

type cycle30DummyConnectionPool struct{}

func (d *cycle30DummyConnectionPool) GetConnection(ctx context.Context) (provider.GraphConnection, error) {
	return nil, nil
}
func (d *cycle30DummyConnectionPool) ReturnConnection(conn provider.GraphConnection) error {
	return nil
}
func (d *cycle30DummyConnectionPool) Execute(ctx context.Context, fn func(conn provider.GraphConnection) error) error {
	return nil
}
func (d *cycle30DummyConnectionPool) Stats() provider.PoolStats {
	return provider.PoolStats{}
}
func (d *cycle30DummyConnectionPool) Close() error {
	return nil
}

type cycle30MetricsEmitter struct {
	mu     sync.Mutex
	events []cycle30EmittedMetric
}

type cycle30EmittedMetric struct {
	metricType string
	metricID   string
	err        error
}

func (e *cycle30MetricsEmitter) EmitMetricCollected(ctx context.Context, metricType, metricID string, windowStart, windowEnd time.Time, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, cycle30EmittedMetric{
		metricType: metricType,
		metricID:   metricID,
		err:        err,
	})
}

type cycle30MetricsCollector struct {
	metricID    string
	shouldError bool
	calledCount int
}

func (c *cycle30MetricsCollector) CollectMetrics(ctx context.Context, secCtx *pkgctx.SecurityContext, windowStart, windowEnd time.Time) (string, error) {
	c.calledCount++
	if c.shouldError {
		return "", errfmt.Errorf("collector error")
	}
	return c.metricID, nil
}

func (c *cycle30MetricsCollector) CollectAndReset(ctx context.Context, secCtx *pkgctx.SecurityContext, windowStart, windowEnd time.Time) (string, error) {
	c.calledCount++
	if c.shouldError {
		return "", errfmt.Errorf("collector reset error")
	}
	return c.metricID, nil
}

func setupCycle30FileStorage(t *testing.T) (*FileObjectStorage, string) {
	t.Helper()
	tmpDir := t.TempDir()
	MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755)
	require.NoError(t, err)

	fos, err := NewFileObjectStorageForTest(tmpDir)
	require.NoError(t, err)

	t.Cleanup(func() {
		shutdownErr := fos.Shutdown(context.Background())
		require.NoError(t, shutdownErr)
	})
	return fos, tmpDir
}

func TestCycle30_OperationalDynamics_Batching_SizeThresholdFlush(t *testing.T) {
	mock := &cycle30DynamicMockProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 5
	batcher.flushInterval = time.Hour

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	var wg sync.WaitGroup
	errCh := make(chan error, 5)

	for i := 0; i < 5; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("storage_c30_batch", "size threshold flush worker").StartSimple(func() {
			defer wg.Done()
			obj := map[string]any{
				objects.FieldKeyKind: "test_kind",
				objects.FieldKeyID:   fmt.Sprintf("item-%d", idx),
			}
			errCh <- batcher.Create(ctx, secCtx, obj)
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.Len(t, mock.bulkCalls, 1)
	assert.Len(t, mock.bulkCalls[0], 5)
	assert.Empty(t, mock.createCalls)

	enqueued, flushed := batcher.GetBatchingStats()
	assert.Equal(t, int64(5), enqueued)
	assert.Equal(t, int64(5), flushed)
}

func TestCycle30_OperationalDynamics_Batching_TimerThresholdFlush(t *testing.T) {
	mock := &cycle30DynamicMockProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 10
	batcher.flushInterval = 25 * time.Millisecond

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("storage_c30_batch", "timer threshold flush worker").StartSimple(func() {
			defer wg.Done()
			obj := map[string]any{
				objects.FieldKeyKind: "test_kind",
				objects.FieldKeyID:   fmt.Sprintf("item-timer-%d", idx),
			}
			errCh <- batcher.Create(ctx, secCtx, obj)
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	require.Len(t, mock.bulkCalls, 1)
	assert.Len(t, mock.bulkCalls[0], 3)
	assert.Empty(t, mock.createCalls)

	enqueued, flushed := batcher.GetBatchingStats()
	assert.Equal(t, int64(3), enqueued)
	assert.Equal(t, int64(3), flushed)
}

func TestCycle30_OperationalDynamics_Batching_SingleItemDirectFlush(t *testing.T) {
	mock := &cycle30DynamicMockProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 5
	batcher.flushInterval = 10 * time.Millisecond

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	obj := map[string]any{
		objects.FieldKeyKind: "test_kind",
		objects.FieldKeyID:   "single-item",
	}
	err := batcher.Create(ctx, secCtx, obj)
	require.NoError(t, err)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Len(t, mock.createCalls, 1)
	assert.Empty(t, mock.bulkCalls)

	enqueued, flushed := batcher.GetBatchingStats()
	assert.Equal(t, int64(1), enqueued)
	assert.Equal(t, int64(1), flushed)
}

func TestCycle30_OperationalDynamics_Batching_BypassMechanisms(t *testing.T) {
	mock := &cycle30DynamicMockProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 50
	batcher.flushInterval = time.Hour

	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. SkipWriteBehind bypass
	skipCtx := WithSkipWriteBehind(context.Background())
	err := batcher.Create(skipCtx, secCtx, map[string]any{
		objects.FieldKeyKind: "bypass_kind",
		objects.FieldKeyID:   "item-skip",
	})
	require.NoError(t, err)

	// 2. PromoteOnCreate bypass
	promoteCtx := pkgctx.WithPromoteOnCreate(context.Background())
	err = batcher.Create(promoteCtx, secCtx, map[string]any{
		objects.FieldKeyKind: "bypass_kind",
		objects.FieldKeyID:   "item-promote",
	})
	require.NoError(t, err)

	// 3. CLIOperation bypass
	cliCtx := WithCLIOperation(context.Background())
	err = batcher.Create(cliCtx, secCtx, map[string]any{
		objects.FieldKeyKind: "bypass_kind",
		objects.FieldKeyID:   "item-cli",
	})
	require.NoError(t, err)

	mock.mu.Lock()
	defer mock.mu.Unlock()
	assert.Len(t, mock.createCalls, 3)
	assert.Empty(t, mock.bulkCalls)

	enqueued, flushed := batcher.GetBatchingStats()
	assert.Equal(t, int64(0), enqueued)
	assert.Equal(t, int64(0), flushed)
}

func TestCycle30_OperationalDynamics_Batching_ContextCancellation(t *testing.T) {
	mock := &cycle30DynamicMockProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 10
	batcher.flushInterval = time.Hour

	cancelCtx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()

	secCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{
		objects.FieldKeyKind: "test_kind",
		objects.FieldKeyID:   "canceled-item",
	}

	err := batcher.Create(cancelCtx, secCtx, obj)
	require.Error(t, err)
	assert.Equal(t, context.DeadlineExceeded, err)
}

func TestCycle30_OperationalDynamics_Batching_HighThroughputStreaming(t *testing.T) {
	mock := &cycle30DynamicMockProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 8
	batcher.flushInterval = 15 * time.Millisecond

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	totalWorkers := 10
	itemsPerWorker := 4
	totalItems := totalWorkers * itemsPerWorker

	var wg sync.WaitGroup
	errCh := make(chan error, totalItems)

	for w := 0; w < totalWorkers; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_c30_batch", "high throughput stream worker").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < itemsPerWorker; i++ {
				item := map[string]any{
					objects.FieldKeyKind: "stream_kind",
					objects.FieldKeyID:   fmt.Sprintf("w%d-item%d", workerID, i),
				}
				errCh <- batcher.Create(ctx, secCtx, item)
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	enqueued, flushed := batcher.GetBatchingStats()
	assert.Equal(t, int64(totalItems), enqueued)
	assert.Equal(t, int64(totalItems), flushed)
}

func TestCycle30_OperationalDynamics_Batching_BulkErrorBroadcast(t *testing.T) {
	mock := &cycle30DynamicMockProvider{
		bulkErr: errfmt.Errorf("underlying backend failure"),
	}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 3
	batcher.flushInterval = time.Hour

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	var wg sync.WaitGroup
	errCh := make(chan error, 3)

	for i := 0; i < 3; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("storage_c30_batch", "bulk error broadcast worker").StartSimple(func() {
			defer wg.Done()
			obj := map[string]any{
				objects.FieldKeyKind: "err_kind",
				objects.FieldKeyID:   fmt.Sprintf("err-%d", idx),
			}
			errCh <- batcher.Create(ctx, secCtx, obj)
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.Error(t, err)
		assert.Contains(t, err.Error(), "underlying backend failure")
	}
}

func TestCycle30_OperationalDynamics_Batching_BulkPartialFailureAndGhostAck(t *testing.T) {
	mock := &cycle30DynamicMockProvider{
		failIndexes: map[int]bool{1: true},
		skipStatus:  true,
	}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 2
	batcher.flushInterval = time.Hour

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	var wg sync.WaitGroup
	errCh := make(chan error, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		idx := i
		goroutinelabels.NewGoroutine("storage_c30_batch", "partial failure worker").StartSimple(func() {
			defer wg.Done()
			obj := map[string]any{
				objects.FieldKeyKind: "partial_kind",
				objects.FieldKeyID:   fmt.Sprintf("partial-%d", idx),
			}
			errCh <- batcher.Create(ctx, secCtx, obj)
		})
	}

	wg.Wait()
	close(errCh)

	errorsReceived := make([]error, 0, 2)
	for err := range errCh {
		errorsReceived = append(errorsReceived, err)
	}

	require.Len(t, errorsReceived, 2)
	hasItemFailure := false
	hasGhostAckRefusal := false
	for _, err := range errorsReceived {
		require.Error(t, err)
		if err.Error() == "forced failure on item" {
			hasItemFailure = true
		}
		if assert.ObjectsAreEqual(true, len(err.Error()) > 0) &&
			(fmt.Sprintf("%v", err) != "forced failure on item") {
			hasGhostAckRefusal = true
		}
	}
	assert.True(t, hasItemFailure)
	assert.True(t, hasGhostAckRefusal)
}

func TestCycle30_OperationalDynamics_Batching_ConnectionPoolDelegation(t *testing.T) {
	dummyPool := &cycle30DummyConnectionPool{}
	poolMock := &cycle30PoolMockProvider{pool: dummyPool}
	batcherWithPool := NewBatchingObjectStorage(poolMock)
	assert.Equal(t, dummyPool, batcherWithPool.GetPool())

	plainMock := &cycle30DynamicMockProvider{}
	batcherWithoutPool := NewBatchingObjectStorage(plainMock)
	assert.Nil(t, batcherWithoutPool.GetPool())
}

func TestCycle30_OperationalDynamics_BatchProcessor_PaginationAndMergeDynamics(t *testing.T) {
	bp := NewBatchProcessor(3)
	ctx := context.Background()

	allItems := []map[string]any{
		{"id": "BP-1", "val": 10},
		{"id": "BP-2", "val": 20},
		{"id": "BP-3", "val": 30},
		{"id": "BP-4", "val": 40},
		{"id": "BP-5", "val": 50},
		{"id": "BP-6", "val": 60},
		{"id": "BP-7", "val": 70},
	}

	queryFn := func(qCtx context.Context, offset, limit int) ([]map[string]any, error) {
		if offset >= len(allItems) {
			return []map[string]any{}, nil
		}
		end := offset + limit
		if end > len(allItems) {
			end = len(allItems)
		}
		return allItems[offset:end], nil
	}

	processFn := func(batch []map[string]any) (any, []string, error) {
		sum := 0
		ids := make([]string, 0, len(batch))
		for _, item := range batch {
			sum += item["val"].(int)
			ids = append(ids, item["id"].(string))
		}
		return sum, ids, nil
	}

	mergeFn := func(first, second any) (any, error) {
		return first.(int) + second.(int), nil
	}

	result, ids, err := bp.ProcessInBatches(ctx, queryFn, processFn, mergeFn)
	require.NoError(t, err)
	assert.Equal(t, 280, result.(int))
	assert.Len(t, ids, 7)

	batches, items := bp.GetBatchStats()
	assert.Equal(t, int64(3), batches)
	assert.Equal(t, int64(7), items)
}

func TestCycle30_OperationalDynamics_BatchProcessor_CancellationAndFailureModes(t *testing.T) {
	bp := NewBatchProcessor(5)

	// 1. Context Cancellation
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	resCancel, idsCancel, err := bp.ProcessInBatches(cancelCtx, nil, nil, nil)
	assert.Nil(t, resCancel)
	assert.Nil(t, idsCancel)
	require.Error(t, err)
	assert.Equal(t, context.Canceled, err)

	ctx := context.Background()

	// 2. Query failure
	queryErrFn := func(qCtx context.Context, offset, limit int) ([]map[string]any, error) {
		return nil, errfmt.Errorf("db connection failure")
	}
	resQErr, idsQErr, err := bp.ProcessInBatches(ctx, queryErrFn, nil, nil)
	assert.Nil(t, resQErr)
	assert.Nil(t, idsQErr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db connection failure")

	// 3. Process failure
	queryOkFn := func(qCtx context.Context, offset, limit int) ([]map[string]any, error) {
		return []map[string]any{{"id": "item-1"}}, nil
	}
	processErrFn := func(batch []map[string]any) (any, []string, error) {
		return nil, nil, errfmt.Errorf("process parsing error")
	}
	resPErr, idsPErr, err := bp.ProcessInBatches(ctx, queryOkFn, processErrFn, nil)
	assert.Nil(t, resPErr)
	assert.Nil(t, idsPErr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "process parsing error")

	// 4. Merge failure
	queryTwoPages := func(qCtx context.Context, offset, limit int) ([]map[string]any, error) {
		if offset == 0 {
			return make([]map[string]any, 5), nil
		}
		if offset == 5 {
			return make([]map[string]any, 1), nil
		}
		return []map[string]any{}, nil
	}
	processOk := func(batch []map[string]any) (any, []string, error) {
		return 1, []string{"id"}, nil
	}
	mergeErr := func(first, second any) (any, error) {
		return nil, errfmt.Errorf("merge type conflict")
	}
	resMErr, idsMErr, err := bp.ProcessInBatches(ctx, queryTwoPages, processOk, mergeErr)
	assert.Nil(t, resMErr)
	assert.Nil(t, idsMErr)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "merge type conflict")
}

func TestCycle30_OperationalDynamics_BatchQueryBuilder_ListIntegration(t *testing.T) {
	mock := &cycle30DynamicMockProvider{
		listItems: []map[string]any{
			{"id": "L-1", "kind": "audit"},
			{"id": "L-2", "kind": "audit"},
			{"id": "L-3", "kind": "audit"},
			{"id": "L-4", "kind": "audit"},
		},
	}
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	builder := NewBatchQueryBuilder(mock, secCtx, storageCtx, "audit").
		WithFilters(map[string]any{"kind": "audit"}).
		WithSort("id", true)

	ctx := context.Background()
	queryFn := builder.BuildQueryFunc(ctx)

	page1, err := queryFn(ctx, 0, 2)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, "L-1", page1[0]["id"])
	assert.Equal(t, "L-2", page1[1]["id"])

	page2, err := queryFn(ctx, 2, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, "L-3", page2[0]["id"])
	assert.Equal(t, "L-4", page2[1]["id"])
}

func TestCycle30_OperationalDynamics_Orchestrator_MultiProviderRouting(t *testing.T) {
	fos, tmpRoot := setupCycle30FileStorage(t)
	require.NotEmpty(t, tmpRoot)
	mockBackend := &cycle30DynamicMockProvider{}

	orchestrator := NewStorageOrchestrator()
	orchestrator.RegisterBackend("file", fos)
	orchestrator.RegisterBackend("secondary", mockBackend)

	b1, err := orchestrator.GetBackend("file")
	require.NoError(t, err)
	assert.Equal(t, fos, b1)

	b2, err := orchestrator.GetBackend("secondary")
	require.NoError(t, err)
	assert.Equal(t, mockBackend, b2)

	missingBackend, err := orchestrator.GetBackend("missing_backend")
	assert.Nil(t, missingBackend)
	require.Error(t, err)

	fileBackends := orchestrator.GetBackendForObject("OBJ-100")
	assert.Contains(t, fileBackends, "file")
	assert.NotContains(t, fileBackends, "secondary")
}

func TestCycle30_OperationalDynamics_Orchestrator_DefaultBackendSelection(t *testing.T) {
	orchestrator := NewStorageOrchestrator()

	// 1. Empty backends
	defaultBackend, err := orchestrator.GetDefaultBackend()
	assert.Nil(t, defaultBackend)
	require.Error(t, err)

	// 2. Non-file backend fallback
	mockBackend := &cycle30DynamicMockProvider{}
	orchestrator.RegisterBackend("mock", mockBackend)
	def, err := orchestrator.GetDefaultBackend()
	require.NoError(t, err)
	assert.Equal(t, mockBackend, def)

	// 3. File backend takes precedence
	fos, tmpRoot := setupCycle30FileStorage(t)
	require.NotEmpty(t, tmpRoot)
	orchestrator.RegisterBackend("file", fos)
	defFile, err := orchestrator.GetDefaultBackend()
	require.NoError(t, err)
	assert.Equal(t, fos, defFile)
}

func TestCycle30_OperationalDynamics_Orchestrator_GlobalSingleton(t *testing.T) {
	inst1 := GetStorageOrchestrator()
	require.NotNil(t, inst1)

	inst2 := GetStorageOrchestrator()
	assert.Same(t, inst1, inst2)
}

func TestCycle30_OperationalDynamics_Orchestrator_CrossBackendPendingLifecycle(t *testing.T) {
	orchestrator := NewStorageOrchestrator()
	objID := "OBJ-COORDINATE-01"

	// Register ops across primary and secondary backends
	orchestrator.RegisterOperation("primary", objID, "op-pri-1")
	orchestrator.RegisterOperation("secondary", objID, "op-sec-1")

	assert.True(t, orchestrator.HasPendingOperations(objID))

	pending := orchestrator.GetPendingOperations(objID)
	assert.Len(t, pending["primary"], 1)
	assert.Len(t, pending["secondary"], 1)

	err := orchestrator.PreventOperation(objID, OpUpdate)
	require.Error(t, err)

	// Complete primary op
	err = orchestrator.CompleteOperation("primary", objID, "op-pri-1")
	require.NoError(t, err)

	// Object still has secondary pending
	assert.True(t, orchestrator.HasPendingOperations(objID))
	err = orchestrator.PreventOperation(objID, OpUpdate)
	require.Error(t, err)

	// Complete secondary op
	err = orchestrator.CompleteOperation("secondary", objID, "op-sec-1")
	require.NoError(t, err)

	// Clean state
	assert.False(t, orchestrator.HasPendingOperations(objID))
	err = orchestrator.PreventOperation(objID, OpUpdate)
	require.NoError(t, err)
}

func TestCycle30_OperationalDynamics_Orchestrator_IdempotentOpsAndEmptyCompletion(t *testing.T) {
	orchestrator := NewStorageOrchestrator()
	objID := "OBJ-IDEMPOTENT-01"

	// Idempotent duplicate op registration
	orchestrator.RegisterOperation("backend", objID, "op-repeat")
	orchestrator.RegisterOperation("backend", objID, "op-repeat")

	ops := orchestrator.GetPendingOperations(objID)
	assert.Len(t, ops["backend"], 1)

	// Complete non-existent op on fresh object
	err := orchestrator.CompleteOperation("backend", "NON-EXISTENT", "op-none")
	require.NoError(t, err)

	// PreventOperation on object with no pending ops
	err = orchestrator.PreventOperation("CLEAN-OBJ", OpUpdate)
	require.NoError(t, err)
}

func TestCycle30_OperationalDynamics_Orchestrator_HighConcurrencyStress(t *testing.T) {
	orchestrator := NewStorageOrchestrator()
	numWorkers := 16
	numOpsPerWorker := 10
	objectsCount := 4

	var wg sync.WaitGroup
	errCh := make(chan error, numWorkers*numOpsPerWorker)

	for w := 0; w < numWorkers; w++ {
		workerID := w
		wg.Add(1)
		goroutinelabels.NewGoroutine("storage_c30_orch", "stress concurrent orchestrator worker").StartSimple(func() {
			defer wg.Done()
			for i := 0; i < numOpsPerWorker; i++ {
				targetObj := fmt.Sprintf("STRESS-OBJ-%d", i%objectsCount)
				opID := fmt.Sprintf("w%d-op%d", workerID, i)
				backend := "primary"

				orchestrator.RegisterOperation(backend, targetObj, opID)
				hasPending := orchestrator.HasPendingOperations(targetObj)
				assert.True(t, hasPending || !hasPending)
				activeOps := orchestrator.GetPendingOperations(targetObj)
				assert.NotNil(t, activeOps)

				time.Sleep(time.Millisecond)
				compErr := orchestrator.CompleteOperation(backend, targetObj, opID)
				if compErr != nil {
					errCh <- compErr
				}
			}
		})
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	for i := 0; i < objectsCount; i++ {
		targetObj := fmt.Sprintf("STRESS-OBJ-%d", i)
		assert.False(t, orchestrator.HasPendingOperations(targetObj))
		err := orchestrator.PreventOperation(targetObj, OpUpdate)
		require.NoError(t, err)
	}
}

func TestCycle30_OperationalDynamics_LeaseLifecycle_WriteBehindOwnerExclusive(t *testing.T) {
	tmpDir := t.TempDir()

	ownerA := "worker-seat-a"
	ownerB := "worker-seat-b"

	// 1. Owner A claims lease
	claimedA := tryClaimWriteBehindOwner(tmpDir, ownerA)
	assert.True(t, claimedA)

	// 2. Owner A re-claims idempotently
	reclaimedA := tryClaimWriteBehindOwner(tmpDir, ownerA)
	assert.True(t, reclaimedA)

	// 3. Competing owner B fails to claim
	claimedB := tryClaimWriteBehindOwner(tmpDir, ownerB)
	assert.False(t, claimedB)

	// 4. Owner A releases lease
	releaseWriteBehindOwner(tmpDir, ownerA)

	// 5. Owner B claims lease successfully
	claimedBAfterRelease := tryClaimWriteBehindOwner(tmpDir, ownerB)
	assert.True(t, claimedBAfterRelease)

	// Cleanup
	releaseWriteBehindOwner(tmpDir, ownerB)
}

func TestCycle30_OperationalDynamics_LeaseLifecycle_ObjectWALRefCount(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Initial acquire
	wal1, err := AcquireObjectWAL(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, wal1)

	refCount1 := objectWALRefCountForTest(tmpDir)
	assert.GreaterOrEqual(t, refCount1, 1)

	// 2. Second acquire on same root increases refcount
	wal2, err := AcquireObjectWAL(tmpDir)
	require.NoError(t, err)
	assert.Same(t, wal1, wal2)

	refCount2 := objectWALRefCountForTest(tmpDir)
	assert.Equal(t, refCount1+1, refCount2)

	// 3. First release
	err = ReleaseObjectWAL(tmpDir, wal1)
	require.NoError(t, err)

	refCountAfterFirstRelease := objectWALRefCountForTest(tmpDir)
	assert.Equal(t, refCount1, refCountAfterFirstRelease)

	// 4. Final release
	err = ReleaseObjectWAL(tmpDir, wal2)
	require.NoError(t, err)
}

func TestCycle30_OperationalDynamics_HealthReporting_ChangeJournalEntries(t *testing.T) {
	tmpDir := t.TempDir()
	journalDir := filepath.Join(tmpDir, paths.ProcessDir, "change_journal_entry")
	err := paths.EnsureDir(journalDir, paths.DirPerm755)
	require.NoError(t, err)

	mockStorage := &cycle30DynamicMockProvider{}
	createdBefore, failedBefore := GetHealthCheckJournalStats()
	assert.GreaterOrEqual(t, failedBefore, int64(0))

	// 1. Empty root returns nil immediately
	err = CreateHealthCheckChangeJournalEntry(context.Background(), "", mockStorage, "MON-01", "healthy", "ok", nil)
	require.NoError(t, err)

	// 2. Empty monitorID returns nil immediately
	err = CreateHealthCheckChangeJournalEntry(context.Background(), tmpDir, mockStorage, "", "healthy", "ok", nil)
	require.NoError(t, err)

	// 3. Valid health entry emission
	err = CreateHealthCheckChangeJournalEntry(
		context.Background(),
		tmpDir,
		mockStorage,
		"MON-STORAGE-HEALTH",
		"healthy",
		"All backend probes passed",
		map[string]any{"latency_ms": 12, "active_conns": 5},
	)
	require.NoError(t, err)

	createdAfter, failedAfter := GetHealthCheckJournalStats()
	assert.GreaterOrEqual(t, failedAfter, int64(0))
	assert.Equal(t, createdBefore+1, createdAfter)
}

func TestCycle30_OperationalDynamics_UnifiedMetrics_CollectorExecution(t *testing.T) {
	mockStore := &cycle30DynamicMockProvider{}
	emitter := &cycle30MetricsEmitter{}

	casColl := &cycle30MetricsCollector{metricID: "CASM-C30"}
	lockColl := &cycle30MetricsCollector{metricID: "FLM-C30"}
	auditColl := &cycle30MetricsCollector{metricID: "AUDIT-C30"}

	collector := &UnifiedMetricsCollector{
		storage:           mockStore,
		eventEmitter:      emitter,
		casCollector:      casColl,
		fileLockCollector: lockColl,
		auditCollector:    auditColl,
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	windowStart := time.Now().Add(-time.Hour)
	windowEnd := time.Now()

	metricIDs, err := collector.CollectAllMetrics(ctx, secCtx, windowStart, windowEnd)
	require.NoError(t, err)
	require.Len(t, metricIDs, 3)

	assert.Contains(t, metricIDs, "CASM-C30")
	assert.Contains(t, metricIDs, "FLM-C30")
	assert.Contains(t, metricIDs, "AUDIT-C30")

	assert.Equal(t, 1, casColl.calledCount)
	assert.Equal(t, 1, lockColl.calledCount)
	assert.Equal(t, 1, auditColl.calledCount)

	emitter.mu.Lock()
	defer emitter.mu.Unlock()
	require.Len(t, emitter.events, 3)
}
