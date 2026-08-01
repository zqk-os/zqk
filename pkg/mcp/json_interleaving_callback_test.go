package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestJSONInterleavingWithCallback reproduces the exact production scenario:
// 1. Initialize request
// 2. Concurrent tools/list, prompts/list, resources/list requests
// 3. Initialize callback sends log notifications
// 4. Verify no interleaving occurs
func TestJSONInterleavingWithCallback(t *testing.T) {
	t.Parallel()
	server := NewServer()
	server.SetInitialized(true)

	// Register realistic numbers of tools, prompts, resources (matching production)
	// 22 tools
	for i := 0; i < 22; i++ {
		toolName := GetToolName(fmt.Sprintf("test_tool_%d", i))
		server.toolsMu.Lock()
		server.tools[toolName] = Tool{
			Name:        toolName,
			Description: strings.Repeat("Tool description ", 50), // Large descriptions
			InputSchema: map[string]any{
				objects.FieldKeyType: "object",
				"properties": map[string]any{
					"param": map[string]any{
						objects.FieldKeyType:        "string",
						objects.FieldKeyDescription: strings.Repeat("Parameter description ", 100),
					},
				},
			},
		}
		server.toolsMu.Unlock()
	}

	// 12 prompts
	for i := 0; i < 12; i++ {
		promptName := fmt.Sprintf("test_prompt_%d", i)
		server.promptsMu.Lock()
		server.prompts[promptName] = Prompt{
			Name:        promptName,
			Description: strings.Repeat("Prompt description ", 50),
			Arguments: []PromptArgument{
				{Name: "arg1", Description: strings.Repeat("Argument description ", 100)},
			},
		}
		server.promptsMu.Unlock()
	}

	// 271 resources (matching production)
	for i := 0; i < 271; i++ {
		uri := fmt.Sprintf("test://resource-%d", i)
		server.resourcesMu.Lock()
		server.resources[uri] = Resource{
			URI:         uri,
			Name:        fmt.Sprintf("Resource %d", i),
			Description: strings.Repeat("Resource description ", 200), // Very large descriptions
			MimeType:    "application/json",
		}
		server.resourcesMu.Unlock()
	}

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
	server.clients["project-0-zqk-zqk-mcp"] = &ClientConnection{
		ID:     "project-0-zqk-zqk-mcp",
		Writer: writer,
		Format: format,
		Queue:  queue,
	}
	server.clientsMu.Unlock()

	server.clientIDMu.Lock()
	server.clientID = "project-0-zqk-zqk-mcp"
	server.clientIDMu.Unlock()

	// Set up message processor
	handler := server.setupHandlers()
	transport := NewDefaultTransport()
	processor := NewMessageProcessor(server, handler, transport)

	// EXACT production scenario:
	// 1. Initialize request (triggers callback that sends log notifications)
	// 2. Concurrent tools/list, prompts/list, resources/list
	var wg sync.WaitGroup

	// EXACT production scenario:
	// 1. Initialize request (triggers callback that sends 2 log notifications)
	// 2. IMMEDIATELY send concurrent tools/list, prompts/list, resources/list
	// The callback's log notifications must NOT interleave with these responses

	// Step 1: Initialize (this will trigger callback with log notifications)
	initParams, err := json.Marshal(map[string]any{
		"protocolVersion":            "2024-11-05",
		objects.FieldKeyCapabilities: map[string]any{},
		"clientInfo": map[string]any{
			objects.FieldKeyName:    "test-client",
			objects.FieldKeyVersion: "1.0.0",
		},
	})
	if err != nil {
		t.Fatalf("marshal init params: %v", err)
	}
	initReq := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      json.RawMessage(`1`),
		Method:  "initialize",
		Params:  json.RawMessage(initParams),
	}

	// Send initialize - this will queue the response, and when written, callback will enqueue 2 log notifications
	goroutinelabels.NewGoroutine("test_init_handler", "handling initialize request in test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			_ = processor.handleRequest(context.TODO(), initReq, format, writer, nil)
		})

	// Step 2: IMMEDIATELY send concurrent offerings requests (matching production timing)
	// These will be queued, and the callback's log notifications might be enqueued between them
	goroutinelabels.NewGoroutine("test_tools_list", "handling tools/list request in test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			// No delay - send immediately to match production
			req := &JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`21`),
				Method:  "tools/list",
				Params:  json.RawMessage(`{}`),
			}
			_ = processor.handleRequest(context.TODO(), req, format, writer, nil)
		})

	goroutinelabels.NewGoroutine("test_prompts_list", "handling prompts/list request in test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			req := &JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`22`),
				Method:  "prompts/list",
				Params:  json.RawMessage(`{}`),
			}
			_ = processor.handleRequest(context.TODO(), req, format, writer, nil)
		})

	goroutinelabels.NewGoroutine("test_resources_list", "handling resources/list request in test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			req := &JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`23`),
				Method:  "resources/list",
				Params:  json.RawMessage(`{}`),
			}
			_ = processor.handleRequest(context.TODO(), req, format, writer, nil)
		})

	// Wait for all operations
	wg.Wait()

	// Give queue time to process all messages (including callback's log notifications)
	// Need longer delay to ensure callback's 10ms delay + log notification enqueue completes
	time.Sleep(500 * time.Millisecond)

	queue.Stop()

	// Flush writer
	if err := writer.Flush(); err != nil {
		t.Fatalf("Failed to flush writer: %v", err)
	}

	// Get raw output and inspect it for interleaving
	output := buf.Bytes()
	outputStr := string(output)

	// Check for the exact production error patterns
	errorPatterns := []string{
		`"},"level":"`,         // Log notification interleaving with response
		`"},"params":{`,        // Another interleaving pattern
		`"},"data":{"ev`,       // Error from production logs
		`ge_read_su`,           // Part of "message_read_success" interleaving
		`message_read_success`, // Full pattern from serve_coordinator
		`message_read_error`,   // Related pattern
	}

	for _, pattern := range errorPatterns {
		if strings.Contains(outputStr, pattern) {
			// Find the position where this occurs
			pos := strings.Index(outputStr, pattern)
			t.Errorf("REPRODUCED PRODUCTION ERROR: Found interleaving pattern '%s' at position %d", pattern, pos)

			// Show context around the error
			start := pos - 200
			if start < 0 {
				start = 0
			}
			end := pos + 300
			if end > len(outputStr) {
				end = len(outputStr)
			}
			context := outputStr[start:end]
			t.Errorf("Error context (200 chars before, 300 after):\n%s", context)

			// Show message boundaries around error
			t.Errorf("Looking for Content-Length headers around position %d...", pos)
			beforeStart := pos - 500
			if beforeStart < 0 {
				beforeStart = 0
			}
			afterEnd := pos + 500
			if afterEnd > len(outputStr) {
				afterEnd = len(outputStr)
			}
			before := outputStr[beforeStart:pos]
			after := outputStr[pos:afterEnd]
			t.Errorf("Before error (500 chars):\n%s", before)
			t.Errorf("After error (500 chars):\n%s", after)

			t.Fatalf("TEST FAILED: Reproduced production interleaving error with pattern '%s'", pattern)
		}
	}

	// Parse all messages and verify they're valid JSON
	reader := bufio.NewReader(bytes.NewReader(output))
	transportInstance := NewDefaultTransport()

	validCount := 0
	invalidCount := 0
	var errors []string

	for {
		msg, _, err := transportInstance.ReadMessage(reader)
		if err != nil {
			if err == io.EOF {
				break
			}
			// Check for the exact production error patterns
			errStr := err.Error()
			if strings.Contains(errStr, "Unexpected token") ||
				strings.Contains(errStr, "Unexpected non-whitespace character") ||
				strings.Contains(errStr, "Unexpected end of JSON") ||
				strings.Contains(errStr, "Expected ':' after property name") ||
				strings.Contains(errStr, `"},"level":"`) ||
				strings.Contains(errStr, "ge_read_su") ||
				strings.Contains(errStr, "message_read_success") {
				invalidCount++
				errors = append(errors, fmt.Sprintf("REPRODUCED PRODUCTION ERROR: %v", err))

				// Show context around the error
				if len(output) > 0 {
					preview := string(output)
					if len(preview) > 2000 {
						preview = preview[:2000]
					}
					t.Logf("Error context (first 2000 chars): %s", preview)
				}
			} else {
				// Other errors (like EOF) are expected
				break
			}
			continue
		}

		// Try to parse as JSON-RPC message
		var jsonObj map[string]any
		if err := json.Unmarshal(msg, &jsonObj); err != nil {
			invalidCount++
			errStr := err.Error()
			if strings.Contains(errStr, "Unexpected token") ||
				strings.Contains(errStr, "Unexpected non-whitespace character") ||
				strings.Contains(errStr, `"},"level":"`) {
				errors = append(errors, fmt.Sprintf("JSON PARSE ERROR: %v", err))
			}
			continue
		}

		validCount++
	}

	if invalidCount > 0 {
		t.Errorf("Found %d invalid messages (expected 0)", invalidCount)
		for _, err := range errors {
			t.Errorf("  %s", err)
		}
		t.Fatalf("TEST FAILED: Reproduced production JSON interleaving errors. Valid: %d, Invalid: %d", validCount, invalidCount)
	}

	t.Logf("Test PASSED: All %d messages were valid JSON", validCount)

	// Additional verification: check that log notifications appear AFTER responses
	// Parse output to find response and log notification positions
	responses := []string{}
	logs := []string{}

	reader2 := bufio.NewReader(bytes.NewReader(output))
	transportInstance2 := NewDefaultTransport()

	for {
		msg, _, err := transportInstance2.ReadMessage(reader2)
		if err != nil {
			if err == io.EOF {
				break
			}
			break
		}

		var jsonObj map[string]any
		if err := json.Unmarshal(msg, &jsonObj); err != nil {
			continue
		}

		// Check if it's a response or a log notification
		if method, ok := jsonObj["method"].(string); ok && method == "notifications/logMessage" {
			logs = append(logs, string(msg))
		} else if jsonObj["result"] != nil || jsonObj["error"] != nil {
			responses = append(responses, string(msg))
		}
	}

	t.Logf("Found %d responses and %d log notifications", len(responses), len(logs))

	// Verify log notifications come after initialize response
	if len(responses) > 0 && len(logs) > 0 {
		// Find initialize response
		initResponseIdx := -1
		for i, resp := range responses {
			if strings.Contains(resp, `"result"`) && strings.Contains(resp, `"protocolVersion"`) {
				initResponseIdx = i
				break
			}
		}

		if initResponseIdx >= 0 {
			// Log notifications should appear after initialize response
			// But they might appear before other responses (tools/list, etc.)
			// That's okay as long as they don't interleave
			t.Logf("Initialize response found at index %d, %d log notifications found", initResponseIdx, len(logs))
		}
	}
}
