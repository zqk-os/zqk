package mcp

import (
	"bufio"
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/config"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// runDeadlockReproductionTests enables the exact-reproduction deadlock tests.
// These tests are sensitive to timing and can block if the welcome goroutine blocks on queue send.
// Set to "1" to run them (e.g. for debugging); leave unset for normal CI.
func runDeadlockReproductionTests() bool {
	return config.MCPRunDeadlockReproduction().Safe()
}

// TestShutdownDeadlockExactReproduction reproduces the EXACT deadlock scenario from the log:
// 1. CreateClient action
// 2. Server creation starts
// 3. Multiple GetInstructions actions (server creation in progress)
// 4. Successfully connected to stdio server
// 5. ListOfferings action
// 6. More GetInstructions actions
// 7. JSON parsing errors occur
// 8. Deadlock: shutdownSequence and handleNotificationInitialized goroutine both try to acquire clientsMu.RLock()
func TestShutdownDeadlockExactReproduction(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping shutdown deadlock test in short mode (designed for full run)")
	}
	if !runDeadlockReproductionTests() {
		t.Skip("skipping exact-reproduction test unless MCP_RUN_DEADLOCK_REPRODUCTION=1 (timing-sensitive)")
	}
	server := NewServer()
	server.SetInitialized(true)

	// Step 1: Set up transport (simulating stdio connection)
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	format := &MessageFormat{IsRawJSON: false}

	server.transportMu.Lock()
	server.transportWriter = writer
	server.transportFormat = format
	server.transportMu.Unlock()

	// Step 2: Create client with queue (simulating "Storing stdio client project-0-zqk-zqk-mcp")
	config := DefaultQueueConfig()
	queue := NewMessageQueue(writer, format, config)
	defer queue.Stop()

	server.clientsMu.Lock()
	server.clients["project-0-zqk-zqk-mcp"] = &ClientConnection{
		ID:     "project-0-zqk-zqk-mcp",
		Writer: writer,
		Format: format,
		Queue:  queue,
	}
	server.clientsMu.Unlock()

	// Step 3: Set client ID (triggers handleNotificationInitialized welcome message goroutine)
	server.clientIDMu.Lock()
	server.clientID = "project-0-zqk-zqk-mcp"
	server.clientIDMu.Unlock()

	// Step 4: Simulate handleNotificationInitialized spawning the welcome message goroutine
	// This matches the exact code: time.Sleep(DefaultLogNotificationTimeout) then SendMessageToClient
	welcomeMessageSent := make(chan bool, 1)
	var welcomeWg sync.WaitGroup
	goroutinelabels.NewGoroutine("test_welcome_shutdown", "sending welcome message in shutdown deadlock test").
		WithWaitGroup(&welcomeWg).
		StartSimple(func() {
			// Exact match: time.Sleep(DefaultLogNotificationTimeout)
			time.Sleep(DefaultLogNotificationTimeout)

			// Check shutdown before sending (this is in the real code)
			// Use atomic flag check (lock-free, thread-safe)
			if server.shutdownFlag.Load() == 1 {
				return
			}

			// Try sending welcome message (matches handleNotificationInitialized.func1())
			// This will try to acquire clientsMu.RLock() at server.go:1324
			err := server.SendMessageToClient(
				"Welcome! Please introduce yourself and let me know how I can help you today.",
				"welcome",
				"medium",
			)
			if err == nil {
				welcomeMessageSent <- true
			}
		})

	// Step 5: Simulate the sequence of events that leads to shutdown
	// Based on the log, JSON parsing errors occur, then shutdown happens
	// We'll trigger shutdown after a short delay (simulating the errors leading to shutdown)
	time.Sleep(50 * time.Millisecond) // Give welcome goroutine time to start sleeping

	// Step 6: Trigger shutdown (this simulates what happens after JSON errors)
	// shutdownSequence will call SendLogDebug multiple times, each trying to acquire clientsMu.RLock()
	var shutdownWg sync.WaitGroup
	goroutinelabels.NewGoroutine("test_shutdown_sequence", "executing shutdown sequence in shutdown deadlock test").
		WithWaitGroup(&shutdownWg).
		StartSimple(func() {
			// This calls SendLogDebug which calls SendLogMessage
			// SendLogMessage tries to acquire clientsMu.RLock() at logging_channel.go:155
			server.shutdownSequence("JSON parsing errors led to shutdown")
		})

	// Step 7: Wait for both with timeout - if deadlock occurs, this will timeout
	done := make(chan bool, 2)
	goroutinelabels.NewGoroutine("test_welcome_wait", "waiting for welcome goroutine in shutdown deadlock test").
		StartSimple(func() {
			welcomeWg.Wait()
			done <- true
		})
	goroutinelabels.NewGoroutine("test_shutdown_wait", "waiting for shutdown goroutine in shutdown deadlock test").
		StartSimple(func() {
			shutdownWg.Wait()
			done <- true
		})

	waitForNSignals(t, done, 2, 15*time.Second, "DEADLOCK DETECTED: Test timed out - goroutines are blocked waiting for clientsMu.RLock()")
}

// TestShutdownDeadlock reproduces the deadlock scenario:
// 1. shutdownSequence calls SendLogDebug
// 2. handleNotificationInitialized goroutine calls SendMessageToClient
// Both try to acquire clientsMu.RLock() simultaneously
func TestShutdownDeadlock(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping shutdown deadlock test in short mode (designed for full run)")
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

	// Create client with queue to trigger the welcome message goroutine
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

	// Set client ID to trigger welcome message
	server.clientIDMu.Lock()
	server.clientID = "test-client"
	server.clientIDMu.Unlock()

	// Simulate the handleNotificationInitialized scenario; both goroutines signal done on exit via builder.
	done := make(chan bool, 2)
	var wg sync.WaitGroup

	goroutinelabels.NewGoroutine("test_welcome_deadlock", "welcome message in shutdown deadlock test").
		WithWaitGroup(&wg).
		WithSignalOnExit(done).
		StartSimple(func() {
			time.Sleep(DefaultLogNotificationTimeout)
			_ = server.SendMessageToClient("Welcome message", "info", "normal") //nolint:errcheck // Test scenario
		})

	goroutinelabels.NewGoroutine("test_shutdown_deadlock", "shutdown sequence in shutdown deadlock test").
		WithWaitGroup(&wg).
		WithSignalOnExit(done).
		StartSimple(func() {
			server.shutdownSequence("test deadlock")
		})

	waitForNSignals(t, done, 2, 15*time.Second, "DEADLOCK DETECTED: Test timed out - goroutines are blocked waiting for locks")
	wg.Wait()
}

// TestShutdownDeadlockConcurrentLogs tests multiple concurrent SendLogDebug calls during shutdown
func TestShutdownDeadlockConcurrentLogs(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping shutdown deadlock test in short mode")
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

	// Start shutdown in one goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	goroutinelabels.NewGoroutine("mcp_test", "trigger shutdown sequence").StartSimple(func() {
		defer wg.Done()
		server.shutdownSequence("test concurrent logs")
	})

	// Spawn many goroutines trying to send messages concurrently
	for i := 0; i < 50; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("mcp_test", "concurrent message send").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				_ = server.SendMessageToClient( //nolint:errcheck // Test scenario
					"Concurrent message",
					"info",
					"normal",
				)
			}(i)
		})
	}

	done := make(chan bool)
	goroutinelabels.NewGoroutine("mcp_test", "wait for completion").StartSimple(func() {
		wg.Wait()
		done <- true
	})
	waitForSignal(t, done, 20*time.Second, "DEADLOCK DETECTED: Test timed out - goroutines are blocked")

}
