package cas

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
)

// mockBatchEventListener captures events emitted by a write queue instance.
type mockBatchEventListener struct {
	mu     sync.Mutex
	events []capturedBatchEvent
}

type capturedBatchEvent struct {
	Kind      string
	BatchSize int
	Status    string
	Err       error
}

func (m *mockBatchEventListener) OnListingIndexBatchEvent(
	ctx context.Context,
	projectRoot string,
	storageProvider CASFacade,
	kind string,
	batchSize int,
	duration time.Duration,
	status string,
	err error,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, capturedBatchEvent{
		Kind:      kind,
		BatchSize: batchSize,
		Status:    status,
		Err:       err,
	})
}

func (m *mockBatchEventListener) EventCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.events)
}

func (m *mockBatchEventListener) Events() []capturedBatchEvent {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]capturedBatchEvent, len(m.events))
	copy(copied, m.events)
	return copied
}

// TestListingIndexWriteQueue_ListenerInterface verifies the static contract and listener registration lifecycle.
func TestListingIndexWriteQueue_ListenerInterface(t *testing.T) {
	t.Parallel()

	q := NewListingIndexWriteQueueForTest()
	require.NotNil(t, q)

	listener := &mockBatchEventListener{}
	q.RegisterBatchEventListener(listener)

	listeners := q.getListeners()
	require.Len(t, listeners, 1)

	q.UnregisterBatchEventListener(listener)
	require.Empty(t, q.getListeners())
}

// TestListingIndexWriteQueue_OperationalProof verifies events are dispatched to instance listeners during batch processing.
func TestListingIndexWriteQueue_OperationalProof(t *testing.T) {
	tempDir := t.TempDir()
	kind := "test_operational_kind"
	kindDir := filepath.Join(tempDir, kind)
	casInstance := filecas.NewContentAddressableStorage(kindDir, kind)
	require.NotNil(t, casInstance)

	q := NewListingIndexWriteQueueForTest()
	q.SetProjectRoot(tempDir)

	listener := &mockBatchEventListener{}
	q.RegisterBatchEventListener(listener)

	// Enqueue an update
	done, err := q.EnqueueUpdateWithCallback(kind, "obj-123", "hash-abc", casInstance)
	require.NoError(t, err)

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for batch update")
	}

	// Verify listener received start and/or complete events
	require.Eventually(t, func() bool {
		return listener.EventCount() >= 1
	}, 3*time.Second, 50*time.Millisecond, "expected at least 1 batch event delivered to listener")

	var foundComplete bool
	for _, ev := range listener.Events() {
		if ev.Kind == kind && ev.Status == "complete" {
			foundComplete = true
			break
		}
	}
	require.True(t, foundComplete, "expected 'complete' batch event for kind %s", kind)
}

// TestListingIndexWriteQueue_ConcurrencySafety verifies isolated execution across multiple queue instances.
func TestListingIndexWriteQueue_ConcurrencySafety(t *testing.T) {
	t.Parallel()

	tempDir1 := t.TempDir()
	kind1 := "concurrent_kind_1"
	cas1 := filecas.NewContentAddressableStorage(filepath.Join(tempDir1, kind1), kind1)
	require.NotNil(t, cas1)

	tempDir2 := t.TempDir()
	kind2 := "concurrent_kind_2"
	cas2 := filecas.NewContentAddressableStorage(filepath.Join(tempDir2, kind2), kind2)
	require.NotNil(t, cas2)

	q1 := NewListingIndexWriteQueueForTest()
	q1.SetProjectRoot(tempDir1)
	l1 := &mockBatchEventListener{}
	q1.RegisterBatchEventListener(l1)

	q2 := NewListingIndexWriteQueueForTest()
	q2.SetProjectRoot(tempDir2)
	l2 := &mockBatchEventListener{}
	q2.RegisterBatchEventListener(l2)

	var wg sync.WaitGroup
	var completedCount atomic.Int32

	wg.Add(2)
	go func() {
		defer wg.Done()
		done, err := q1.EnqueueUpdateWithCallback(kind1, "obj-q1", "hash-q1", cas1)
		require.NoError(t, err)
		<-done
		completedCount.Add(1)
	}()

	go func() {
		defer wg.Done()
		done, err := q2.EnqueueUpdateWithCallback(kind2, "obj-q2", "hash-q2", cas2)
		require.NoError(t, err)
		<-done
		completedCount.Add(1)
	}()

	wg.Wait()
	require.Equal(t, int32(2), completedCount.Load())

	// Ensure l1 only observed kind1 events and l2 only observed kind2 events
	require.Eventually(t, func() bool {
		return l1.EventCount() >= 1 && l2.EventCount() >= 1
	}, 3*time.Second, 50*time.Millisecond)

	for _, ev := range l1.Events() {
		require.NotEqual(t, kind2, ev.Kind, "listener 1 cross-contaminated with kind 2")
	}
	for _, ev := range l2.Events() {
		require.NotEqual(t, kind1, ev.Kind, "listener 2 cross-contaminated with kind 1")
	}
}
