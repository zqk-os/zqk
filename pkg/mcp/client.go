package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/telemetry"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

const defaultMCPResponseTimeout = 180 * time.Second

func mcpResponseTimeout() time.Duration {
	if s := zqkenv.MCPTimeout().Get(); s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
		if sec, err := strconv.Atoi(s); err == nil && sec > 0 {
			return time.Duration(sec) * time.Second
		}
	}
	return defaultMCPResponseTimeout
}

// Client provides a transport-agnostic interface for invoking MCP tools.
type Client struct {
	transport Transport
	format    *MessageFormat
	in        *bufio.Reader
	out       *bufio.Writer

	writeMu   sync.Mutex
	mu        sync.Mutex
	requestID int
	pending   map[int]chan JSONRPCResponse
	cancel    context.CancelFunc

	tracker telemetry.Tracker
}

// SetTracker configures a telemetry tracker for this client to record IPC latency.
func (c *Client) SetTracker(t telemetry.Tracker) {
	c.tracker = t
}

// NewClient creates a new MCP client.
func NewClient(r io.Reader, w io.Writer, transport Transport) *Client {
	ctx, cancel := context.WithCancel(context.Background()) // Background: request-or-shutdown derived
	c := &Client{
		transport: transport,
		format:    &MessageFormat{IsRawJSON: false}, // Default to Content-Length framing
		in:        bufio.NewReader(r),
		out:       bufio.NewWriter(w),
		pending:   make(map[int]chan JSONRPCResponse),
		cancel:    cancel,
	}

	goroutinelabels.NewGoroutine("mcp", "read_loop").
		StartSimple(func() {
			c.readLoop(ctx)
		})
	return c
}

// Close shuts down the client connection.
func (c *Client) Close() error {
	c.cancel()
	return nil
}

func (c *Client) readLoop(ctx context.Context) {
	defer func() {
		c.mu.Lock()
		for _, ch := range c.pending {
			close(ch)
		}
		c.pending = make(map[int]chan JSONRPCResponse)
		c.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, _, err := c.transport.ReadMessage(c.in)
		if err != nil {
			c.logToSandbox("readLoop error: %v\n", err)
			return
		}

		c.logToSandbox("readLoop read: %s\n", string(msg))

		var resp JSONRPCResponse
		if err := json.Unmarshal(msg, &resp); err != nil {
			c.logToSandbox("unmarshal error: %v\n", err)
			continue // Not a response we understand
		}

		c.mu.Lock()
		// Cast ID to float64 (default for JSON numbers) and convert to int
		var reqID int
		if idFloat, ok := resp.ID.(float64); ok {
			reqID = int(idFloat)
		} else if idInt, ok := resp.ID.(int); ok {
			reqID = idInt
		}

		ch, ok := c.pending[reqID]
		if ok {
			delete(c.pending, reqID)
		}
		c.mu.Unlock()

		if ok {
			ch <- resp
		}
	}
}

func (c *Client) registerPendingRequest() (int, chan JSONRPCResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requestID++
	id := c.requestID
	ch := make(chan JSONRPCResponse, 1)
	c.pending[id] = ch
	return id, ch
}

func (c *Client) removePendingRequest(id int) {
	c.mu.Lock()
	delete(c.pending, id)
	c.mu.Unlock()
}

func (c *Client) sendRawMessage(ctx context.Context, label, desc string, reqBytes []byte) error {
	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine(label, desc).StartSimple(func() {
		c.writeMu.Lock()
		defer c.writeMu.Unlock()
		errCh <- c.transport.WriteMessage(c.out, reqBytes, c.format)
	})

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		if err != nil {
			return errfmt.Newf("write message").Wrap(err)
		}
		return nil
	}
}

func (c *Client) sendRequest(ctx context.Context, label, desc string, id int, reqBytes []byte) error {
	if err := c.sendRawMessage(ctx, label, desc, reqBytes); err != nil {
		c.removePendingRequest(id)
		return err
	}
	return nil
}

func (c *Client) waitResponse(ctx context.Context, id int, ch <-chan JSONRPCResponse, timeout time.Duration) (*JSONRPCResponse, error) {
	var timeoutChan <-chan time.Time
	if timeout > 0 {
		timeoutChan = time.After(timeout)
	}

	select {
	case <-ctx.Done():
		c.removePendingRequest(id)
		return nil, ctx.Err()
	case resp, ok := <-ch:
		if !ok {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errfmt.Errorf("mcp connection closed")
		}
		if resp.Error != nil {
			return nil, errfmt.Errorf("mcp error: %s (code: %d)", resp.Error.Message, resp.Error.Code)
		}
		return &resp, nil
	case <-timeoutChan:
		c.removePendingRequest(id)
		return nil, errfmt.Errorf("timeout waiting for MCP response")
	}
}

// Initialize performs the MCP protocol handshake.
func (c *Client) Initialize(ctx context.Context, params InitializeParams) (*InitializeResult, error) {
	id, ch := c.registerPendingRequest()

	paramsBytes, _ := json.Marshal(params)
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "initialize",
		Params:  json.RawMessage(paramsBytes),
	}

	reqBytes, _ := json.Marshal(req)
	if err := c.sendRequest(ctx, "mcp_client_initialize", "sending initialize request to MCP server", id, reqBytes); err != nil {
		return nil, err
	}

	resp, err := c.waitResponse(ctx, id, ch, 0)
	if err != nil {
		return nil, err
	}

	var result InitializeResult
	resultBytes, _ := json.Marshal(resp.Result)
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, errfmt.Newf("unmarshal init result").Wrap(err)
	}
	return &result, nil
}

// Initialized sends the notifications/initialized message to the server, which is required
// after the Initialize response is received.
func (c *Client) Initialized(ctx context.Context) error {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "notifications/initialized",
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		return errfmt.Newf("marshal notification").Wrap(err)
	}

	return c.sendRawMessage(ctx, "mcp_client_initialized_notification", "sending initialized notification to MCP server", reqBytes)
}

// CallTool synchronously executes a tool via the MCP protocol.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (json.RawMessage, error) {
	start := time.Now()

	id, ch := c.registerPendingRequest()

	params := struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}{
		Name:      name,
		Arguments: args,
	}
	paramsBytes, _ := json.Marshal(params)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/call",
		Params:  json.RawMessage(paramsBytes),
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		c.removePendingRequest(id)
		return nil, errfmt.Newf("marshal request").Wrap(err)
	}

	if err := c.sendRequest(ctx, "mcp_client_call_tool", "sending call_tool request to MCP server", id, reqBytes); err != nil {
		return nil, err
	}

	resp, err := c.waitResponse(ctx, id, ch, mcpResponseTimeout())
	if err != nil {
		return nil, err
	}

	duration := time.Since(start)
	if c.tracker != nil {
		c.tracker.RecordIPCLatency(ctx, "mcp_call_tool", duration)
	}

	// The MCP protocol returns tool execution results wrapped in a standard schema
	// Unpack it for the caller.
	var toolResult ToolCallResult
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, errfmt.Newf("marshal raw result").Wrap(err)
	}
	if err := json.Unmarshal(resultBytes, &toolResult); err != nil {
		return nil, errfmt.Newf("unmarshal tool result").Wrap(err)
	}

	if toolResult.IsError {
		var errMsg string
		for _, content := range toolResult.Content {
			if content.Type == "text" {
				errMsg += content.Text + "\n"
			}
		}
		return nil, errfmt.Errorf("tool execution failed: %s", errMsg)
	}

	// Concatenate text content as the raw message payload
	var resultText string
	for _, content := range toolResult.Content {
		if content.Type == "text" {
			resultText += content.Text
		}
	}

	return json.RawMessage(resultText), nil
}

// ListTools retrieves the list of tools available from the MCP server.
func (c *Client) ListTools(ctx context.Context) (*ToolsListResult, error) {
	id, ch := c.registerPendingRequest()

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  "tools/list",
	}

	reqBytes, err := json.Marshal(req)
	if err != nil {
		c.removePendingRequest(id)
		return nil, errfmt.Newf("marshal request").Wrap(err)
	}

	if err := c.sendRequest(ctx, "mcp_client_list_tools", "sending list_tools request to MCP server", id, reqBytes); err != nil {
		return nil, err
	}

	resp, err := c.waitResponse(ctx, id, ch, mcpResponseTimeout())
	if err != nil {
		return nil, err
	}

	var result ToolsListResult
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, errfmt.Newf("marshal raw result").Wrap(err)
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, errfmt.Newf("unmarshal tools list result").Wrap(err)
	}

	return &result, nil
}

func (c *Client) logToSandbox(format string, args ...any) {
	if f, err := fileutil.OpenFile(".sandbox/mcp-client.log", fileutil.O_APPEND|fileutil.O_CREATE|fileutil.O_WRONLY, paths.FilePerm644); err == nil {
		_, _ = f.WriteString(fmt.Sprintf(format, args...))
		_ = f.Close()
	}
}
