package cas

import (
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

type testOperationCallback struct {
	mu             sync.Mutex
	startEvents    int
	completeEvents int
	errorEvents    int
}

func (t *testOperationCallback) OnStart(operationID string, metadata map[string]any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.startEvents++
}

func (t *testOperationCallback) OnProgress(operationID string, progress int, total int, message string) {
}

func (t *testOperationCallback) OnComplete(operationID string, result any, duration time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.completeEvents++
}

func (t *testOperationCallback) OnError(operationID string, err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.errorEvents++
}

func (t *testOperationCallback) OnCancel(operationID string, reason string) {}

func TestListingIndexWriteQueue_OperationCallback(t *testing.T) {
	tmpDir := t.TempDir()
	kind := "callback_test_kind"
	objectID := "OBJ-CALLBACK-1"
	hash := "hash-callback-1"

	casQueue := NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := filecas.NewContentAddressableStorage(tmpDir, kind, casQueue)
	queue := GetGlobalListingIndexWriteQueue()

	recorder := &testOperationCallback{}

	done, err := queue.EnqueueUpdateWithOperationCallback(kind, objectID, hash, "", cas, recorder)
	if err != nil {
		t.Fatalf("failed to enqueue update with callback: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("index update failed: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout waiting for index update completion")
	}

	recorder.mu.Lock()
	startEvents := recorder.startEvents
	completeEvents := recorder.completeEvents
	errorEvents := recorder.errorEvents
	recorder.mu.Unlock()

	if startEvents != 1 {
		t.Fatalf("expected 1 start event, got %d", startEvents)
	}

	if completeEvents != 1 {
		t.Fatalf("expected 1 complete event, got %d", completeEvents)
	}

	if errorEvents != 0 {
		t.Fatalf("expected 0 error events, got %d", errorEvents)
	}
}

func TestListingIndexWriteQueue_StatsCounters(t *testing.T) {
	q := NewListingIndexWriteQueueForTest()
	defer q.Shutdown()
	stats := q.GetQueueStats()
	if stats["queue_count"] == nil {
		t.Fatalf("Expected queue_count key in stats map")
	}

	queues, ok := stats["queues"].(map[string]any)
	if !ok {
		t.Fatalf("Expected queues map in stats")
	}

	for _, detailAny := range queues {
		detail, ok := detailAny.(map[string]any)
		if !ok {
			continue
		}
		if _, exists := detail["total_enqueued"]; !exists {
			t.Errorf("Expected total_enqueued in queue stats detail")
		}
		if _, exists := detail["total_processed"]; !exists {
			t.Errorf("Expected total_processed in queue stats detail")
		}
	}
}

func TestListingIndexWriteQueue_StatsCounters_Delta(t *testing.T) {
	q := NewListingIndexWriteQueueForTest()
	defer q.Shutdown()

	kind := "test_kind_delta"
	_ = q.EnqueueUpdate(kind, "obj1", "hash1", nil)

	stats := q.GetQueueStats()
	queues, ok := stats["queues"].(map[string]any)
	if !ok {
		t.Fatalf("Expected queues map in stats")
	}

	detailAny, exists := queues[kind]
	if !exists {
		t.Fatalf("Expected kind %s in queue stats", kind)
	}

	detail, ok := detailAny.(map[string]any)
	if !ok {
		t.Fatalf("Expected map detail for kind %s", kind)
	}

	enqueued, ok := detail["total_enqueued"].(int64)
	if !ok || enqueued < 1 {
		t.Errorf("Expected total_enqueued >= 1 after enqueue, got %v", detail["total_enqueued"])
	}
}

func TestListingIndexWriteQueue_StatsCounters_ProcessedDelta(t *testing.T) {
	q := NewListingIndexWriteQueueForTest()
	defer q.Shutdown()

	kind := "test_kind_processed"
	done, err := q.EnqueueUpdateWithCallback(kind, "obj2", "hash2", nil)
	if err == nil && done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}

	stats := q.GetQueueStats()
	queues, ok := stats["queues"].(map[string]any)
	if !ok {
		t.Fatalf("Expected queues map in stats")
	}

	detailAny, exists := queues[kind]
	if !exists {
		t.Fatalf("Expected kind %s in queue stats", kind)
	}

	detail, ok := detailAny.(map[string]any)
	if !ok {
		t.Fatalf("Expected map detail for kind %s", kind)
	}

	processed, ok := detail["total_processed"].(int64)
	if !ok || processed < 1 {
		t.Errorf("Expected total_processed >= 1 after worker batch processing, got %v", detail["total_processed"])
	}
}
