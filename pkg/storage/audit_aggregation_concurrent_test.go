package storage

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestAuditAggregationService_ConcurrentAggregateEvents tests that aggregateEvents
// can be called concurrently without causing concurrent map writes when using builders
func TestAuditAggregationService_ConcurrentAggregateEvents(t *testing.T) {
	// Create a temporary test directory with process structure (use project root without /001 so object_specs can be copied)
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)

	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	t.Cleanup(func() {
		var fos *FileObjectStorage
		if s, ok := storageFactory.GetStorage().(*FileObjectStorage); ok {
			fos = s
		}
		_ = RunProjectTestTeardown(TempProjectTeardown(tmpDir, fos))
	})
	storageProvider := storageFactory.GetStorage()

	// Create aggregation service
	service := NewAuditAggregationService(storageProvider)

	// Create test events
	events := []map[string]any{
		{
			objects.FieldKeyID:         "AUD-001",
			objects.FieldKeyEventType:  "object_creation",
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeyOperation:  "create",
		},
		{
			objects.FieldKeyID:         "AUD-002",
			objects.FieldKeyEventType:  "object_update",
			objects.FieldKeyTargetKind: "backlog_item",
			objects.FieldKeyOperation:  "update",
		},
	}

	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	var wg sync.WaitGroup
	concurrency := 20
	errors := make(chan error, concurrency)

	// Launch many goroutines that all aggregate events concurrently
	for i := 0; i < concurrency; i++ {
		i := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_aggregate_events_%d", i), fmt.Sprintf("aggregating events %d in concurrent test", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				ctx := context.Background()
				secCtx := pkgctx.NewSystemSecurityContext()
				_, _, _ = service.aggregateEvents(ctx, secCtx, events, windowStart, windowEnd)
				// Ignore return values - we're just testing for panics/race conditions
			})
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for errors (should be empty - we're testing for panics, not errors)
	for err := range errors {
		if err != nil {
			t.Errorf("aggregateEvents failed: %v", err)
		}
	}
}

// TestChangeJournalAggregationService_ConcurrentAggregateEntries tests that aggregateEntries
// can be called concurrently without causing concurrent map writes
func TestChangeJournalAggregationService_ConcurrentAggregateEntries(t *testing.T) {
	// Create a temporary test directory with process structure (use project root without /001 so object_specs can be copied)
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)

	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	t.Cleanup(func() {
		var fos *FileObjectStorage
		if s, ok := storageFactory.GetStorage().(*FileObjectStorage); ok {
			fos = s
		}
		_ = RunProjectTestTeardown(TempProjectTeardown(tmpDir, fos))
	})
	storageProvider := storageFactory.GetStorage()

	// Create aggregation service
	service := NewChangeJournalAggregationService(storageProvider)

	// Create test entries
	entries := []map[string]any{
		{
			objects.FieldKeyID:         "CJE-001",
			objects.FieldKeyChangeType: "create",
			objects.FieldKeyObjectRef:  "backlog_item:BLI-001",
		},
		{
			objects.FieldKeyID:         "CJE-002",
			objects.FieldKeyChangeType: "update",
			objects.FieldKeyObjectRef:  "backlog_item:BLI-002",
		},
	}

	windowStart := time.Now().Add(-1 * time.Hour)
	windowEnd := time.Now()

	var wg sync.WaitGroup
	concurrency := 20
	errors := make(chan error, concurrency)

	// Launch many goroutines that all aggregate entries concurrently
	for i := 0; i < concurrency; i++ {
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_aggregate_entries_%d", i), fmt.Sprintf("aggregating entries %d in concurrent test", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				ctx := context.Background()
				secCtx := pkgctx.NewSystemSecurityContext()
				_, _, _ = service.aggregateEntries(ctx, secCtx, entries, windowStart, windowEnd)
				// Ignore return values - we're just testing for panics/race conditions
			})
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(errors)

	// Check for errors (should be empty - we're testing for panics, not errors)
	for err := range errors {
		if err != nil {
			t.Errorf("aggregateEntries failed: %v", err)
		}
	}
}

// TestCreateChangeJournalEntryWithBuilder_Concurrent tests that CreateChangeJournalEntryWithBuilder
// can be called concurrently without causing concurrent map writes
func TestCreateChangeJournalEntryWithBuilder_Concurrent(t *testing.T) {
	// Create a temporary test directory with process structure (use project root without /001 so object_specs can be copied)
	tmpDir := t.TempDir()
	CopyObjectSpecsFromModuleOrSkip(t, tmpDir)

	// Create storage (will use process directory)
	storageFactory, err := NewStorageFactory(context.Background(), tmpDir)
	if err != nil {
		t.Fatalf("Failed to create storage factory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()
	t.Cleanup(func() {
		var fos *FileObjectStorage
		if s, ok := storageProvider.(*FileObjectStorage); ok {
			fos = s
		}
		_ = RunProjectTestTeardown(TempProjectTeardown(tmpDir, fos))
	})

	secCtx := pkgctx.NewSystemSecurityContext()

	var wg sync.WaitGroup
	concurrency := 30

	// Launch many goroutines that all create change journal entries concurrently
	for i := 0; i < concurrency; i++ {
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_create_cje_%d", i), fmt.Sprintf("creating change journal entry %d in concurrent test", i)).
			WithWaitGroup(&wg).
			StartSimple(func() {
				func(index int) {
					options := &ChangeJournalEntryOptions{
						ChangeType:  "test",
						ObjectRef:   "test:TEST-001",
						DiffSummary: "test change",
					}

					err := CreateChangeJournalEntryWithBuilder(
						context.Background(),
						tmpDir,
						secCtx,
						storageProvider,
						options,
					)
					// Ignore errors - we're testing for panics, not functionality
					_ = err
				}(i)
			})
	}

	// Wait for all goroutines to complete before TempDir cleanup / factory shutdown.
	wg.Wait()
}
