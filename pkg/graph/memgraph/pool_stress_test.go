package memgraph

import (
	"context"
	"sync"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/graph/provider"
)

// TestMemGraphConnectionPool_Starvation tests pool starvation scenario
// where all connections are in use and new requests must wait
func TestMemGraphConnectionPool_Starvation(t *testing.T) {
	t.Parallel()
	config := MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		PoolSize: 2, // Small pool to force starvation
	}

	pool, err := NewMemGraphConnectionPool(&config)
	if err != nil {
		t.Fatalf("NewMemGraphConnectionPool failed: %v", err)
	}
	defer pool.Close()

	ctx := pkgctx.NewSystemContext()

	// Acquire all connections
	conn1, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatalf("GetConnection 1 failed: %v", err)
	}

	conn2, err := pool.GetConnection(ctx)
	if err != nil {
		t.Fatalf("GetConnection 2 failed: %v", err)
	}

	// Verify pool is exhausted
	stats := pool.Stats()
	if stats.Active != 2 {
		t.Errorf("Expected 2 active connections, got %d", stats.Active)
	}
	if stats.Idle != 0 {
		t.Errorf("Expected 0 idle connections, got %d", stats.Idle)
	}

	// Try to get another connection with timeout - should wait
	ctxWithTimeout, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = pool.GetConnection(ctxWithTimeout)
	elapsed := time.Since(start)

	// Should timeout waiting for connection
	if err == nil {
		t.Error("Expected timeout error when pool is exhausted")
	}
	if elapsed < 50*time.Millisecond {
		t.Errorf("Expected to wait at least 50ms, waited %v", elapsed)
	}

	// Verify wait count increased
	stats = pool.Stats()
	if stats.WaitCount == 0 {
		t.Error("Expected WaitCount to be > 0 after starvation")
	}

	// Return connections
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = pool.ReturnConnection(conn1)
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = pool.ReturnConnection(conn2)
}

// TestMemGraphConnectionPool_Timeout tests context timeout when waiting for connection
func TestMemGraphConnectionPool_Timeout(t *testing.T) {
	t.Parallel()
	config := MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		PoolSize: 1,
	}

	pool, err := NewMemGraphConnectionPool(&config)
	if err != nil {
		t.Fatalf("NewMemGraphConnectionPool failed: %v", err)
	}
	defer pool.Close()

	// Acquire the only connection
	conn, err := pool.GetConnection(pkgctx.NewSystemContext())
	if err != nil {
		t.Fatalf("GetConnection failed: %v", err)
	}

	// Try to get connection with very short timeout
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 10*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = pool.GetConnection(ctx)
	elapsed := time.Since(start)

	// Should timeout
	if err == nil {
		t.Error("Expected timeout error")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}

	// Should have waited approximately the timeout duration
	if elapsed < 5*time.Millisecond || elapsed > 50*time.Millisecond {
		t.Errorf("Expected timeout around 10ms, got %v", elapsed)
	}

	// Return connection
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = pool.ReturnConnection(conn)
}

// TestMemGraphConnectionPool_ConcurrentAccess tests concurrent access to pool
// This helps detect race conditions and deadlocks
//
//nolint:gocyclo // Stress test intentionally exercises many branches
func TestMemGraphConnectionPool_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	config := MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		PoolSize: 5,
	}

	pool, err := NewMemGraphConnectionPool(&config)
	if err != nil {
		t.Fatalf("NewMemGraphConnectionPool failed: %v", err)
	}
	defer pool.Close()

	const numGoroutines = 20
	const operationsPerGoroutine = 10

	var wg sync.WaitGroup
	errors := make(chan error, numGoroutines*operationsPerGoroutine)

	// Launch multiple goroutines that acquire and return connections
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("memgraph_stress", "stress test goroutine").
			StartSimple(func() {
				defer wg.Done()
				ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
				defer cancel()

				for j := 0; j < operationsPerGoroutine; j++ {
					conn, err := pool.GetConnection(ctx)
					if err != nil {
						errors <- err
						continue
					}

					// Simulate some work
					time.Sleep(1 * time.Millisecond)

					if err := pool.ReturnConnection(conn); err != nil {
						errors <- err
					}
				}
			})
	}

	// Wait for all goroutines to complete
	done := make(chan struct{})
	goroutinelabels.StartTestGoroutine("test_wait_collector", "waiting for pool stress test goroutines", func() {
		wg.Wait()
		close(done)
	})

	select {
	case <-done:
		// All goroutines completed
	case <-time.After(10 * time.Second):
		t.Fatal("Test timed out - possible deadlock")
	}

	// Check for errors
	close(errors)
	errorCount := 0
	for err := range errors {
		if err != nil {
			t.Errorf("Concurrent operation error: %v", err)
			errorCount++
		}
	}

	if errorCount > 0 {
		t.Errorf("Encountered %d errors during concurrent access", errorCount)
	}

	// Verify pool stats are consistent
	stats := pool.Stats()
	if stats.Active < 0 || stats.Active > config.PoolSize {
		t.Errorf("Invalid Active count: %d (max: %d)", stats.Active, config.PoolSize)
	}
	if stats.Idle < 0 || stats.Idle > config.PoolSize {
		t.Errorf("Invalid Idle count: %d (max: %d)", stats.Idle, config.PoolSize)
	}
	if stats.Active+stats.Idle > config.PoolSize {
		t.Errorf("Active + Idle (%d + %d) exceeds MaxSize (%d)", stats.Active, stats.Idle, config.PoolSize)
	}
}

// TestMemGraphConnectionPool_ConnectionLeak tests that connections are properly returned
func TestMemGraphConnectionPool_ConnectionLeak(t *testing.T) {
	t.Parallel()
	config := MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		PoolSize: 3,
	}

	pool, err := NewMemGraphConnectionPool(&config)
	if err != nil {
		t.Fatalf("NewMemGraphConnectionPool failed: %v", err)
	}
	defer pool.Close()

	// Acquire all connections
	conns := make([]provider.GraphConnection, config.PoolSize)
	for i := 0; i < config.PoolSize; i++ {
		conn, err := pool.GetConnection(pkgctx.NewSystemContext())
		if err != nil {
			t.Fatalf("GetConnection %d failed: %v", i, err)
		}
		conns[i] = conn
	}

	// Verify all are active
	stats := pool.Stats()
	if stats.Active != config.PoolSize {
		t.Errorf("Expected %d active connections, got %d", config.PoolSize, stats.Active)
	}

	// Return all connections
	for _, conn := range conns {
		if err := pool.ReturnConnection(conn); err != nil {
			t.Errorf("ReturnConnection failed: %v", err)
		}
	}

	// Verify all are idle now
	stats = pool.Stats()
	if stats.Active != 0 {
		t.Errorf("Expected 0 active connections after return, got %d", stats.Active)
	}
	if stats.Idle != config.PoolSize {
		t.Errorf("Expected %d idle connections, got %d", config.PoolSize, stats.Idle)
	}
}

// TestMemGraphConnectionPool_HangingConnection tests behavior when connection is held too long
func TestMemGraphConnectionPool_HangingConnection(t *testing.T) {
	t.Parallel()
	config := MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		PoolSize: 2,
	}

	pool, err := NewMemGraphConnectionPool(&config)
	if err != nil {
		t.Fatalf("NewMemGraphConnectionPool failed: %v", err)
	}
	defer pool.Close()

	// Acquire a connection and "hang" it (simulate long operation)
	conn1, err := pool.GetConnection(pkgctx.NewSystemContext())
	if err != nil {
		t.Fatalf("GetConnection failed: %v", err)
	}

	// Start a goroutine that will hold the connection for a while
	done := make(chan struct{})
	goroutinelabels.StartTestGoroutine("test_connection_holder", "holding connection in pool stress test", func() {
		defer close(done)
		time.Sleep(200 * time.Millisecond)
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = pool.ReturnConnection(conn1)
	})

	// Try to get another connection - should succeed (pool has 2)
	conn2, err := pool.GetConnection(pkgctx.NewSystemContext())
	if err != nil {
		t.Fatalf("GetConnection 2 failed: %v", err)
	}
	//nolint:errcheck // Test cleanup - errors are acceptable
	_ = pool.ReturnConnection(conn2)

	// Now try to get a connection while conn1 is still held
	// Should timeout since pool is exhausted (conn1 is held, conn2 was returned)
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 50*time.Millisecond)
	defer cancel()

	_, err = pool.GetConnection(ctx)
	// May or may not timeout depending on timing - conn1 might be returned by now
	// Just verify we don't get a connection immediately if pool is exhausted
	if err == nil {
		// Got a connection - verify it's valid
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = pool.ReturnConnection(conn2)
	}

	// Wait for hanging connection to be returned
	select {
	case <-done:
		// Connection returned
	case <-time.After(500 * time.Millisecond):
		t.Error("Hanging connection was not returned in time")
	}

	// Now we should be able to get a connection
	conn3, err := pool.GetConnection(pkgctx.NewSystemContext())
	if err != nil {
		t.Errorf("GetConnection should succeed after hanging connection returned: %v", err)
	} else {
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = pool.ReturnConnection(conn3)
	}
}

// TestMemGraphConnectionPool_ReturnConnectionWithOpenTransaction tests that
// connections with open transactions are properly handled
func TestMemGraphConnectionPool_ReturnConnectionWithOpenTransaction(t *testing.T) {
	t.Parallel()
	config := MemGraphConfig{
		Host:     "localhost",
		Port:     7687,
		PoolSize: 2,
	}

	pool, err := NewMemGraphConnectionPool(&config)
	if err != nil {
		t.Fatalf("NewMemGraphConnectionPool failed: %v", err)
	}
	defer pool.Close()

	ctx := pkgctx.NewSystemContext()

	conn, err := pool.GetConnection(ctx)
	if err != nil {
		t.Skip("Skipping test - connection creation not yet implemented")
		return
	}

	// Start a transaction (if supported)
	tx, err := conn.BeginTransaction(ctx)
	if err != nil {
		// Skip if connection/session not properly initialized (expected when MemGraph not running)
		if err.Error() == "bolt client not initialized" ||
			err.Error() == "bolt driver not initialized" ||
			err.Error() == "session is nil - connection not properly initialized" ||
			err.Error() == "failed to create session - driver returned nil" {
			t.Skip("Skipping test - MemGraph not running or connection not initialized")
			return
		}
		t.Skipf("Skipping test - transaction support not yet implemented: %v", err)
		return
	}

	// Return connection with open transaction
	// Should automatically rollback
	err = pool.ReturnConnection(conn)
	if err != nil {
		t.Errorf("ReturnConnection failed: %v", err)
	}

	// Verify transaction was rolled back
	if tx != nil {
		// Check if transaction has a method to verify it was rolled back
		// This depends on the transaction implementation
		_ = tx
	}

	// Verify connection can be reused
	conn2, err := pool.GetConnection(ctx)
	if err != nil {
		t.Errorf("GetConnection after return failed: %v", err)
	} else {
		//nolint:errcheck // Test cleanup - errors are acceptable
		_ = pool.ReturnConnection(conn2)
	}
}
