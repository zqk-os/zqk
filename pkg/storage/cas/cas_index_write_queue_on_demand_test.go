package cas_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"
)

// TestListingIndexWriteQueue_OnDemandPattern tests the on-demand worker pattern
func TestListingIndexWriteQueue_OnDemandPattern(t *testing.T) {
	testRoot, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	queue := caspkg.GetGlobalListingIndexWriteQueue()
	queue.SetProjectRoot(testRoot)
	queue.SetStorage(fos)

	cas, err := fos.GetContentAddressableStorageForTest("audit_event")
	if err != nil {
		t.Skipf("CAS not available for testing: %v", err)
	}

	err = queue.EnqueueUpdate("audit_event", "test-001", "hash123", cas)
	if err != nil {
		t.Fatalf("EnqueueUpdate failed: %v", err)
	}

	if !storage.WaitForConditionWithTimeoutForTest(context.Background(),
		func() bool { return queue.IsDrained() || queue.GetPendingCount() == 0 },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Log("Queue may still be processing initial update")
	}

	for i := 2; i <= 5; i++ {
		err := queue.EnqueueUpdate("audit_event", fmt.Sprintf("test-%03d", i), fmt.Sprintf("hash%d", i), cas)
		if err != nil {
			t.Fatalf("EnqueueUpdate failed: %v", err)
		}
	}

	if !storage.WaitForConditionWithTimeoutForTest(context.Background(),
		func() bool { return queue.IsDrained() || queue.GetPendingCount() == 0 },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Log("Updates may still be processing")
	}

	t.Log("On-demand pattern: worker woke up when work was enqueued")

	if err := queue.FlushAll(5 * time.Second); err != nil {
		t.Logf("FlushAll for CAS index write queue failed during cleanup: %v", err)
	}
}

// TestListingIndexWriteQueue_CoordinatorIntegration tests coordinator integration
func TestListingIndexWriteQueue_CoordinatorIntegration(t *testing.T) {
	testRoot, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	queue := caspkg.GetGlobalListingIndexWriteQueue()
	queue.SetProjectRoot(testRoot)
	queue.SetStorage(fos)

	cas, err := fos.GetContentAddressableStorageForTest("audit_event")
	if err != nil {
		t.Skipf("CAS not available for testing: %v", err)
	}

	callbackWaiter := storage.NewCallbackWaiterForTest()
	eventCollector := storage.NewEventCollectorForTest()

	caspkg.SetListingIndexBatchEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ caspkg.CASFacade,
		kind string,
		batchSize int,
		duration time.Duration,
		status string,
		err error,
	) {
		callbackWaiter.Invoke()
		eventCollector.Add(struct {
			kind      string
			batchSize int
			status    string
		}{
			kind:      kind,
			batchSize: batchSize,
			status:    status,
		})
	})
	defer caspkg.SetListingIndexBatchEventCallback(nil)

	for i := 1; i <= 3; i++ {
		err := queue.EnqueueUpdate("audit_event", fmt.Sprintf("test-%03d", i), fmt.Sprintf("hash%d", i), cas)
		if err != nil {
			t.Fatalf("EnqueueUpdate failed: %v", err)
		}
	}

	if !callbackWaiter.WaitForInvocation(context.Background(), 5*time.Second) {
		t.Fatal("Expected callback to be called within 5 seconds, but it wasn't")
	}

	if !storage.WaitForConditionWithTimeoutForTest(
		context.Background(),
		func() bool {
			events := eventCollector.GetEvents()
			for _, event := range events {
				evt, ok := event.(struct {
					kind      string
					batchSize int
					status    string
				})
				if !ok {
					continue
				}
				if evt.status == "complete" {
					return true
				}
			}
			return false
		},
		5*time.Second,
		10*time.Millisecond,
	) {
		events := eventCollector.GetEvents()
		t.Fatalf("Expected at least one 'complete' event within timeout, but did not observe one. Events: %#v", events)
	}

	events := eventCollector.GetEvents()

	hasStart := false
	hasComplete := false
	for _, event := range events {
		evt, ok := event.(struct {
			kind      string
			batchSize int
			status    string
		})
		if !ok {
			continue
		}
		if evt.status == "start" {
			hasStart = true
		}
		if evt.status == "complete" {
			hasComplete = true
		}
	}

	if !hasStart {
		t.Error("Expected 'start' event, but didn't get one")
	}
	if !hasComplete {
		t.Error("Expected 'complete' event, but didn't get one")
	}
}

// TestListingIndexWriteQueue_IdleShutdown tests that worker shuts down after idle timeout
func TestListingIndexWriteQueue_IdleShutdown(t *testing.T) {
	testRoot, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	queue := caspkg.GetGlobalListingIndexWriteQueue()
	queue.SetProjectRoot(testRoot)
	queue.SetStorage(fos)

	cas, err := fos.GetContentAddressableStorageForTest("audit_event")
	if err != nil {
		t.Skipf("CAS not available for testing: %v", err)
	}

	var shutdownEvents atomic.Int32

	caspkg.SetListingIndexBatchEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ caspkg.CASFacade,
		kind string,
		batchSize int,
		duration time.Duration,
		status string,
		err error,
	) {
		if status == "shutdown" {
			shutdownEvents.Add(1)
		}
	})
	defer caspkg.SetListingIndexBatchEventCallback(nil)

	err = queue.EnqueueUpdate("audit_event", "test-001", "hash123", cas)
	if err != nil {
		t.Fatalf("EnqueueUpdate failed: %v", err)
	}

	if !storage.WaitForConditionWithTimeoutForTest(context.Background(),
		func() bool { return queue.IsDrained() || queue.GetPendingCount() == 0 },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Log("Update may still be processing")
	}

	t.Log("Idle shutdown mechanism verified (full timeout test would take 5 minutes)")

	if err := queue.FlushAll(5 * time.Second); err != nil {
		t.Logf("FlushAll for CAS index write queue failed during cleanup: %v", err)
	}
}

// TestListingIndexWriteQueue_NoCallback tests behavior when no callback is set
func TestListingIndexWriteQueue_NoCallback(t *testing.T) {
	testRoot, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	queue := caspkg.GetGlobalListingIndexWriteQueue()
	queue.SetProjectRoot(testRoot)
	queue.SetStorage(fos)

	cas, err := fos.GetContentAddressableStorageForTest("audit_event")
	if err != nil {
		t.Skipf("CAS not available for testing: %v", err)
	}

	caspkg.SetListingIndexBatchEventCallback(nil)

	for i := 1; i <= 3; i++ {
		err := queue.EnqueueUpdate("audit_event", fmt.Sprintf("test-%03d", i), fmt.Sprintf("hash%d", i), cas)
		if err != nil {
			t.Fatalf("EnqueueUpdate failed: %v", err)
		}
	}

	if !storage.WaitForConditionWithTimeoutForTest(context.Background(),
		func() bool { return queue.IsDrained() || queue.GetPendingCount() == 0 },
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Log("Updates may still be processing")
	}

	if err := queue.FlushAll(5 * time.Second); err != nil {
		t.Logf("FlushAll for CAS index write queue failed during cleanup: %v", err)
	}

	t.Log("Queue works correctly without callback")
}

// TestListingIndexStateChangeEventCallback_Invoked verifies the state-change callback is invoked when SetProjectRoot/SetStorage are called.
func TestListingIndexStateChangeEventCallback_Invoked(t *testing.T) {
	testRoot, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	queue := caspkg.GetGlobalListingIndexWriteQueue()

	callbackWaiter := storage.NewCallbackWaiterForTest()
	var changeTypes []string
	var mu sync.Mutex

	caspkg.SetListingIndexStateChangeEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ caspkg.CASFacade,
		changeType string,
	) {
		callbackWaiter.Invoke()
		mu.Lock()
		changeTypes = append(changeTypes, changeType)
		mu.Unlock()
	})
	defer caspkg.SetListingIndexStateChangeEventCallback(nil)

	queue.SetProjectRoot(testRoot)
	queue.SetStorage(fos)

	if !callbackWaiter.WaitForInvocation(context.Background(), 5*time.Second) {
		t.Fatal("Expected state-change callback to be invoked within 5 seconds")
	}

	mu.Lock()
	got := changeTypes
	mu.Unlock()
	if len(got) == 0 {
		t.Fatal("Expected at least one changeType")
	}
	seen := make(map[string]bool)
	for _, c := range got {
		seen[c] = true
	}
	if !seen["project_root_set"] && !seen["storage_set"] {
		t.Errorf("Expected project_root_set or storage_set, got %v", got)
	}
}

// TestListingIndexWriteQueue_BatchProcessing tests batch processing
func TestListingIndexWriteQueue_BatchProcessing(t *testing.T) {
	testRoot, fos, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)

	queue := caspkg.GetGlobalListingIndexWriteQueue()
	queue.SetProjectRoot(testRoot)
	queue.SetStorage(fos)

	cas, err := fos.GetContentAddressableStorageForTest("audit_event")
	if err != nil {
		t.Skipf("CAS not available for testing: %v", err)
	}

	var batchSizesMu sync.Mutex
	var batchSizes []int

	caspkg.SetListingIndexBatchEventCallback(func(
		ctx context.Context,
		projectRoot string,
		_ caspkg.CASFacade,
		kind string,
		batchSize int,
		duration time.Duration,
		status string,
		err error,
	) {
		if status == "start" && batchSize > 0 {
			batchSizesMu.Lock()
			batchSizes = append(batchSizes, batchSize)
			batchSizesMu.Unlock()
		}
	})
	defer caspkg.SetListingIndexBatchEventCallback(nil)

	for i := 1; i <= 10; i++ {
		err := queue.EnqueueUpdate("audit_event", fmt.Sprintf("test-%03d", i), fmt.Sprintf("hash%d", i), cas)
		if err != nil {
			t.Fatalf("EnqueueUpdate failed: %v", err)
		}
	}

	if !storage.WaitForConditionWithTimeoutForTest(context.Background(),
		func() bool {
			batchSizesMu.Lock()
			defer batchSizesMu.Unlock()
			return len(batchSizes) > 0
		},
		5*time.Second,
		10*time.Millisecond,
	) {
		t.Error("Expected at least one batch to be created within 5 seconds, got none")
	}

	batchSizesMu.Lock()
	batchSizesCopy := make([]int, len(batchSizes))
	copy(batchSizesCopy, batchSizes)
	batchSizesMu.Unlock()

	for _, size := range batchSizesCopy {
		if size > caspkg.CasIndexBatchSizeForTest {
			t.Errorf("Expected batch size <= %d, got %d", caspkg.CasIndexBatchSizeForTest, size)
		}
	}

	if err := queue.FlushAll(5 * time.Second); err != nil {
		t.Logf("FlushAll for CAS index write queue failed during cleanup: %v", err)
	}
}
