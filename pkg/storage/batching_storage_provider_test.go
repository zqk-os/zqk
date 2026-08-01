package storage

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
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
	m.createCalls = append(m.createCalls, obj)
	return nil
}

func (m *mockStorageProvider) BulkCreate(ctx context.Context, secCtx *pkgctx.SecurityContext, objs []map[string]any) (*BulkResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.bulkCalls = append(m.bulkCalls, objs)
	return &BulkResult{Results: objs}, nil
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
