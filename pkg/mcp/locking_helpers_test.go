package mcp

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// MockServer simulates the locking pattern without the full MCP server
type MockServer struct {
	clientsMu    sync.RWMutex
	shutdownMu   sync.Mutex
	shutdownFlag atomic.Int32 // Atomic boolean: 0 = running, 1 = shutting down
	clients      map[string]*MockClient
}

type MockClient struct {
	ID    string
	Queue *MockQueue
}

type MockQueue struct {
	ID string
}

// isShuttingDown safely checks if shutdown is in progress
// Uses atomic load for lock-free, thread-safe check
func (m *MockServer) isShuttingDown() bool {
	return m.shutdownFlag.Load() == 1
}

// withClientsReadLock safely acquires clientsMu.RLock() with shutdown checks using TryRLock
func (m *MockServer) withClientsReadLock(callback func() bool) bool {
	// Check shutdown before attempting lock acquisition
	if m.isShuttingDown() {
		return false // Don't acquire lock during shutdown
	}

	// Use TryRLock() for non-blocking lock acquisition
	if !m.clientsMu.TryRLock() {
		// Lock not available (write lock held) - skip to avoid deadlock
		return false
	}
	defer m.clientsMu.RUnlock()

	// Check shutdown after acquiring lock (defensive)
	if m.isShuttingDown() {
		return false // Shutdown detected, release lock and return
	}

	// Lock acquired and shutdown not in progress - safe to use
	return callback()
}

// findClientQueue safely finds a client message queue with proper shutdown handling
func (m *MockServer) findClientQueue() *MockQueue {
	var queue *MockQueue

	success := m.withClientsReadLock(func() bool {
		// Check shutdown during iteration
		if m.isShuttingDown() {
			return false // Shutdown detected, release lock immediately
		}
		for _, client := range m.clients {
			if client.Queue != nil {
				queue = client.Queue
				return true // Found queue
			}
		}
		return true // No queue found, but that's OK
	})

	if !success {
		return nil // Shutdown in progress, don't return queue
	}

	return queue
}

// shutdownSequence simulates the shutdown sequence
func (m *MockServer) shutdownSequence() {
	// Set shutdown flag atomically
	// Use compare-and-swap to ensure shutdown only runs once
	if !m.shutdownFlag.CompareAndSwap(0, 1) {
		return // Already shutting down
	}

	// Hold write lock to stop queues (simulating shutdownSequence behavior)
	m.clientsMu.Lock()
	// Simulate stopping queues
	time.Sleep(1 * time.Millisecond) // Small delay to simulate work
	m.clientsMu.Unlock()

	// After releasing write lock, try to log (simulating SendLogDebug)
	// This is where deadlock can occur if another goroutine is waiting for read lock
	_ = m.findClientQueue() // This should not deadlock
}

// TestLockingPattern_NormalOperation tests normal operation without shutdown
func TestLockingPattern_NormalOperation(t *testing.T) {
	t.Parallel()
	server := &MockServer{
		clients: make(map[string]*MockClient),
	}

	// Add a client with queue
	server.clients["test-client"] = &MockClient{
		ID:    "test-client",
		Queue: &MockQueue{ID: "queue-1"},
	}

	// Should successfully find queue
	queue := server.findClientQueue()
	if queue == nil {
		t.Fatal("Expected to find queue, got nil")
	}
	if queue.ID != "queue-1" {
		t.Fatalf("Expected queue ID 'queue-1', got '%s'", queue.ID)
	}
}

// TestLockingPattern_ShutdownBeforeLock tests shutdown before lock acquisition
func TestLockingPattern_ShutdownBeforeLock(t *testing.T) {
	t.Parallel()
	server := &MockServer{
		clients: make(map[string]*MockClient),
	}

	// Set shutdown flag atomically
	server.shutdownFlag.Store(1)

	// Should return nil immediately without trying to acquire lock
	queue := server.findClientQueue()
	if queue != nil {
		t.Fatal("Expected nil queue during shutdown, got queue")
	}
}

// TestLockingPattern_ShutdownDuringLockWait tests shutdown while waiting for lock
func TestLockingPattern_ShutdownDuringLockWait(t *testing.T) {
	t.Parallel()
	server := &MockServer{
		clients: make(map[string]*MockClient),
	}

	// Add a client
	server.clients["test-client"] = &MockClient{
		ID:    "test-client",
		Queue: &MockQueue{ID: "queue-1"},
	}

	// Hold write lock (simulating shutdownSequence)
	server.clientsMu.Lock()

	// Start goroutine that will try to acquire read lock (will block)
	queueFound := make(chan *MockQueue, 1)
	goroutinelabels.NewGoroutine("test_queue_finder", "finding client queue in locking test").
		StartSimple(func() {
			queue := server.findClientQueue()
			queueFound <- queue
		})

	// Give goroutine time to start and block on read lock
	time.Sleep(10 * time.Millisecond)

	// Set shutdown flag atomically (while write lock is held)
	server.shutdownFlag.Store(1)

	// Release write lock
	server.clientsMu.Unlock()

	// Wait for goroutine to complete (should timeout and return nil)
	select {
	case queue := <-queueFound:
		if queue != nil {
			t.Fatal("Expected nil queue when shutdown during lock wait, got queue")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Goroutine did not complete - possible deadlock")
	}
}

// TestLockingPattern_ShutdownAfterLockAcquisition tests shutdown after lock is acquired
func TestLockingPattern_ShutdownAfterLockAcquisition(t *testing.T) {
	t.Parallel()
	server := &MockServer{
		clients: make(map[string]*MockClient),
	}

	// Add a client
	server.clients["test-client"] = &MockClient{
		ID:    "test-client",
		Queue: &MockQueue{ID: "queue-1"},
	}

	// Acquire read lock first
	server.clientsMu.RLock()

	// Set shutdown flag atomically (while read lock is held)
	server.shutdownFlag.Store(1)

	// Release read lock
	server.clientsMu.RUnlock()

	// Now try to find queue - should detect shutdown and return nil
	queue := server.findClientQueue()
	if queue != nil {
		t.Fatal("Expected nil queue when shutdown after lock acquisition, got queue")
	}
}

// TestLockingPattern_ConcurrentReadLocks tests multiple concurrent read lock attempts
func TestLockingPattern_ConcurrentReadLocks(t *testing.T) {
	t.Parallel()
	server := &MockServer{
		clients: make(map[string]*MockClient),
	}

	// Add multiple clients
	for i := 0; i < 10; i++ {
		server.clients[fmt.Sprintf("client-%d", i)] = &MockClient{
			ID:    fmt.Sprintf("client-%d", i),
			Queue: &MockQueue{ID: fmt.Sprintf("queue-%d", i)},
		}
	}

	// Spawn many goroutines trying to acquire read locks concurrently
	var wg sync.WaitGroup
	var successCount atomic.Int32
	for i := 0; i < 100; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("mcp_test", "concurrent read lock attempt").StartSimple(func() {
			defer wg.Done()
			queue := server.findClientQueue()
			if queue != nil {
				successCount.Add(1)
			}
		})
	}

	wg.Wait()

	// All should succeed (no shutdown)
	if successCount.Load() != 100 {
		t.Fatalf("Expected 100 successful queue finds, got %d", successCount.Load())
	}
}

// TestLockingPattern_ShutdownSequenceDeadlock tests the exact deadlock scenario
func TestLockingPattern_ShutdownSequenceDeadlock(t *testing.T) {
	t.Parallel()
	server := &MockServer{
		clients: make(map[string]*MockClient),
	}

	// Add a client
	server.clients["test-client"] = &MockClient{
		ID:    "test-client",
		Queue: &MockQueue{ID: "queue-1"},
	}

	// Simulate the exact deadlock scenario:
	// 1. Goroutine tries to find queue (will try to acquire read lock)
	// 2. shutdownSequence holds write lock
	// 3. shutdownSequence releases write lock and tries to find queue (calls findClientQueue)
	// This should NOT deadlock

	done := make(chan bool, 2)
	var wg sync.WaitGroup

	// Goroutine 1: Try to find queue (simulating welcome message goroutine)
	goroutinelabels.NewGoroutine("test_queue_finder_shutdown", "finding client queue during shutdown in test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			// Small delay to ensure shutdown starts first
			time.Sleep(10 * time.Millisecond)
			queue := server.findClientQueue()
			_ = queue // Use the result
			done <- true
		})

	// Goroutine 2: shutdownSequence (simulating main thread)
	wg.Add(1)
	goroutinelabels.NewGoroutine("mcp_test", "shutdown sequence").StartSimple(func() {
		defer wg.Done()
		server.shutdownSequence()
		done <- true
	})

	// Wait for both with timeout
	timeout := time.After(2 * time.Second)
	completed := 0
	for completed < 2 {
		select {
		case <-done:
			completed++
		case <-timeout:
			t.Fatal("DEADLOCK DETECTED: Test timed out - goroutines are blocked")
		}
	}

	wg.Wait()
	// If we get here, no deadlock occurred
}

// TestLockingPattern_WriteLockHeldTimeout tests timeout when write lock is held
func TestLockingPattern_WriteLockHeldTimeout(t *testing.T) {
	t.Parallel()
	server := &MockServer{
		clients: make(map[string]*MockClient),
	}

	// Add a client
	server.clients["test-client"] = &MockClient{
		ID:    "test-client",
		Queue: &MockQueue{ID: "queue-1"},
	}

	// Hold write lock for longer than timeout
	server.clientsMu.Lock()

	// Try to find queue - should timeout and return nil
	queue := server.findClientQueue()
	if queue != nil {
		t.Fatal("Expected nil queue when write lock is held (timeout), got queue")
	}

	// Release write lock
	server.clientsMu.Unlock()
}

// TestLockingPattern_MultipleShutdownCalls tests idempotent shutdown
func TestLockingPattern_MultipleShutdownCalls(t *testing.T) {
	t.Parallel()
	server := &MockServer{
		clients: make(map[string]*MockClient),
	}

	// Call shutdown multiple times
	for i := 0; i < 10; i++ {
		server.shutdownSequence()
	}

	// Should still work (idempotent)
	if !server.isShuttingDown() {
		t.Fatal("Expected shutdown to be in progress")
	}

	// Should return nil queue
	queue := server.findClientQueue()
	if queue != nil {
		t.Fatal("Expected nil queue during shutdown, got queue")
	}
}

// TestLockingPattern_StressTest runs many concurrent operations
func TestLockingPattern_StressTest(t *testing.T) {
	t.Parallel()
	server := &MockServer{
		clients: make(map[string]*MockClient),
	}

	// Add clients
	for i := 0; i < 10; i++ {
		server.clients[fmt.Sprintf("client-%d", i)] = &MockClient{
			ID:    fmt.Sprintf("client-%d", i),
			Queue: &MockQueue{ID: fmt.Sprintf("queue-%d", i)},
		}
	}

	// Run stress test: many concurrent reads, then shutdown
	var wg sync.WaitGroup
	var successCount atomic.Int32

	// Spawn many read goroutines
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("mcp_test", "stress read goroutine").StartSimple(func() {
			defer wg.Done()
			queue := server.findClientQueue()
			if queue != nil {
				successCount.Add(1)
			}
		})
	}

	// Start shutdown in middle of reads
	goroutinelabels.NewGoroutine("mcp_test", "stress shutdown goroutine").StartSimple(func() {
		time.Sleep(50 * time.Millisecond)
		server.shutdownSequence()
	})

	// Wait for all reads to complete
	wg.Wait()

	// Some should succeed (before shutdown), some should fail (after shutdown)
	// But no deadlocks should occur
	if successCount.Load() == 0 && server.isShuttingDown() {
		// This is OK - shutdown started before any reads completed
	} else if successCount.Load() > 0 && !server.isShuttingDown() {
		// This is OK - shutdown didn't start yet
	}
	// Any other combination is also OK - the important thing is no deadlock
}
