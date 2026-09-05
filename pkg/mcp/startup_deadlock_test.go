package mcp

import (
	"bufio"
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TestStartupDeadlockExactSequence reproduces the EXACT startup deadlock scenario:
// 1. Server starts up
// 2. Successfully connected to stdio server (triggers handleNotificationInitialized)
// 3. handleNotificationInitialized spawns goroutine that sleeps then sends welcome message
// 4. JSON parsing errors occur (invalid characters in output)
// 5. Error responses are sent (which may trigger SendLogDebug)
// 6. Write errors may occur, triggering shutdownSequence
// 7. Deadlock: welcome message goroutine and shutdownSequence both try to acquire clientsMu.RLock()
//
// Skipped in -short to avoid flaky timeouts; run with -short=false to exercise deadlock detection.
func TestStartupDeadlockExactSequence(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("skipping deadlock detection test in short mode")
	}
	if !runDeadlockReproductionTests() {
		t.Skip("skipping exact-sequence test unless MCP_RUN_DEADLOCK_REPRODUCTION=1 (timing-sensitive)")
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

	// Step 3: Simulate handleNotificationInitialized being called
	// This sets client ID and spawns welcome message goroutine
	server.clientIDMu.Lock()
	server.clientID = "project-0-zqk-zqk-mcp"
	server.clientIDMu.Unlock()

	// Step 4: Simulate handleNotificationInitialized spawning welcome message goroutine
	// Exact match: time.Sleep(DefaultLogNotificationTimeout) then SendMessageToClient
	// WithWaitGroup does Add(1); WithSignalOnExit signals welcomeDone on any exit path.
	var welcomeWg sync.WaitGroup
	welcomeDone := make(chan bool, 1)
	goroutinelabels.NewGoroutine("test_welcome_startup", "sending welcome message in startup deadlock test").
		WithWaitGroup(&welcomeWg).
		WithSignalOnExit(welcomeDone).
		StartSimple(func() {
			// Exact match: time.Sleep(DefaultLogNotificationTimeout)
			time.Sleep(DefaultLogNotificationTimeout)

			// Check shutdown before sending (matches real code)
			if server.isShuttingDown() {
				return
			}

			// Try sending welcome message (matches handleNotificationInitialized.func1())
			// This will try to acquire clientsMu.RLock() via SendMessageToClientByID
			err := server.SendMessageToClient(
				"Welcome! Please introduce yourself and let me know how I can help you today.",
				"welcome",
				"medium",
			)
			_ = err
		})

	// Step 5: Simulate JSON parsing errors triggering error responses
	// When JSON parsing fails, handleParseError is called, which sends error response
	// If that write fails, sendResponse calls shutdownSequence
	// Also, SendLogDebug might be called during error handling
	var errorWg sync.WaitGroup
	errorDone := make(chan bool, 1)
	goroutinelabels.NewGoroutine("test_error_handler", "handling JSON parsing errors in startup deadlock test").
		WithWaitGroup(&errorWg).
		WithSignalOnExit(errorDone).
		StartSimple(func() {
			// Small delay to let welcome goroutine start
			time.Sleep(20 * time.Millisecond)

			// Simulate JSON parsing error triggering SendLogDebug
			// This happens when errors are logged during message processing
			_ = server.SendLogDebug("JSON parsing error occurred", map[string]any{ //nolint:errcheck // Test scenario
				"error": "Unexpected token 'd', \"distribute\"... is not valid JSON",
				"event": "parse_error",
			})

			// Simulate multiple error logs (like in the real log file)
			for i := 0; i < 5; i++ {
				_ = server.SendLogDebug("Client error for command", map[string]any{ //nolint:errcheck // Test scenario
					"error": "Unexpected token",
					"event": "client_error",
				})
			}

			// Simulate write error triggering shutdown (like sendResponse does)
			// This is what actually triggers shutdownSequence during startup
			// In the real code, if WriteMessage fails in sendResponse, it calls shutdownSequence
			server.shutdownSequence("write error during error response")
		})

	waitForOneFromEach(t, welcomeDone, errorDone, 30*time.Second,
		"DEADLOCK DETECTED: Test timed out - goroutines are blocked waiting for clientsMu.RLock()")
	welcomeWg.Wait()
	errorWg.Wait()
}

// TestStartupDeadlockWithParseError simulates the exact scenario:
// handleParseError -> sendResponse -> write error -> shutdownSequence
// while welcome message goroutine is also trying to send
//
// Skipped in -short to avoid flaky timeouts; run with -short=false to exercise deadlock detection.
func TestStartupDeadlockWithParseError(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping deadlock detection test in short mode")
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

	// Set client ID (triggers welcome message goroutine)
	server.clientIDMu.Lock()
	server.clientID = "test-client"
	server.clientIDMu.Unlock()

	// Simulate handleNotificationInitialized welcome message goroutine
	// WithWaitGroup adds 1 internally; WithSignalOnExit signals welcomeDone on any exit path.
	var welcomeWg sync.WaitGroup
	welcomeDone := make(chan bool, 1)
	goroutinelabels.NewGoroutine("test_welcome_retry", "sending welcome message retry in startup deadlock test").
		WithWaitGroup(&welcomeWg).
		WithSignalOnExit(welcomeDone).
		StartSimple(func() {
			time.Sleep(DefaultLogNotificationTimeout)

			if server.isShuttingDown() {
				return
			}

			// Try to send welcome message
			_ = server.SendMessageToClient("Welcome message", "welcome", "medium") //nolint:errcheck // Test scenario
		})

	// Simulate parse error handling that triggers shutdown
	// In real code: handleParseError -> sendResponse -> write error -> shutdownSequence
	var shutdownWg sync.WaitGroup
	shutdownDone := make(chan bool, 1)
	goroutinelabels.NewGoroutine("test_shutdown_parse_error", "shutdown after parse error in startup deadlock test").
		WithWaitGroup(&shutdownWg).
		WithSignalOnExit(shutdownDone).
		StartSimple(func() {
			// Small delay to let welcome goroutine start
			time.Sleep(30 * time.Millisecond)

			// Simulate what happens when write fails in sendResponse
			// This calls shutdownSequence, which calls SendLogDebug multiple times
			server.shutdownSequence("write error: failed to write parse error response")
		})

	waitForOneFromEach(t, welcomeDone, shutdownDone, 30*time.Second, "DEADLOCK DETECTED: Test timed out")
	welcomeWg.Wait()
	shutdownWg.Wait()
}

// TestStartupDeadlockConcurrentLogs tests multiple concurrent SendLogDebug calls
// during startup error handling (simulating multiple JSON parse errors).
//
// Skipped in -short to avoid flaky timeouts; run with -short=false to exercise.
func TestStartupDeadlockConcurrentLogs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping deadlock detection test in short mode")
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

	// Welcome message goroutine (builder does Add(1), no done channel needed for this test)
	var welcomeWg sync.WaitGroup
	goroutinelabels.NewGoroutine("test_welcome_concurrent", "welcome in concurrent logs test").
		WithWaitGroup(&welcomeWg).
		StartSimple(func() {
			time.Sleep(DefaultLogNotificationTimeout)
			if !server.isShuttingDown() {
				_ = server.SendMessageToClient("Welcome", "welcome", "medium") //nolint:errcheck // Test scenario
			}
		})

	// Spawn many goroutines simulating concurrent error logging
	// (like multiple JSON parse errors happening simultaneously)
	var errorWg sync.WaitGroup
	for i := 0; i < 50; i++ {
		errorWg.Add(1)
		goroutinelabels.NewGoroutine("mcp_test", "concurrent error logging").StartSimple(func() {
			func(id int) {
				defer errorWg.Done()
				time.Sleep(time.Duration(id) * time.Millisecond)
				_ = server.SendLogDebug("JSON parse error", map[string]any{ //nolint:errcheck // Test scenario
					"error_id": id,
					"event":    "parse_error",
				})
			}(i)
		})
	}

	// One goroutine triggers shutdown (simulating write error)
	errorWg.Add(1)
	goroutinelabels.NewGoroutine("mcp_test", "trigger shutdown").StartSimple(func() {
		defer errorWg.Done()
		time.Sleep(50 * time.Millisecond)
		server.shutdownSequence("write error")
	})

	done := make(chan bool)
	goroutinelabels.NewGoroutine("mcp_test", "wait for completion").StartSimple(func() {
		welcomeWg.Wait()
		errorWg.Wait()
		done <- true
	})
	waitForSignal(t, done, 30*time.Second, "DEADLOCK DETECTED: Test timed out")
}
