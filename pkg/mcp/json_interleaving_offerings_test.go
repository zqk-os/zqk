package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestJSONInterleavingOfferings reproduces the exact error scenario:
// "Unexpected non-whitespace character after JSON at position 64326"
// This happens during initial offerings fetch (tools/list, prompts/list, resources/list)
func TestJSONInterleavingOfferings(t *testing.T) {
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

	// Create client with queue (simulating "Storing stdio client project-0-zqk-zqk-mcp")
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

	// Register some tools to make tools/list response large (like 22 tools)
	// Use internal registration to bypass security checks in test
	for i := 0; i < 22; i++ {
		toolName := fmt.Sprintf("test_tool_%d", i)
		server.toolsMu.Lock()
		server.tools[toolName] = Tool{
			Name:        toolName,
			Description: "Test tool description that makes the response larger",
			InputSchema: map[string]any{
				objects.FieldKeyType: "object",
				"properties": map[string]any{
					"param": map[string]any{
						objects.FieldKeyType:        "string",
						objects.FieldKeyDescription: "A parameter with a long description to make the JSON response larger",
					},
				},
			},
		}
		server.toolsMu.Unlock()
	}

	// Register some prompts (like 12 prompts)
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

	// Register many resources (like 271 resources)
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

	// Set up message processor to handle requests properly
	handler := server.setupHandlers()
	transport := NewDefaultTransport()
	processor := NewMessageProcessor(server, handler, transport)

	// Simulate the exact sequence: "Connected to stdio server, fetching offerings"
	// This triggers concurrent tools/list, prompts/list, resources/list calls
	var wg sync.WaitGroup

	// Goroutine 1: tools/list (large response ~64KB)
	goroutinelabels.NewGoroutine("test_tools_list_offerings", "handling tools/list in offerings test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			req := &JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`21`),
				Method:  "tools/list",
				Params:  json.RawMessage(`{}`),
			}
			// Process through message processor (sends response via queue)
			_ = processor.handleRequest(context.TODO(), req, format, writer, nil)
		})

	// Goroutine 2: prompts/list
	goroutinelabels.NewGoroutine("test_prompts_list_offerings", "handling prompts/list in offerings test").
		WithWaitGroup(&wg).
		StartSimple(func() {
			time.Sleep(1 * time.Millisecond) // Small delay to increase interleaving chance
			req := &JSONRPCRequest{
				JSONRPC: "2.0",
				ID:      json.RawMessage(`22`),
				Method:  "prompts/list",
				Params:  json.RawMessage(`{}`),
			}
			_ = processor.handleRequest(context.TODO(), req, format, writer, nil)
		})

	// Goroutine 3: resources/list (also large)
	goroutinelabels.NewGoroutine("test_resources_list_offerings", "handling resources/list in offerings test").
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

	// Also simulate log notifications that might be sent concurrently
	// (like SendLogDebug calls from handleRequest)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		goroutinelabels.NewGoroutine("mcp_test", "concurrent log during offerings").StartSimple(func() {
			func(id int) {
				defer wg.Done()
				time.Sleep(time.Duration(id) * time.Millisecond)
				_ = server.SendLogDebug("Test log during offerings", map[string]any{
					objects.FieldKeyID: id,
				})
			}(i)
		})
	}

	// Wait for all operations
	wg.Wait()

	// Give queue time to process all messages
	time.Sleep(100 * time.Millisecond)

	queue.Stop()

	// Flush writer
	if err := writer.Flush(); err != nil {
		t.Fatalf("Failed to flush writer: %v", err)
	}

	// Parse all messages and verify they're valid JSON
	output := buf.Bytes()
	reader := bufio.NewReader(bytes.NewReader(output))
	transportInstance := NewDefaultTransport()

	validCount := 0
	invalidCount := 0
	var firstError error
	var firstErrorContent string

	for {
		msg, _, err := transportInstance.ReadMessage(reader)
		if err != nil {
			// EOF is expected when no more messages
			if err.Error() == "EOF" || err.Error() == "unexpected EOF" {
				break
			}
			invalidCount++
			if firstError == nil {
				firstError = err
				firstErrorContent = string(output)
				if len(firstErrorContent) > 1000 {
					firstErrorContent = firstErrorContent[:1000]
				}
			}
			continue
		}

		// Try to parse as JSON-RPC message
		var jsonObj map[string]any
		if err := json.Unmarshal(msg, &jsonObj); err != nil {
			invalidCount++
			if firstError == nil {
				firstError = err
				msgPreview := string(msg)
				if len(msgPreview) > 500 {
					msgPreview = msgPreview[:500]
				}
				firstErrorContent = msgPreview
			}
			msgPreview := string(msg)
			if len(msgPreview) > 200 {
				msgPreview = msgPreview[:200]
			}
			t.Errorf("Invalid JSON message: %v\nContent preview: %s", err, msgPreview)
		} else {
			validCount++
		}
	}

	if invalidCount > 0 {
		t.Errorf("REPRODUCED ISSUE: Found %d invalid JSON messages out of %d total", invalidCount, validCount+invalidCount)
		t.Errorf("First error: %v", firstError)
		t.Errorf("Error content preview: %s", firstErrorContent)
		t.Errorf("This test should FAIL to reproduce the production issue")
	}
}
