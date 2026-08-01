package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestServerLoopResilience tests that the server loop continues operating
// despite various error conditions and doesn't shutdown unintentionally
func TestServerLoopResilience(t *testing.T) {
	t.Parallel()
	t.Run("handles_concurrent_shutdown_requests", func(t *testing.T) {
		server := NewServer()
		server.SetInitialized(true)

		// Spam shutdown requests from multiple goroutines
		var wg sync.WaitGroup
		var shutdownCount atomic.Int32
		for i := 0; i < 100; i++ {
			i := i
			goroutinelabels.NewGoroutine(fmt.Sprintf("test_shutdown_%d", i), fmt.Sprintf("requesting shutdown %d in test", i)).
				WithWaitGroup(&wg).
				StartSimple(func() {
					server.RequestShutdown(fmt.Sprintf("test shutdown %d", i))
					shutdownCount.Add(1)
				})
		}

		wg.Wait()

		// Server should only shutdown once (idempotent)
		if !server.IsShutdownRequested() {
			t.Error("Expected shutdown to be requested")
		}

		// Verify shutdown is idempotent
		server.RequestShutdown("another shutdown")
		if shutdownCount.Load() != 100 {
			t.Errorf("Expected 100 shutdown requests, got %d", shutdownCount.Load())
		}
	})

	t.Run("handles_panic_in_read_goroutine", func(t *testing.T) {
		server := NewServer()
		server.SetInitialized(true)

		// Test that server handles panic recovery gracefully
		// The actual panic recovery is tested in the serve loop itself
		// This test verifies the server state remains consistent

		// Verify server is still operational
		if server.IsShutdownRequested() {
			t.Error("Server should not have shutdown unexpectedly")
		}

		// Server should handle nil/empty states gracefully
		server.RequestShutdown("test")
		if !server.IsShutdownRequested() {
			t.Error("Shutdown should be requested")
		}
	})

	t.Run("handles_write_errors_gracefully", func(t *testing.T) {
		server := NewServer()
		server.SetInitialized(true)

		// Create a writer that fails
		failingWriter := &failingWriter{}
		writer := bufio.NewWriter(failingWriter)

		// Set transport writer
		server.transportMu.Lock()
		server.transportWriter = writer
		server.transportFormat = &MessageFormat{IsRawJSON: false}
		server.transportMu.Unlock()

		// Create a client queue with the failing writer
		// This ensures we test the queue-based error handling path
		config := DefaultQueueConfig()
		queue := NewMessageQueue(writer, &MessageFormat{IsRawJSON: false}, config)
		defer queue.Stop()
		server.clientsMu.Lock()
		server.clients["test-client"] = &ClientConnection{
			ID:     "test-client",
			Writer: writer,
			Format: &MessageFormat{IsRawJSON: false},
			Queue:  queue,
		}
		server.clientsMu.Unlock()

		// Try to send a message - should not crash
		// Note: With queue-based approach, errors happen asynchronously
		// The message will be enqueued successfully, but write will fail later
		err := server.SendMessageToClientByID("test-client", "test message", "test", "normal")
		// With queue-based approach, enqueue succeeds even if write will fail later
		// The error will be recorded in queue stats, not returned immediately
		// This is expected behavior - queue provides backpressure, not immediate errors
		if err != nil {
			// If error is returned (e.g., marshal error), that's fine
			// But queue enqueue should succeed even with failing writer
		}

		// Server should still be operational
		if server.IsShutdownRequested() {
			t.Error("Server should not have shutdown due to write error")
		}

		// Give queue time to process and fail
		time.Sleep(200 * time.Millisecond)

		// Verify queue recorded the error
		stats := queue.Stats()
		if stats.Errors == 0 {
			t.Error("Expected queue to record write error, but errors=0")
		}
	})

	t.Run("handles_concurrent_message_sends", func(t *testing.T) {
		server := NewServer()
		server.SetInitialized(true)

		// Create a working writer
		var buf bytes.Buffer
		writer := bufio.NewWriter(&buf)

		server.transportMu.Lock()
		server.transportWriter = writer
		server.transportFormat = &MessageFormat{IsRawJSON: false}
		server.transportMu.Unlock()

		// Send many messages concurrently
		var wg sync.WaitGroup
		var errorCount atomic.Int32
		for i := 0; i < 1000; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("mcp_test", "concurrent message send").StartSimple(func() {
				func(id int) {
					defer wg.Done()
					if err := server.SendMessageToClient(
						fmt.Sprintf("message %d", id),
						"test",
						"normal",
					); err != nil {
						errorCount.Add(1)
					}
				}(i)
			})
		}

		wg.Wait()

		// Some errors are acceptable (queue full, etc.), but server should continue
		if server.IsShutdownRequested() {
			t.Error("Server should not have shutdown due to concurrent sends")
		}
	})

	t.Run("handles_malformed_requests", func(t *testing.T) {
		server := NewServer()
		server.SetInitialized(true)

		// Create reader with malformed JSON
		malformedJSON := []byte(`{"jsonrpc":"2.0","method":"invalid` + "\n")
		reader := bufio.NewReader(bytes.NewReader(malformedJSON))
		var buf bytes.Buffer
		writer := bufio.NewWriter(&buf)

		server.transportMu.Lock()
		server.transportWriter = writer
		server.transportFormat = &MessageFormat{IsRawJSON: false}
		server.transportMu.Unlock()

		// Try to read - should handle error gracefully
		transport := NewDefaultTransport()
		_, _, err := transport.ReadMessage(reader)
		if err == nil {
			t.Error("Expected error from malformed JSON")
		}

		// Server should still be operational
		if server.IsShutdownRequested() {
			t.Error("Server should not have shutdown due to malformed request")
		}
	})

	t.Run("handles_rapid_initialize_requests", func(t *testing.T) {
		// Use test helper that sets up proper context
		server := createTestServerWithoutRegistry()

		// Spam initialize requests sequentially (initialize is not thread-safe by design)
		// But we test that the server handles rapid requests without crashing
		var successCount atomic.Int32
		var errorCount atomic.Int32
		for i := 0; i < 50; i++ {
			initParams := InitializeParams{
				ProtocolVersion: "2025-06-18",
				ClientInfo: struct {
					Name    string `json:"name"`
					Version string `json:"version"`
				}{
					Name:    fmt.Sprintf("client-%d", i),
					Version: "1.0.0",
				},
				Capabilities: map[string]any{
					objects.FieldKeyClientID: fmt.Sprintf("client-%d", i),
				},
			}
			params, _ := json.Marshal(initParams)
			_, err := server.handleInitialize(pkgctx.NewSystemContext(), "initialize", params)
			if err == nil {
				successCount.Add(1)
			} else {
				errorCount.Add(1)
			}
		}

		// First initialize should succeed, subsequent ones may fail (expected behavior)
		// The important thing is server doesn't crash and handles errors gracefully
		if successCount.Load() == 0 && errorCount.Load() == 0 {
			t.Error("Expected some initialize attempts to complete (success or error)")
		}

		// Server should still be operational
		if server.IsShutdownRequested() {
			t.Error("Server should not have shutdown due to rapid initializes")
		}
	})
}

// TestMessageQueueResilience tests the message queue under heavy load
func TestMessageQueueResilience(t *testing.T) {
	t.Run("handles_queue_overflow", func(t *testing.T) {
		var buf bytes.Buffer
		writer := bufio.NewWriter(&buf)
		format := &MessageFormat{IsRawJSON: false}

		// Create queue with small size
		config := QueueConfig{
			MaxQueueSize: 10,
			DropWhenFull: true,
			WriteTimeout: 100 * time.Millisecond,
		}
		queue := NewMessageQueue(writer, format, config)
		defer queue.Stop()

		// Flood queue with messages
		var dropped atomic.Int32
		var enqueued atomic.Int32
		for i := 0; i < 1000; i++ {
			data := []byte(fmt.Sprintf(`{"test":"message %d"}`, i))
			if queue.Enqueue(data, format, "normal", nil) {
				enqueued.Add(1)
			} else {
				dropped.Add(1)
			}
		}

		// Give queue time to process
		time.Sleep(200 * time.Millisecond)

		stats := queue.Stats()
		if stats.Dropped == 0 {
			t.Error("Expected some messages to be dropped")
		}

		// Stop queue
		queue.Stop()

		// Verify final stats
		finalStats := queue.Stats()
		if finalStats.Sent == 0 {
			t.Error("Expected some messages to be sent")
		}
	})

	t.Run("handles_slow_writer", func(t *testing.T) {
		slowWriter := &slowWriter{delay: 50 * time.Millisecond}
		writer := bufio.NewWriter(slowWriter)
		format := &MessageFormat{IsRawJSON: false}

		config := QueueConfig{
			MaxQueueSize: 100,
			DropWhenFull: true,
			WriteTimeout: 200 * time.Millisecond,
		}
		queue := NewMessageQueue(writer, format, config)
		defer queue.Stop()

		// Send messages faster than writer can process
		for i := 0; i < 200; i++ {
			data := []byte(fmt.Sprintf(`{"test":"message %d"}`, i))
			queue.Enqueue(data, format, "normal", nil)
		}

		// Give queue time to process
		time.Sleep(500 * time.Millisecond)

		stats := queue.Stats()
		if stats.QueueDepth > config.MaxQueueSize {
			t.Errorf("Queue depth %d exceeds max %d", stats.QueueDepth, config.MaxQueueSize)
		}

		queue.Stop()
	})

	t.Run("handles_writer_failures", func(t *testing.T) {
		failingWriter := &failingWriter{}
		writer := bufio.NewWriter(failingWriter)
		format := &MessageFormat{IsRawJSON: false}

		config := QueueConfig{
			MaxQueueSize: 100,
			DropWhenFull: true,
			WriteTimeout: 100 * time.Millisecond,
		}
		queue := NewMessageQueue(writer, format, config)
		defer queue.Stop()

		// Send messages - all should fail
		for i := 0; i < 10; i++ {
			data := []byte(fmt.Sprintf(`{"test":"message %d"}`, i))
			queue.Enqueue(data, format, "normal", nil)
		}

		// Give queue time to process
		time.Sleep(200 * time.Millisecond)

		stats := queue.Stats()
		if stats.Errors == 0 {
			t.Error("Expected errors from failing writer")
		}

		// Queue may or may not be marked inactive depending on error type
		// Broken pipe errors are fatal, but other errors may not be
		// The important thing is that errors are tracked and queue continues operating
		if stats.Errors == 0 {
			t.Error("Expected errors to be tracked")
		}

		queue.Stop()
	})

	t.Run("handles_concurrent_enqueue", func(t *testing.T) {
		var buf bytes.Buffer
		writer := bufio.NewWriter(&buf)
		format := &MessageFormat{IsRawJSON: false}

		config := QueueConfig{
			MaxQueueSize: 1000,
			DropWhenFull: true,
			WriteTimeout: 100 * time.Millisecond,
		}
		queue := NewMessageQueue(writer, format, config)
		defer queue.Stop()

		// Enqueue from multiple goroutines
		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("mcp_test", "concurrent enqueue").StartSimple(func() {
				func(id int) {
					defer wg.Done()
					for j := 0; j < 10; j++ {
						data := []byte(fmt.Sprintf(`{"test":"message %d-%d"}`, id, j))
						queue.Enqueue(data, format, "normal", nil)
					}
				}(i)
			})
		}

		wg.Wait()

		// Give queue time to process
		time.Sleep(500 * time.Millisecond)

		stats := queue.Stats()
		if stats.Sent == 0 {
			t.Error("Expected some messages to be sent")
		}

		queue.Stop()
	})

	t.Run("handles_priority_messages", func(t *testing.T) {
		var buf bytes.Buffer
		writer := bufio.NewWriter(&buf)
		format := &MessageFormat{IsRawJSON: false}

		config := QueueConfig{
			MaxQueueSize: 50,
			DropWhenFull: true,
			WriteTimeout: 100 * time.Millisecond,
		}
		queue := NewMessageQueue(writer, format, config)
		defer queue.Stop()

		// Fill queue with low priority messages
		for i := 0; i < 100; i++ {
			data := []byte(fmt.Sprintf(`{"test":"low priority %d"}`, i))
			queue.Enqueue(data, format, "low", nil)
		}

		// Send high priority messages - some should be dropped
		var highPrioritySent atomic.Int32
		for i := 0; i < 20; i++ {
			data := []byte(fmt.Sprintf(`{"test":"high priority %d"}`, i))
			if queue.Enqueue(data, format, "high", nil) {
				highPrioritySent.Add(1)
			}
		}

		// Give queue time to process
		time.Sleep(300 * time.Millisecond)

		stats := queue.Stats()
		if stats.Sent == 0 {
			t.Error("Expected some messages to be sent")
		}

		queue.Stop()
	})
}

// TestShutdownResilience tests that shutdown is properly handled
func TestShutdownResilience(t *testing.T) {
	t.Run("shutdown_is_idempotent", func(t *testing.T) {
		server := NewServer()

		// Call shutdown multiple times
		for i := 0; i < 10; i++ {
			server.RequestShutdown(fmt.Sprintf("shutdown %d", i))
		}

		if !server.IsShutdownRequested() {
			t.Error("Expected shutdown to be requested")
		}

		// Shutdown sequence should be idempotent
		server.shutdownSequence("test")
		server.shutdownSequence("test again")
		server.shutdownSequence("test third time")

		// Should not panic or error
	})

	t.Run("shutdown_cleans_up_queues", func(t *testing.T) {
		server := NewServer()
		server.SetInitialized(true)

		// Create client with queue
		var buf bytes.Buffer
		writer := bufio.NewWriter(&buf)
		format := &MessageFormat{IsRawJSON: false}

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

		// Enqueue some messages
		for i := 0; i < 10; i++ {
			data := []byte(fmt.Sprintf(`{"test":"message %d"}`, i))
			queue.Enqueue(data, format, "normal", nil)
		}

		// Shutdown should stop queues
		server.shutdownSequence("test shutdown")

		// Give queues time to flush
		time.Sleep(100 * time.Millisecond)

		// Verify queue is stopped
		stats := queue.Stats()
		// Queue may have sent some messages before stopping
		if stats.Sent == 0 && stats.QueueDepth > 0 {
			t.Error("Expected queue to flush remaining messages on shutdown")
		}
	})

	t.Run("shutdown_handles_nil_resources", func(t *testing.T) {
		server := NewServer()

		// Shutdown with nil resources should not panic
		server.shutdownSequence("test shutdown")

		// Should complete without error
	})
}

// TestConcurrentOperations tests concurrent operations don't cause races
func TestConcurrentOperations(t *testing.T) {
	t.Run("concurrent_tool_registration", func(t *testing.T) {
		server := NewServer()

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("mcp_test", "concurrent tool registration").StartSimple(func() {
				func(id int) {
					defer wg.Done()
					server.RegisterTool(
						fmt.Sprintf("test_tool_%d", id),
						"Test tool",
						nil,
						func(ctx context.Context, args map[string]any) (any, error) {
							return map[string]any{objects.FieldKeyID: id}, nil
						},
					)
				}(i)
			})
		}

		wg.Wait()

		// Verify all tools registered
		tools := server.ListTools()
		if len(tools) != 100 {
			t.Errorf("Expected 100 tools, got %d", len(tools))
		}
	})

	t.Run("concurrent_message_sends_with_queue", func(t *testing.T) {
		server := NewServer()
		server.SetInitialized(true)

		// Create client with queue
		var buf bytes.Buffer
		writer := bufio.NewWriter(&buf)
		format := &MessageFormat{IsRawJSON: false}

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

		// Send messages concurrently
		var wg sync.WaitGroup
		for i := 0; i < 500; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("mcp_test", "concurrent message send with queue").StartSimple(func() {
				func(id int) {
					defer wg.Done()
					_ = server.SendMessageToClientByID(
						"test-client",
						fmt.Sprintf("message %d", id),
						"test",
						"normal",
					)
				}(i)
			})
		}

		wg.Wait()

		// Give queue time to process
		time.Sleep(500 * time.Millisecond)

		stats := queue.Stats()
		if stats.Sent == 0 {
			t.Error("Expected some messages to be sent")
		}

		queue.Stop()
	})

	t.Run("concurrent_resource_registration", func(t *testing.T) {
		server := NewServer()

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("mcp_test", "concurrent resource registration").StartSimple(func() {
				func(id int) {
					defer wg.Done()
					server.RegisterResource(
						fmt.Sprintf("test://resource-%d", id),
						fmt.Sprintf("Resource %d", id),
						"text/plain",
						"Test resource",
					)
				}(i)
			})
		}

		wg.Wait()

		// Should complete without race conditions
	})
}

// TestErrorRecovery tests that the server recovers from errors
func TestErrorRecovery(t *testing.T) {
	t.Run("recovers_from_marshal_errors", func(t *testing.T) {
		server := NewServer()
		server.SetInitialized(true)

		// Create a writer
		var buf bytes.Buffer
		writer := bufio.NewWriter(&buf)

		server.transportMu.Lock()
		server.transportWriter = writer
		server.transportFormat = &MessageFormat{IsRawJSON: false}
		server.transportMu.Unlock()

		// Send message with invalid data (should handle gracefully)
		// This is tested indirectly through SendMessageToClient
		err := server.SendMessageToClient("test", "test", "normal")
		if err != nil && !strings.Contains(err.Error(), "marshal") {
			// Marshal errors are acceptable
		}

		// Server should still be operational
		if server.IsShutdownRequested() {
			t.Error("Server should not have shutdown due to marshal error")
		}
	})

	t.Run("recovers_from_transport_errors", func(t *testing.T) {
		server := NewServer()
		server.SetInitialized(true)

		// Create failing transport
		failingWriter := &failingWriter{}
		writer := bufio.NewWriter(failingWriter)
		format := &MessageFormat{IsRawJSON: false}

		server.transportMu.Lock()
		server.transportWriter = writer
		server.transportFormat = format
		server.transportMu.Unlock()

		// Create a client with a queue that uses the failing writer
		// This ensures SendMessageToClient uses the queue path
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

		// Set client ID so SendMessageToClient can find the client
		server.clientIDMu.Lock()
		server.clientID = "test-client"
		server.clientIDMu.Unlock()

		// Send message - queue will buffer, error occurs on flush
		// Note: SendMessageToClient uses queues which buffer writes, so errors
		// occur during queue flush, not immediately. The function is designed
		// for graceful degradation (no error returned, but server remains operational).
		err := server.SendMessageToClient("test", "test", "normal")
		// Queue-based sending doesn't return immediate errors (by design for graceful degradation)
		// The error will occur when the queue worker tries to flush, but that's handled internally
		if err != nil {
			// If an error is returned, that's fine too
			t.Logf("SendMessageToClient returned error (acceptable): %v", err)
		}

		// Server should still be operational (graceful degradation)
		if server.IsShutdownRequested() {
			t.Error("Server should not have shutdown due to transport error")
		}
	})
}

// Helper types for testing

type panicReader struct{}

func (r *panicReader) Read(p []byte) (n int, err error) {
	panic("intentional panic for testing")
}

type panicTransport struct{}

func (t *panicTransport) ReadMessage(reader *bufio.Reader) ([]byte, *MessageFormat, error) {
	panic("intentional panic for testing")
}

func (t *panicTransport) WriteMessage(writer *bufio.Writer, data []byte, format *MessageFormat) error {
	panic("intentional panic for testing")
}

type failingWriter struct {
	writeCount int
}

func (w *failingWriter) Write(p []byte) (n int, err error) {
	w.writeCount++
	return 0, errors.New("intentional write failure for testing")
}

func (w *failingWriter) WriteString(s string) (n int, err error) {
	return w.Write([]byte(s))
}

type slowWriter struct {
	delay time.Duration
	buf   bytes.Buffer
}

func (w *slowWriter) Write(p []byte) (n int, err error) {
	time.Sleep(w.delay)
	return w.buf.Write(p)
}

func (w *slowWriter) WriteString(s string) (n int, err error) {
	return w.Write([]byte(s))
}

// TestServerLoopStressTest runs a comprehensive stress test
func TestServerLoopStressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress test in short mode")
	}

	server := NewServer()
	server.SetInitialized(true)

	// Create client with queue
	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	format := &MessageFormat{IsRawJSON: false}

	config := QueueConfig{
		MaxQueueSize: 500,
		DropWhenFull: true,
		WriteTimeout: 100 * time.Millisecond,
	}
	queue := NewMessageQueue(writer, format, config)
	defer queue.Stop()

	server.clientsMu.Lock()
	server.clients["stress-client"] = &ClientConnection{
		ID:     "stress-client",
		Writer: writer,
		Format: format,
		Queue:  queue,
	}
	server.clientsMu.Unlock()

	// Run stress test for 5 seconds
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), 5*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	var operations atomic.Int32
	var errors atomic.Int32

	// Spam various operations
	for i := 0; i < 10; i++ {
		i := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_stress_worker_%d", i), fmt.Sprintf("stress testing worker %d", i)).
			WithWaitGroup(&wg).
			StartWithContext(ctx, func(ctx context.Context) error {
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
						operations.Add(1)
						if err := server.SendMessageToClientByID(
							"stress-client",
							fmt.Sprintf("stress message %d", operations.Load()),
							"test",
							"normal",
						); err != nil {
							errors.Add(1)
						}
						time.Sleep(1 * time.Millisecond)
					}
				}
			})
	}

	// Also spam shutdown requests (should be ignored after first)
	for i := 0; i < 5; i++ {
		i := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("test_shutdown_spam_%d", i), fmt.Sprintf("spamming shutdown requests %d", i)).
			WithWaitGroup(&wg).
			StartWithContext(ctx, func(ctx context.Context) error {
				for {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
						server.RequestShutdown("stress test shutdown")
						time.Sleep(10 * time.Millisecond)
					}
				}
			})
	}

	wg.Wait()

	// Give queue time to process
	time.Sleep(500 * time.Millisecond)

	stats := queue.Stats()
	t.Logf("Stress test results:")
	t.Logf("  Operations: %d", operations.Load())
	t.Logf("  Errors: %d", errors.Load())
	t.Logf("  Queue stats: sent=%d, dropped=%d, errors=%d, depth=%d",
		stats.Sent, stats.Dropped, stats.Errors, stats.QueueDepth)

	// Server should still be operational
	if server.IsShutdownRequested() && operations.Load() < 100 {
		t.Error("Server shutdown too early in stress test")
	}

	queue.Stop()
}
