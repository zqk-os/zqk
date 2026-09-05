package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
)

// TestJSONInterleavingReproduction reproduces the EXACT production error:
// "Unexpected non-whitespace character after JSON at position 64326"
// This test should FAIL if the issue exists, PASS if fixed
func TestJSONInterleavingReproduction(t *testing.T) {
	t.Parallel()
	server := NewServer()
	server.SetInitialized(true)

	// Set up transport with buffer to capture ALL output
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

	// Register tools to make response large (like 22 tools in production)
	// Make them large enough to create ~64KB response
	for i := 0; i < 22; i++ {
		toolName := fmt.Sprintf("test_tool_%d", i)
		server.toolsMu.Lock()
		server.tools[toolName] = Tool{
			Name:        toolName,
			Description: strings.Repeat("Test tool description that makes the response larger. ", 50), // Large description
			InputSchema: map[string]any{
				objects.FieldKeyType: "object",
				"properties": map[string]any{
					"param": map[string]any{
						objects.FieldKeyType:        "string",
						objects.FieldKeyDescription: strings.Repeat("A parameter with a long description to make the JSON response larger. ", 30),
					},
				},
			},
		}
		server.toolsMu.Unlock()
	}

	// Register prompts (12 prompts)
	for i := 0; i < 12; i++ {
		promptName := fmt.Sprintf("test_prompt_%d", i)
		server.promptsMu.Lock()
		server.prompts[promptName] = Prompt{
			Name:        promptName,
			Description: "Test prompt description",
			Arguments: []PromptArgument{
				{Name: "arg1", Description: "Argument description"},
			},
		}
		server.promptsMu.Unlock()
	}

	// Register many resources (271 resources)
	for i := 0; i < 271; i++ {
		uri := fmt.Sprintf("test://resource-%d", i)
		server.resourcesMu.Lock()
		server.resources[uri] = Resource{
			URI:         uri,
			Name:        fmt.Sprintf("Resource %d", i),
			Description: "Resource description",
			MimeType:    "application/json",
		}
		server.resourcesMu.Unlock()
	}

	// Set up message processor
	handler := server.setupHandlers()
	transport := NewDefaultTransport()
	processor := NewMessageProcessor(server, handler, transport)

	// Simulate EXACT production scenario: concurrent tools/list, prompts/list, resources/list
	var wg sync.WaitGroup

	// Goroutine 1: tools/list (large response ~64KB)
	goroutinelabels.NewGoroutine("test_tools_list_repro", "handling tools/list in reproduction test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			req := &JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`21`),
				Method:  "tools/list",
				Params:  json.RawMessage(`{}`),
			}
			_ = processor.handleRequest(context.TODO(), req, format, writer, nil)
		})

	// Goroutine 2: prompts/list
	goroutinelabels.NewGoroutine("test_prompts_list_repro", "handling prompts/list in reproduction test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			time.Sleep(1 * time.Millisecond) // Small delay
			req := &JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`22`),
				Method:  "prompts/list",
				Params:  json.RawMessage(`{}`),
			}
			_ = processor.handleRequest(context.TODO(), req, format, writer, nil)
		})

	// Goroutine 3: resources/list (also large)
	goroutinelabels.NewGoroutine("test_resources_list_repro", "handling resources/list in reproduction test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			time.Sleep(2 * time.Millisecond) // Small delay
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

	// Give queue time to process
	time.Sleep(200 * time.Millisecond)

	queue.Stop()

	// Flush writer
	if err := writer.Flush(); err != nil {
		t.Fatalf("Failed to flush writer: %v", err)
	}

	// Now parse the raw output exactly as the client would
	output := buf.Bytes()

	// Try to parse as Content-Length framed messages
	reader := bufio.NewReader(bytes.NewReader(output))
	transportInstance := NewDefaultTransport()

	validCount := 0
	invalidCount := 0
	var errs []string

	for {
		msg, _, err := transportInstance.ReadMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			// Check for the exact production error patterns
			errStr := err.Error()
			if strings.Contains(errStr, "Unexpected non-whitespace character") ||
				strings.Contains(errStr, "Unexpected end of JSON") ||
				strings.Contains(errStr, "Expected ':' after property name") {
				invalidCount++
				errs = append(errs, fmt.Sprintf("REPRODUCED PRODUCTION ERROR: %v", err))

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
			errs = append(errs, fmt.Sprintf("Invalid JSON: %v", errStr))

			// Check for the exact production error pattern
			if strings.Contains(errStr, "},") && strings.Contains(errStr, "level") {
				msgPreview := string(msg)
				if len(msgPreview) > 500 {
					msgPreview = msgPreview[:500]
				}
				t.Errorf("REPRODUCED INTERLEAVING ERROR: %v\nMessage: %s", err, msgPreview)
			}
		} else {
			validCount++
		}
	}

	// If we found the production errors, the test should fail
	if invalidCount > 0 {
		t.Errorf("REPRODUCED PRODUCTION ISSUE: Found %d invalid JSON messages out of %d total", invalidCount, validCount+invalidCount)
		for _, errMsg := range errs {
			t.Errorf("  - %s", errMsg)
		}
		t.Errorf("This test FAILED - the issue is still present!")
	} else {
		t.Logf("Test PASSED: All %d messages were valid JSON", validCount)
	}
}
