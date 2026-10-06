package storage

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

// mockStorageProvider is a simple mock for testing batching
type mockStorageProvider struct {
	ObjectStorageProvider

	mu          sync.Mutex
	createCalls []map[string]any
	bulkCalls   [][]map[string]any
}

func (m *mockStorageProvider) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if objects.GetString(obj, objects.FieldKeyStatus) == "" {
		obj[objects.FieldKeyStatus] = objects.ObjectStatusActive
	}
	m.createCalls = append(m.createCalls, obj)
	return nil
}

func (m *mockStorageProvider) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objs []map[string]any) (*BulkResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bulkCalls = append(m.bulkCalls, objs)
	for _, obj := range objs {
		if objects.GetString(obj, objects.FieldKeyStatus) == "" {
			obj[objects.FieldKeyStatus] = objects.ObjectStatusActive
		}
	}
	return &BulkResult{Results: objs, SuccessCount: len(objs), TotalCount: len(objs)}, nil
}

func (m *mockStorageProvider) BulkDelete(ctx context.Context, secCtx *pkgctx.SecurityContext, ids []string, cascade bool) (*BulkResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &BulkResult{SuccessCount: len(ids), TotalCount: len(ids)}, nil
}

func TestBatchingStorageProvider_Batching(t *testing.T) {
	mock := &mockStorageProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 5
	batcher.flushInterval = 10 * time.Millisecond

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	var wg sync.WaitGroup
	// Send 5 creates concurrently to trigger batch size flush
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			obj := map[string]any{
				objects.FieldKeyKind: "test",
				objects.FieldKeyID:   "test-id",
			}
			err := batcher.Create(ctx, secCtx, obj)
			if err != nil {
				t.Errorf("Create failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	mock.mu.Lock()
	defer mock.mu.Unlock()

	// Since we set batch size to 5 and sent 5, we expect 1 BulkCreate call
	// and 0 single Create calls.
	if len(mock.bulkCalls) != 1 {
		t.Errorf("Expected 1 bulk call, got %d", len(mock.bulkCalls))
	}
	if len(mock.bulkCalls) > 0 && len(mock.bulkCalls[0]) != 5 {
		t.Errorf("Expected bulk call with 5 items, got %d", len(mock.bulkCalls[0]))
	}
	if len(mock.createCalls) != 0 {
		t.Errorf("Expected 0 single create calls, got %d", len(mock.createCalls))
	}
}

func TestBatchingStorageProvider_Timeout(t *testing.T) {
	mock := &mockStorageProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 5
	batcher.flushInterval = 50 * time.Millisecond

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Send 2 creates, which should trigger the interval flush
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			obj := map[string]any{
				objects.FieldKeyKind: "test",
				objects.FieldKeyID:   "test-id",
			}
			err := batcher.Create(ctx, secCtx, obj)
			if err != nil {
				t.Errorf("Create failed: %v", err)
			}
		}(i)
	}

	wg.Wait()

	mock.mu.Lock()
	defer mock.mu.Unlock()

	// We expect 1 BulkCreate call with 2 items.
	if len(mock.bulkCalls) != 1 {
		t.Errorf("Expected 1 bulk call, got %d", len(mock.bulkCalls))
	}
	if len(mock.bulkCalls) > 0 && len(mock.bulkCalls[0]) != 2 {
		t.Errorf("Expected bulk call with 2 items, got %d", len(mock.bulkCalls[0]))
	}
}

func TestBatchingStorageProvider_Single(t *testing.T) {
	mock := &mockStorageProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 5
	batcher.flushInterval = 5 * time.Millisecond

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// Send 1 create, which should fallback to single Create instead of BulkCreate
	obj := map[string]any{
		objects.FieldKeyKind: "test",
		objects.FieldKeyID:   "test-id",
	}
	err := batcher.Create(ctx, secCtx, obj)
	if err != nil {
		t.Errorf("Create failed: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if len(mock.createCalls) != 1 {
		t.Errorf("Expected 1 single create call, got %d", len(mock.createCalls))
	}
	if len(mock.bulkCalls) != 0 {
		t.Errorf("Expected 0 bulk calls, got %d", len(mock.bulkCalls))
	}
}

func TestBatchingStorageProvider_BypassSkipWriteBehind(t *testing.T) {
	mock := &mockStorageProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 100
	batcher.flushInterval = time.Hour // would hang if batched

	ctx := WithSkipWriteBehind(context.Background())
	secCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{
		objects.FieldKeyKind:   "test",
		objects.FieldKeyID:     "test-id",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	if err := batcher.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.createCalls) != 1 {
		t.Fatalf("expected sync Create passthrough, got createCalls=%d bulkCalls=%d", len(mock.createCalls), len(mock.bulkCalls))
	}
	if len(mock.bulkCalls) != 0 {
		t.Fatalf("expected no BulkCreate on skip-write-behind, got %d", len(mock.bulkCalls))
	}
	enqueued, _ := batcher.GetBatchingStats()
	if enqueued != 0 {
		t.Fatalf("expected enqueued 0 on bypass, got %d", enqueued)
	}
}

func TestBatchingStorageProvider_BypassCLIOperation(t *testing.T) {
	mock := &mockStorageProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 100
	batcher.flushInterval = time.Hour

	ctx := WithCLIOperation(context.Background())
	secCtx := pkgctx.NewSystemSecurityContext()
	obj := map[string]any{
		objects.FieldKeyKind:   "test",
		objects.FieldKeyID:     "test-id",
		objects.FieldKeyStatus: objects.ObjectStatusActive,
	}
	if err := batcher.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	mock.mu.Lock()
	defer mock.mu.Unlock()
	if len(mock.createCalls) != 1 {
		t.Fatalf("expected sync Create passthrough, got createCalls=%d bulkCalls=%d", len(mock.createCalls), len(mock.bulkCalls))
	}
}

func TestBatchingStorageProvider_BulkCreatePartialFailureNoGhostACK(t *testing.T) {
	mock := &mockBulkPartialProvider{failIndexes: map[int]bool{1: true}}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 2
	batcher.flushInterval = time.Hour

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			obj := map[string]any{
				objects.FieldKeyKind: "test",
				objects.FieldKeyID:   "id",
				// no status → fail-closed if bulk ACKs without error index
			}
			errs[i] = batcher.Create(ctx, secCtx, obj)
		}(i)
	}
	wg.Wait()
	// BulkCreate fails index 1 and sets status on index 0 → one nil ACK, one forced error.
	var nilCount, forcedCount int
	for _, err := range errs {
		if err == nil {
			nilCount++
			continue
		}
		if strings.Contains(err.Error(), "forced fail") {
			forcedCount++
		}
	}
	if nilCount != 1 || forcedCount != 1 {
		t.Fatalf("expected 1 nil + 1 forced fail; errs=%v", errs)
	}
}

type mockBulkPartialProvider struct {
	ObjectStorageProvider
	failIndexes map[int]bool
}

func (m *mockBulkPartialProvider) Create(ctx context.Context, secCtx *pkgctx.SecurityContext, obj map[string]any) error {
	return nil
}

func (m *mockBulkPartialProvider) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objs []map[string]any) (*BulkResult, error) {
	res := &BulkResult{TotalCount: len(objs)}
	for i, obj := range objs {
		if m.failIndexes[i] {
			res.FailureCount++
			res.Errors = append(res.Errors, BulkOperationError{Index: i, Message: "forced fail"})
			continue
		}
		obj[objects.FieldKeyStatus] = objects.ObjectStatusActive
		res.SuccessCount++
		res.Results = append(res.Results, obj)
	}
	return res, nil // nil err with partial failures — the production bug shape
}

func TestBatchingStorageProvider_LifetimeCounters(t *testing.T) {
	mock := &mockStorageProvider{}
	batcher := NewBatchingObjectStorage(mock)
	batcher.batchSize = 3
	batcher.flushInterval = 10 * time.Millisecond

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			obj := map[string]any{
				objects.FieldKeyKind: "test",
				objects.FieldKeyID:   "test-id",
			}
			_ = batcher.Create(ctx, secCtx, obj)
		}()
	}
	wg.Wait()

	enqueued, flushed := batcher.GetBatchingStats()
	if enqueued != 3 {
		t.Errorf("Expected enqueued 3, got %d", enqueued)
	}
	if flushed != 3 {
		t.Errorf("Expected flushed 3, got %d", flushed)
	}
}
