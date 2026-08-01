package objects

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/paths"
)

// TestSpecCacheUpdateBuffer_Batching verifies that updates are batched correctly
func TestSpecCacheUpdateBuffer_Batching(t *testing.T) {
	t.Parallel()

	// Create a buffer with small batch size for testing
	batchSize := 10
	buffer := NewSpecCacheUpdateBuffer(1000, batchSize, 100*time.Millisecond)

	// Create a test shard
	shard := &specShard{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buffer.Start(ctx)
	defer buffer.Stop()

	// Enqueue more updates than batch size
	numUpdates := 25
	keys := make([]string, numUpdates)
	for i := 0; i < numUpdates; i++ {
		key := fmt.Sprintf("test-key-%d", i)
		keys[i] = key
		update := &CacheUpdate{
			Type:      CacheUpdateSpec,
			Key:       key,
			Value:     fmt.Sprintf("test-value-%d", i),
			Shard:     shard,
			Timestamp: time.Now(),
		}
		buffer.Enqueue(update)
	}

	// Wait for batches to be processed (should get at least 2 batches of 10, plus remainder)
	time.Sleep(200 * time.Millisecond)

	// Verify all updates were applied by checking the cache
	for i, key := range keys {
		value, ok := shard.cache.Load(key)
		if !ok {
			t.Errorf("Update %d (key=%s) was not applied to cache", i, key)
			continue
		}
		expectedValue := fmt.Sprintf("test-value-%d", i)
		if value != expectedValue {
			t.Errorf("Update %d: expected value %s, got %v", i, expectedValue, value)
		}
	}
}

// TestSpecCacheUpdateBuffer_TimeBasedFlush verifies that batches are flushed on interval
func TestSpecCacheUpdateBuffer_TimeBasedFlush(t *testing.T) {
	t.Parallel()

	flushInterval := 50 * time.Millisecond
	buffer := NewSpecCacheUpdateBuffer(1000, 100, flushInterval)

	shard := &specShard{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buffer.Start(ctx)
	defer buffer.Stop()

	// Enqueue a small number of updates (less than batch size)
	numUpdates := 5
	keys := make([]string, numUpdates)
	for i := 0; i < numUpdates; i++ {
		key := fmt.Sprintf("test-key-%d", i)
		keys[i] = key
		update := &CacheUpdate{
			Type:      CacheUpdateSpec,
			Key:       key,
			Value:     fmt.Sprintf("test-value-%d", i),
			Shard:     shard,
			Timestamp: time.Now(),
		}
		buffer.Enqueue(update)
	}

	// Wait for time-based flush
	time.Sleep(flushInterval * 2)

	// Verify all updates were applied (proves time-based flush worked)
	for i, key := range keys {
		_, ok := shard.cache.Load(key)
		if !ok {
			t.Errorf("Update %d (key=%s) was not flushed by time interval", i, key)
		}
	}
}

// TestSpecCacheUpdateBuffer_HighContention simulates high contention scenario
func TestSpecCacheUpdateBuffer_HighContention(t *testing.T) {
	t.Parallel()

	buffer := NewSpecCacheUpdateBuffer(10000, 100, 10*time.Millisecond)

	shard := &specShard{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buffer.Start(ctx)
	defer buffer.Stop()

	// Simulate high contention: many goroutines enqueueing updates simultaneously
	numGoroutines := 100
	updatesPerGoroutine := 50
	var wg sync.WaitGroup
	var updatesEnqueued int64

	startTime := time.Now()
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("objects_test", "high contention enqueue").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				for j := 0; j < updatesPerGoroutine; j++ {
					key := fmt.Sprintf("test-key-%d-%d", id, j)
					update := &CacheUpdate{
						Type:      CacheUpdateSpec,
						Key:       key,
						Value:     fmt.Sprintf("test-value-%d-%d", id, j),
						Shard:     shard,
						Timestamp: time.Now(),
					}
					buffer.Enqueue(update)
					atomic.AddInt64(&updatesEnqueued, 1)
				}
			}(i)
		})
	}

	wg.Wait()
	enqueueDuration := time.Since(startTime)

	// Wait for all batches to be processed
	time.Sleep(500 * time.Millisecond)

	totalUpdates := int64(numGoroutines * updatesPerGoroutine)
	if atomic.LoadInt64(&updatesEnqueued) != totalUpdates {
		t.Errorf("Expected %d updates enqueued, got %d", totalUpdates, atomic.LoadInt64(&updatesEnqueued))
	}

	// Verify all updates were applied to cache
	var missingCount int64
	for i := 0; i < numGoroutines; i++ {
		for j := 0; j < updatesPerGoroutine; j++ {
			key := fmt.Sprintf("test-key-%d-%d", i, j)
			_, ok := shard.cache.Load(key)
			if !ok {
				atomic.AddInt64(&missingCount, 1)
			}
		}
	}

	if missingCount > 0 {
		t.Errorf("Expected all updates to be applied, but %d are missing", atomic.LoadInt64(&missingCount))
	}

	t.Logf("Enqueued %d updates in %v, all applied successfully", totalUpdates, enqueueDuration)
}

// TestSpecCacheUpdateBuffer_ShardGrouping verifies that updates are grouped by shard
func TestSpecCacheUpdateBuffer_ShardGrouping(t *testing.T) {
	t.Parallel()

	buffer := NewSpecCacheUpdateBuffer(1000, 100, 10*time.Millisecond)

	// Create multiple shards
	shard1 := &specShard{}
	shard2 := &specShard{}
	shard3 := &specShard{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buffer.Start(ctx)
	defer buffer.Stop()

	// Enqueue updates for different shards
	numUpdates := 30
	for i := 0; i < numUpdates; i++ {
		var shard *specShard
		switch i % 3 {
		case 0:
			shard = shard1
		case 1:
			shard = shard2
		case 2:
			shard = shard3
		}
		key := fmt.Sprintf("test-key-%d", i)
		update := &CacheUpdate{
			Type:      CacheUpdateSpec,
			Key:       key,
			Value:     fmt.Sprintf("test-value-%d", i),
			Shard:     shard,
			Timestamp: time.Now(),
		}
		buffer.Enqueue(update)
	}

	// Wait for batches to be processed
	time.Sleep(100 * time.Millisecond)

	// Verify all updates were applied to correct shards
	for i := 0; i < numUpdates; i++ {
		key := fmt.Sprintf("test-key-%d", i)
		var shard *specShard
		switch i % 3 {
		case 0:
			shard = shard1
		case 1:
			shard = shard2
		case 2:
			shard = shard3
		}
		value, ok := shard.cache.Load(key)
		if !ok {
			t.Errorf("Update %d (key=%s) was not applied to correct shard", i, key)
			continue
		}
		expectedValue := fmt.Sprintf("test-value-%d", i)
		if value != expectedValue {
			t.Errorf("Update %d: expected value %s, got %v", i, expectedValue, value)
		}
	}
}

// TestSpecCacheUpdateBuffer_ConcurrentEnqueue verifies thread-safety of Enqueue
func TestSpecCacheUpdateBuffer_ConcurrentEnqueue(t *testing.T) {
	t.Parallel()

	buffer := NewSpecCacheUpdateBuffer(10000, 100, 10*time.Millisecond)

	shard := &specShard{}
	var updatesEnqueued int64

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	buffer.Start(ctx)
	defer buffer.Stop()

	// Many goroutines enqueueing concurrently
	numGoroutines := 200
	updatesPerGoroutine := 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("objects_test", "concurrent enqueue").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < updatesPerGoroutine; j++ {
				update := &CacheUpdate{
					Type:      CacheUpdateSpec,
					Key:       "test-key",
					Value:     "test-value",
					Shard:     shard,
					Timestamp: time.Now(),
				}
				buffer.Enqueue(update)
				atomic.AddInt64(&updatesEnqueued, 1)
			}
		})
	}

	// Wait for all enqueues to complete
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("objects_test", "wait for completion").StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		// All enqueues completed successfully
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for concurrent enqueues")
	}

	expectedUpdates := int64(numGoroutines * updatesPerGoroutine)
	if atomic.LoadInt64(&updatesEnqueued) != expectedUpdates {
		t.Errorf("Expected %d updates enqueued, got %d", expectedUpdates, atomic.LoadInt64(&updatesEnqueued))
	}

	// Wait for batches to be processed
	time.Sleep(200 * time.Millisecond)
}

// TestSpecCacheUpdateBuffer_IntegrationWithSpecLoader tests integration with actual spec loading
func TestSpecCacheUpdateBuffer_IntegrationWithSpecLoader(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	specsDir := tmpDir + "/specs"
	if err := os.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create specs dir: %v", err)
	}

	// Create many spec files to trigger cache updates
	numSpecs := 50
	for i := 0; i < numSpecs; i++ {
		specFile := filepath.Join(specsDir, fmt.Sprintf("spec%d.yaml", i))
		content := fmt.Sprintf(`ontology: spec%d
schema_version: "1.0"
fields:
  field1:
    type: string
`, i)
		if err := os.WriteFile(specFile, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("Failed to write spec file: %v", err)
		}
	}

	loader := NewSpecLoader(specsDir)

	// Track cache updates by monitoring when specs are loaded
	var loadsCompleted int64
	var wg sync.WaitGroup

	// Concurrently load specs (this will trigger cache updates through the buffer)
	numGoroutines := 20
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("objects_test", "concurrent spec load").StartSimple(func() {
			defer wg.Done()
			// Each goroutine loads all specs
			for j := 0; j < numSpecs; j++ {
				specFile := fmt.Sprintf("spec%d.yaml", j)
				_, err := loader.LoadSpecWithInheritance(specFile)
				if err != nil {
					t.Errorf("Failed to load spec %s: %v", specFile, err)
					return
				}
				atomic.AddInt64(&loadsCompleted, 1)
			}
		})
	}

	// Wait for all loads to complete
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("objects_test", "wait for completion").StartSimple(func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		// All loads completed successfully
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for concurrent spec loads")
	}

	expectedLoads := int64(numGoroutines * numSpecs)
	if atomic.LoadInt64(&loadsCompleted) != expectedLoads {
		t.Errorf("Expected %d loads completed, got %d", expectedLoads, atomic.LoadInt64(&loadsCompleted))
	}

	// Wait for all cache updates to be applied by the write-behind buffer
	time.Sleep(200 * time.Millisecond)

	// Verify specs are cached (second load should be faster and return same pointer)
	for i := 0; i < numSpecs; i++ {
		specFile := fmt.Sprintf("spec%d.yaml", i)
		spec1, err := loader.LoadSpecWithInheritance(specFile)
		if err != nil {
			t.Fatalf("Failed to load spec %s: %v", specFile, err)
		}
		spec2, err := loader.LoadSpecWithInheritance(specFile)
		if err != nil {
			t.Fatalf("Failed to load spec %s: %v", specFile, err)
		}
		if spec1 != spec2 {
			t.Errorf("Expected cached spec for %s (same pointer), got different pointers", specFile)
		}
	}
}
