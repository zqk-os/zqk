package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

type mockTransport struct {
	readResp []byte
	writeReq []byte
}

func (m *mockTransport) ReadMessage(r *bufio.Reader) ([]byte, *MessageFormat, error) {
	// Block until a write happens to simulate real network delay
	time.Sleep(10 * time.Millisecond)
	return m.readResp, &MessageFormat{IsRawJSON: true}, nil
}

func (m *mockTransport) WriteMessage(w *bufio.Writer, data []byte, format *MessageFormat) error {
	m.writeReq = data
	return nil
}

func TestClient_CallTool(t *testing.T) {
	resultObj := ToolCallResult{
		Content: []Content{
			{Type: "text", Text: `{"key": "value"}`},
		},
		IsError: false,
	}
	resultBytes, _ := json.Marshal(resultObj)

	mockResp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(resultBytes),
	}
	respBytes, _ := json.Marshal(mockResp)

	transport := &mockTransport{readResp: respBytes}
	in := bytes.NewBuffer(nil)
	out := bytes.NewBuffer(nil)

	client := NewClient(in, out, transport)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := client.CallTool(ctx, "test_tool", map[string]any{"arg1": "val1"})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}

	if string(result) != `{"key": "value"}` {
		t.Errorf("Expected '{\"key\": \"value\"}', got '%s'", string(result))
	}
}

func TestClient_DisconnectDeadlock(t *testing.T) {
	t.Parallel()
	r, w := io.Pipe()
	transport := NewDefaultTransport()
	client := NewClient(r, io.Discard, transport)

	goroutinelabels.NewGoroutine("test", "pipe_closer").
		StartSimple(func() {
			time.Sleep(100 * time.Millisecond)
			_ = w.Close()
		})

	_, err := client.CallTool(context.Background(), "some_tool", nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_ConcurrentCallTool(t *testing.T) {
	t.Parallel()

	// Create a mock transport that adds a small delay to WriteMessage to force concurrency overlaps
	type delayedMockTransport struct {
		mockTransport
	}
	transport := &delayedMockTransport{}

	in := bytes.NewBuffer(nil)
	out := bytes.NewBuffer(nil)

	client := NewClient(in, out, transport)
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Launch multiple goroutines to call the tool concurrently
	// If bufio.Writer or maps aren't properly protected, this will panic or race
	errCh := make(chan error, 100)
	for i := 0; i < 100; i++ {
		idx := i
		goroutinelabels.NewGoroutine("test", "concurrent_call").
			StartSimple(func() {
				// We expect a timeout since the mock transport readResp is empty/invalid JSON for this mock
				_, err := client.CallTool(ctx, "concurrent_tool", map[string]any{"idx": idx})
				errCh <- err
			})
	}

	for i := 0; i < 100; i++ {
		select {
		case <-errCh:
			// Just receiving the result is enough; we care that it didn't panic.
		case <-time.After(3 * time.Second):
			t.Fatal("Test hung, possible deadlock or write loop")
		}
	}
}

func TestMCPResponseTimeout(t *testing.T) {
	t.Setenv(zqkenv.MCPTimeout().Name(), "")
	if got := mcpResponseTimeout(); got != defaultMCPResponseTimeout {
		t.Fatalf("expected default timeout %v, got %v", defaultMCPResponseTimeout, got)
	}

	t.Setenv(zqkenv.MCPTimeout().Name(), "45s")
	if got := mcpResponseTimeout(); got != 45*time.Second {
		t.Fatalf("expected 45s, got %v", got)
	}

	t.Setenv(zqkenv.MCPTimeout().Name(), "60")
	if got := mcpResponseTimeout(); got != 60*time.Second {
		t.Fatalf("expected 60s, got %v", got)
	}

	t.Setenv(zqkenv.MCPTimeout().Name(), "invalid")
	if got := mcpResponseTimeout(); got != defaultMCPResponseTimeout {
		t.Fatalf("expected fallback to default timeout %v, got %v", defaultMCPResponseTimeout, got)
	}
}
