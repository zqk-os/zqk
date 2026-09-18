package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestJSONInterleaving reproduces the JSON parsing error:
// "Unexpected non-whitespace character after JSON at position X"
// This happens when log notifications interleave with responses
func TestJSONInterleaving(t *testing.T) {
	t.Parallel()
	server := NewServer()
	server.SetInitialized(true)

	// Set up transport with buffer to capture output
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

	server.clientIDMu.Lock()
	server.clientID = "test-client"
	server.clientIDMu.Unlock()

	// Simulate concurrent response and log notification
	// This is the scenario that causes interleaving
	var wg sync.WaitGroup

	// Goroutine 1: Send a response (like tools/list response)
	goroutinelabels.NewGoroutine("test_large_response", "sending large response in test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			// Create a large response (like tools/list with many tools)
			response := &JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`21`),
				Result: map[string]any{
					"tools": make([]map[string]any, 100), // Large response
				},
			}
			data, _ := response.Marshal()
			queue.Enqueue(data, format, "high", nil)
		})

	// Goroutine 2: Send log notification (like SendLogDebug)
	goroutinelabels.NewGoroutine("test_log_notification", "sending log notification in test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			// Small delay to increase chance of interleaving
			time.Sleep(1 * time.Millisecond)
			_ = server.SendLogDebug("Test log message", map[string]any{
				"event": "test",
			})
		})

	// Wait for both to complete
	wg.Wait()

	queue.Stop()

	// Flush writer to ensure all messages are written
	if err := writer.Flush(); err != nil {
		t.Fatalf("Failed to flush writer: %v", err)
	}

	// Read and parse all messages from buffer
	output := buf.Bytes()
	messages := strings.Split(string(output), "Content-Length: ")

	// Verify each message is valid JSON
	validMessages := 0
	invalidMessages := 0
	for i, msg := range messages {
		if i == 0 {
			continue // Skip empty first split
		}
		// Extract JSON part (after headers)
		parts := strings.SplitN(msg, "\r\n\r\n", 2)
		if len(parts) != 2 {
			t.Logf("Message %d: Invalid format (no headers separator)", i)
			invalidMessages++
			continue
		}
		jsonPart := parts[1]

		// Try to parse JSON
		var jsonObj any
		if err := json.Unmarshal([]byte(jsonPart), &jsonObj); err != nil {
			preview := jsonPart
			if len(preview) > 200 {
				preview = preview[:200]
			}
			t.Errorf("Message %d: Invalid JSON: %v\nContent: %s", i, err, preview)
			invalidMessages++
		} else {
			validMessages++
		}
	}

	if invalidMessages > 0 {
		t.Errorf("Found %d invalid JSON messages out of %d total. This indicates interleaving occurred.", invalidMessages, len(messages)-1)
		t.Logf("Valid messages: %d, Invalid messages: %d", validMessages, invalidMessages)
		preview := string(output)
		if len(preview) > 1000 {
			preview = preview[:1000]
		}
		t.Logf("Full output (first 1000 chars): %s", preview)
	}
}

// TestJSONInterleavingConcurrent reproduces the error with many concurrent operations
func TestJSONInterleavingConcurrent(t *testing.T) {
	t.Parallel()
	server := NewServer()
	server.SetInitialized(true)

	var buf bytes.Buffer
	writer := bufio.NewWriter(&buf)
	format := &MessageFormat{IsRawJSON: false}

	server.transportMu.Lock()
	server.transportWriter = writer
	server.transportFormat = format
	server.transportMu.Unlock()

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

	server.clientIDMu.Lock()
	server.clientID = "test-client"
	server.clientIDMu.Unlock()

	// Spawn many concurrent operations (like the real scenario)
	var wg sync.WaitGroup
	numOperations := 50

	// Mix of responses and log notifications
	for i := 0; i < numOperations; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("mcp_test", "concurrent json message").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				if id%2 == 0 {
					// Send response
					response := &JSONRPCResponse{
						JSONRPC: "2.0",
						ID:      json.RawMessage(`21`),
						Result:  map[string]any{objects.FieldKeyID: id},
					}
					data, _ := response.Marshal()
					queue.Enqueue(data, format, "high", nil)
				} else {
					// Send log notification
					_ = server.SendLogDebug("Test log", map[string]any{
						objects.FieldKeyID: id,
					})
				}
			}(i)
		})
	}

	wg.Wait()

	queue.Stop()

	// Flush and verify
	if err := writer.Flush(); err != nil {
		t.Fatalf("Failed to flush: %v", err)
	}

	// Parse all messages
	output := buf.Bytes()
	reader := bufio.NewReader(bytes.NewReader(output))
	transport := NewDefaultTransport()

	validCount := 0
	invalidCount := 0

	for {
		msg, _, err := transport.ReadMessage(reader)
		if err != nil {
			// EOF is expected when no more messages
			if err.Error() == "EOF" {
				break
			}
			invalidCount++
			continue
		}

		// Try to parse as JSON-RPC message
		var jsonObj map[string]any
		if err := json.Unmarshal(msg, &jsonObj); err != nil {
			msgPreview := string(msg)
			if len(msgPreview) > 200 {
				msgPreview = msgPreview[:200]
			}
			t.Errorf("Invalid JSON message: %v\nContent: %s", err, msgPreview)
			invalidCount++
		} else {
			validCount++
		}
	}

	if invalidCount > 0 {
		t.Errorf("Found %d invalid JSON messages out of %d total. Interleaving detected!", invalidCount, validCount+invalidCount)
	}
}
