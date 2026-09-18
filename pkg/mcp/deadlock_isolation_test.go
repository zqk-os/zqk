package mcp

import (
	"bufio"
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestDeadlockIsolation reproduces the exact deadlock scenario:
//  1. shutdownSequence calls SendLogDebug (which calls SendLogMessage -> findClientQueue)
//  2. Welcome goroutine calls SendMessageToClient (which calls findClientByID)
//  3. Both try to acquire clientsMu via TryRLock(), but one might block if shutdownSequence
//     acquires clientsMu.Lock() between the shutdown context check and TryRLock()
func TestDeadlockIsolation(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping deadlock isolation test in short mode (designed for full run)")
	}
	if !runDeadlockReproductionTests() {
		t.Skip("skipping isolation test unless MCP_RUN_DEADLOCK_REPRODUCTION=1 (timing-sensitive)")
	}
	server := NewServer()
	server.SetInitialized(true)

	// Set up transport
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	format := &MessageFormat{IsRawJSON: false}

	server.transportMu.Lock()
	server.transportWriter = writer
	server.transportFormat = format
	server.transportMu.Unlock()

	// Create client with queue
	config := DefaultQueueConfig()
	queue := NewMessageQueue(writer, format, config)
	defer queue.Stop()

	server.clientsMu.Lock()
	server.clients["test-client"] = &ClientConnection{
		ID:     "test-client",
		Writer: writer,
		Format: format,
		Queue:  queue,
	}
	server.clientsMu.Unlock()

	// Set client ID
	server.clientIDMu.Lock()
	server.clientID = "test-client"
	server.clientIDMu.Unlock()

	// Welcome message goroutine (matches handleNotificationInitialized)
	// WithWaitGroup adds 1; WithSignalOnExit signals welcomeDone on any exit path.
	var welcomeWg sync.WaitGroup
	welcomeDone := make(chan bool, 1)
	goroutinelabels.NewGoroutine("test_welcome_message", "sending welcome message in deadlock test").
		WithWaitGroup(&welcomeWg).
		WithSignalOnExit(welcomeDone).
		StartSimple(func() {
			// NO delay - start immediately to maximize race condition
			// This simulates the real scenario where welcome goroutine starts
			// at the same time as shutdownSequence

			// Check atomic shutdown flag first (fastest check)
			if server.shutdownFlag.Load() == 1 {
				return // Shutdown ordered, exit immediately
			}

			// Check shutdown context (matches real code)
			select {
			case <-server.GetShutdownContext().Done():
				return // Shutdown ordered, exit immediately
			default:
			}

			// Try to send welcome message
			// This calls SendMessageToClient -> SendMessageToClientByID -> findClientByID
			// findClientByID uses withClientsReadLock which uses TryRLock()
			// SendMessageToClientByID has a guard at the start, so it will exit if shutdown is set
			_ = server.SendMessageToClient("Welcome", "welcome", "medium") //nolint:errcheck // Test scenario
		})

	// Shutdown sequence goroutine
	// This calls SendLogDebug at line 999, which calls SendLogMessage -> findClientQueue
	// Then it acquires clientsMu.Lock() at line 1027
	var shutdownWg sync.WaitGroup
	shutdownDone := make(chan bool, 1)
	goroutinelabels.NewGoroutine("test_shutdown_sequence", "executing shutdown sequence in deadlock test").
		WithWaitGroup(&shutdownWg).
		WithSignalOnExit(shutdownDone).
		StartSimple(func() {
			// Start shutdown - this will call SendLogDebug which calls SendLogMessage
			// SendLogMessage calls findClientQueue which uses TryRLock()
			server.shutdownSequence("test deadlock isolation")
		})

	// Wait for both goroutines to complete
	// Use WaitGroup to ensure both actually complete, not just signal
	done := make(chan bool, 1)
	goroutinelabels.NewGoroutine("test_wait_collector", "waiting for test goroutines to complete").
		StartSimple(func() {
			welcomeWg.Wait()
			shutdownWg.Wait()
			done <- true
		})

	waitForSignal(t, done, 15*time.Second, "DEADLOCK DETECTED: Test timed out - goroutines are blocked or leaked")
	// Both goroutines completed - verify shutdown flag was set
	if server.shutdownFlag.Load() != 1 {
		t.Fatal("Shutdown flag was not set - shutdownSequence may not have run")
	}
	// Verify both channels were signaled (defensive check)
	select {
	case <-welcomeDone:
		// Welcome goroutine signaled
	default:
		t.Fatal("Welcome goroutine did not signal completion")
	}
	select {
	case <-shutdownDone:
		// Shutdown goroutine signaled
	default:
		t.Fatal("Shutdown goroutine did not signal completion")
	}
}
