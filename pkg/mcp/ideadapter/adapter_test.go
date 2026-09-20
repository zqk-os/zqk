package ideadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/objects"
)

// fakeDaemon answers initialize, tools/list, ping, events/subscribe on newline JSON.
func startFakeDaemon(t *testing.T) (addr string, closeFn func(), sawSubscribe *bool) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var sub bool
	var mu sync.Mutex
	done := make(chan struct{})
	goroutinelabels.NewGoroutine("mcp_ide_adapter_test", "fake daemon accept loop").
		StartSimple(func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					select {
					case <-done:
						return
					default:
						return
					}
				}
				conn := c
				goroutinelabels.NewGoroutine("mcp_ide_adapter_test", "fake daemon conn").
					StartSimple(func() { serveFakeDaemonConn(conn, &sub, &mu) })
			}
		})
	sawSubscribe = &sub
	return ln.Addr().String(), func() { close(done); _ = ln.Close() }, sawSubscribe
}

func serveFakeDaemonConn(c net.Conn, sawSubscribe *bool, mu *sync.Mutex) {
	defer c.Close()
	br := bufio.NewReader(c)
	bw := bufio.NewWriter(c)
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
		var req struct {
			ID     any            `json:"id"`
			Method string         `json:"method"`
			Params map[string]any `json:"params"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		switch req.Method {
		case methodInitialize:
			writeJSON(bw, map[string]any{
				rpcKeyJSONRPC:      mcp.JSONRPCVersion,
				objects.FieldKeyID: req.ID,
				rpcKeyResult: map[string]any{
					wireProtocolVersion: defaultProtocolVersion,
					objects.FieldKeyCapabilities: map[string]any{
						wireTools: map[string]any{},
						wireElicitation: map[string]any{
							objects.FieldKeyEnabled: true,
						},
					},
					wireServerInfo: map[string]any{objects.FieldKeyName: "fake", objects.FieldKeyVersion: "1"},
				},
			})
		case methodInitializedNotification:
			// no response
		case methodPing:
			writeJSON(bw, map[string]any{rpcKeyJSONRPC: mcp.JSONRPCVersion, objects.FieldKeyID: req.ID, rpcKeyResult: map[string]any{}})
		case methodEventsSubscribe:
			mu.Lock()
			*sawSubscribe = true
			mu.Unlock()
			writeJSON(bw, map[string]any{
				rpcKeyJSONRPC:      mcp.JSONRPCVersion,
				objects.FieldKeyID: req.ID,
				rpcKeyResult:       map[string]any{"subscriptionId": "t", "subscriberCount": 1},
			})
		case methodToolsList:
			writeJSON(bw, map[string]any{
				rpcKeyJSONRPC:      mcp.JSONRPCVersion,
				objects.FieldKeyID: req.ID,
				rpcKeyResult: map[string]any{
					wireTools: []map[string]any{
						{objects.FieldKeyName: "zqk_chat_send", objects.FieldKeyDescription: "chat", "inputSchema": map[string]any{objects.FieldKeyType: "object"}},
						{objects.FieldKeyName: "zqk_object_list", objects.FieldKeyDescription: "list", "inputSchema": map[string]any{objects.FieldKeyType: "object"}},
					},
				},
			})
		default:
			writeJSON(bw, map[string]any{
				rpcKeyJSONRPC:      mcp.JSONRPCVersion,
				objects.FieldKeyID: req.ID,
				rpcKeyError:        map[string]any{rpcKeyCode: mcp.MethodNotFound, rpcKeyMessage: "Method not found"},
			})
		}
	}
}

func writeJSON(bw *bufio.Writer, v any) {
	b, _ := json.Marshal(v)
	_, _ = bw.Write(append(b, '\n'))
	_ = bw.Flush()
}

func TestAdapter_InitializeNoElicitationAndToolsList(t *testing.T) {
	addr, closeFn, sawSub := startFakeDaemon(t)
	defer closeFn()

	cfg := DefaultConfig()
	cfg.DaemonTCP = addr
	cfg.HeartbeatInterval = time.Hour // avoid noise
	cfg.StdioKeepaliveIntervalRaw = "0"
	cfg.AdvertiseElicitation = false

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	defer stdinR.Close()
	defer stdoutR.Close()

	adapter := New(cfg, logging.GetLoggerFromProfile("")).WithStdio(stdinR, stdoutW)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("mcp_ide_adapter_test", "adapter Run").
		StartSimple(func() { errCh <- adapter.Run(ctx) })

	writeLine(t, stdinW, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"ide","version":"1"}}}`)
	initLine := readLine(t, stdoutR, 5*time.Second)
	var initResp struct {
		Result map[string]any `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(initLine), &initResp); err != nil {
		t.Fatalf("init unmarshal: %v body=%s", err, initLine)
	}
	if initResp.Error != nil {
		t.Fatalf("init error: %s", initResp.Error.Message)
	}
	caps, _ := initResp.Result[objects.FieldKeyCapabilities].(map[string]any)
	if caps == nil {
		t.Fatalf("missing capabilities: %s", initLine)
	}
	if _, ok := caps["elicitation"]; ok {
		t.Fatalf("IDE-facing initialize must not advertise elicitation: %s", initLine)
	}

	writeLine(t, stdinW, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	writeLine(t, stdinW, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	toolsLine := readLine(t, stdoutR, 5*time.Second)
	var toolsResp struct {
		ID     any `json:"id"`
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(toolsLine), &toolsResp); err != nil {
		t.Fatalf("tools unmarshal: %v body=%s", err, toolsLine)
	}
	if toolsResp.Error != nil {
		t.Fatalf("tools error: %s", toolsResp.Error.Message)
	}
	if idNum, ok := toolsResp.ID.(float64); !ok || int(idNum) != 2 {
		t.Fatalf("expected IDE id 2, got %#v", toolsResp.ID)
	}
	if len(toolsResp.Result.Tools) < 2 {
		t.Fatalf("expected tools from daemon, got %s", toolsLine)
	}

	deadline := time.After(2 * time.Second)
	waitTick := time.NewTicker(20 * time.Millisecond)
	defer waitTick.Stop()
waitLoop:
	for {
		if *sawSub {
			break
		}
		select {
		case <-deadline:
			break waitLoop
		case <-waitTick.C:
		}
	}
	if !*sawSub {
		t.Fatal("expected under-cover events/subscribe")
	}

	_ = stdinW.Close()
	_ = stdoutW.Close()
	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
	}
}

func TestAdapter_ControlResponsesNeverOnStdio(t *testing.T) {
	addr, closeFn, _ := startFakeDaemon(t)
	defer closeFn()

	cfg := DefaultConfig()
	cfg.DaemonTCP = addr
	cfg.HeartbeatInterval = 50 * time.Millisecond
	cfg.StdioKeepaliveIntervalRaw = "0" // host hourglass ping covered in TestAdapter_StdioKeepalivePing

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	defer stdinR.Close()
	defer stdoutR.Close()

	adapter := New(cfg, logging.GetLoggerFromProfile("")).WithStdio(stdinR, stdoutW)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	goroutinelabels.NewGoroutine("mcp_ide_adapter_test", "adapter Run control leak").
		StartSimple(func() { _ = adapter.Run(ctx) })

	writeLine(t, stdinW, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"ide","version":"1"}}}`)
	_ = readLine(t, stdoutR, 5*time.Second)

	// Collect stdout for a heartbeat window; must not see control-plane noise.
	type chunk struct {
		b   []byte
		err error
	}
	ch := make(chan chunk, 1)
	goroutinelabels.NewGoroutine("mcp_ide_adapter_test", "drain stdout").
		StartSimple(func() {
			buf, err := io.ReadAll(stdoutR)
			ch <- chunk{buf, err}
		})
	// Let heartbeat window elapse without time.Sleep (goroutine policy).
	select {
	case <-time.After(250 * time.Millisecond):
	case <-ctx.Done():
	}
	_ = stdinW.Close()
	_ = stdoutW.Close()
	cancel()
	var out string
	select {
	case c := <-ch:
		out = string(c.b)
	case <-time.After(2 * time.Second):
		t.Fatal("stdout drain timeout")
	}
	if strings.Contains(out, `"method":"ping"`) {
		t.Fatalf("control ping leaked to IDE stdio: %s", out)
	}
	// Only initialize reply is expected before client asks for more.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var env struct {
			ID any `json:"id"`
		}
		_ = json.Unmarshal([]byte(line), &env)
		if i == 0 {
			continue // initialize
		}
		if env.ID != nil {
			t.Fatalf("unexpected extra response on IDE stdio: %s", line)
		}
	}
}

func writeLine(t *testing.T, w io.Writer, s string) {
	t.Helper()
	if _, err := io.WriteString(w, s+"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func readLine(t *testing.T, r *io.PipeReader, timeout time.Duration) string {
	t.Helper()
	ch := make(chan string, 1)
	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("mcp_ide_adapter_test", "readLine").
		StartSimple(func() {
			br := bufio.NewReader(r)
			line, err := br.ReadString('\n')
			if err != nil {
				errCh <- err
				return
			}
			ch <- strings.TrimRight(line, "\r\n")
		})
	select {
	case line := <-ch:
		return line
	case err := <-errCh:
		t.Fatalf("read: %v", err)
		return ""
	case <-time.After(timeout):
		t.Fatal("read timeout")
		return ""
	}
}

func TestDefaultConfig_UsesIDEProxyClient(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.ClientInfoName != mcp.IDEProxySubscriberClientID {
		t.Fatalf("client=%q", cfg.ClientInfoName)
	}
	if cfg.AdvertiseElicitation {
		t.Fatal("default must not advertise elicitation")
	}
	if cfg.StdioKeepaliveInterval != 45*time.Second {
		t.Fatalf("stdio keepalive default=%s want 45s", cfg.StdioKeepaliveInterval)
	}
}

// IDE cancels cmd.Context after a successful tools/call; Run must stay up while stdin is open.
func TestAdapter_RunSurvivesCanceledContext(t *testing.T) {
	addr, closeFn, _ := startFakeDaemon(t)
	defer closeFn()

	cfg := DefaultConfig()
	cfg.DaemonTCP = addr
	cfg.HeartbeatInterval = time.Hour
	cfg.StdioKeepaliveIntervalRaw = "0"

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	defer stdinR.Close()
	defer stdoutR.Close()

	adapter := New(cfg, logging.GetLoggerFromProfile("")).WithStdio(stdinR, stdoutW)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("mcp_ide_adapter_test", "adapter Run survive cancel").
		StartSimple(func() { errCh <- adapter.Run(ctx) })

	writeLine(t, stdinW, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"ide","version":"1"}}}`)
	_ = readLine(t, stdoutR, 5*time.Second)
	writeLine(t, stdinW, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	writeLine(t, stdinW, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	_ = readLine(t, stdoutR, 5*time.Second)

	cancel() // IDE-style request cancel — must not kill the peer
	select {
	case err := <-errCh:
		t.Fatalf("Run exited on canceled ctx (want stay alive until stdin EOF): %v", err)
	case <-time.After(200 * time.Millisecond):
	}

	writeLine(t, stdinW, `{"jsonrpc":"2.0","id":3,"method":"tools/list","params":{}}`)
	line := readLine(t, stdoutR, 5*time.Second)
	var toolsResp struct {
		ID    any `json:"id"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &toolsResp); err != nil {
		t.Fatalf("tools unmarshal: %v body=%s", err, line)
	}
	if toolsResp.Error != nil {
		t.Fatalf("tools error after cancel: %s", toolsResp.Error.Message)
	}
	if idNum, ok := toolsResp.ID.(float64); !ok || int(idNum) != 3 {
		t.Fatalf("expected IDE id 3 after cancel, got %#v body=%s", toolsResp.ID, line)
	}

	_ = stdinW.Close()
	_ = stdoutW.Close()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not exit after stdin EOF")
	}
}

// IDE cancels cmd.Context mid-tools/call; Call must still complete under lifeCtx∩RequestTimeout.
func TestDaemonSession_CallIgnoresCanceledParent(t *testing.T) {
	addr, closeFn, _ := startFakeDaemon(t)
	defer closeFn()

	cfg := DefaultConfig()
	cfg.DaemonTCP = addr
	cfg.RequestTimeout = 5 * time.Second
	cfg.HeartbeatInterval = time.Hour

	s := newDaemonSession(cfg, logging.GetLoggerFromProfile(""))
	defer s.Close()

	parent, cancel := context.WithCancel(context.Background())
	cancel()
	if parent.Err() == nil {
		t.Fatal("expected canceled parent")
	}

	raw, err := s.Call(parent, methodToolsList, map[string]any{})
	if err != nil {
		t.Fatalf("Call with canceled parent: %v", err)
	}
	var env struct {
		Result map[string]any `json:"result"`
		Error  *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.Error != nil {
		t.Fatalf("daemon error: %s", env.Error.Message)
	}
	if env.Result == nil {
		t.Fatalf("empty result: %s", raw)
	}
}

// Close() is the control plane: it cancels lifeCtx and aborts waiters (not unbounded Background).
func TestDaemonSession_CloseCancelsInFlightCall(t *testing.T) {
	addr, closeFn, toolsCallSeen := startSlowToolsCallDaemon(t, 2*time.Second)
	defer closeFn()

	cfg := DefaultConfig()
	cfg.DaemonTCP = addr
	cfg.RequestTimeout = 5 * time.Second
	cfg.HeartbeatInterval = time.Hour

	s := newDaemonSession(cfg, logging.GetLoggerFromProfile(""))
	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("mcp_ide_adapter_test", "slow Call").
		StartSimple(func() {
			_, err := s.Call(context.Background(), methodToolsCall, map[string]any{
				objects.FieldKeyName: "slow",
			})
			errCh <- err
		})

	select {
	case <-toolsCallSeen:
	case <-time.After(2 * time.Second):
		t.Fatal("tools/call never reached daemon")
	}
	s.Close()
	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("expected Close to abort in-flight Call")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Call did not return after Close")
	}
}

// startSlowToolsCallDaemon is like startFakeDaemon but delays tools/call responses.
func startSlowToolsCallDaemon(t *testing.T, delay time.Duration) (addr string, closeFn func(), toolsCallSeen <-chan struct{}) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan struct{})
	seen := make(chan struct{}, 1)
	goroutinelabels.NewGoroutine("mcp_ide_adapter_test", "slow fake daemon accept").
		StartSimple(func() {
			for {
				c, err := ln.Accept()
				if err != nil {
					select {
					case <-done:
						return
					default:
						return
					}
				}
				conn := c
				goroutinelabels.NewGoroutine("mcp_ide_adapter_test", "slow fake daemon conn").
					StartSimple(func() { serveSlowToolsCallDaemonConn(conn, delay, seen) })
			}
		})
	return ln.Addr().String(), func() { close(done); _ = ln.Close() }, seen
}

func serveSlowToolsCallDaemonConn(c net.Conn, delay time.Duration, toolsCallSeen chan<- struct{}) {
	defer c.Close()
	br := bufio.NewReader(c)
	bw := bufio.NewWriter(c)
	for {
		line, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
		var req struct {
			ID     any    `json:"id"`
			Method string `json:"method"`
		}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		switch req.Method {
		case methodInitialize:
			writeJSON(bw, map[string]any{
				rpcKeyJSONRPC:      mcp.JSONRPCVersion,
				objects.FieldKeyID: req.ID,
				rpcKeyResult: map[string]any{
					wireProtocolVersion:          defaultProtocolVersion,
					objects.FieldKeyCapabilities: map[string]any{wireTools: map[string]any{}},
					wireServerInfo:               map[string]any{objects.FieldKeyName: "fake", objects.FieldKeyVersion: "1"},
				},
			})
		case methodInitializedNotification:
		case methodPing:
			writeJSON(bw, map[string]any{rpcKeyJSONRPC: mcp.JSONRPCVersion, objects.FieldKeyID: req.ID, rpcKeyResult: map[string]any{}})
		case methodEventsSubscribe:
			writeJSON(bw, map[string]any{
				rpcKeyJSONRPC:      mcp.JSONRPCVersion,
				objects.FieldKeyID: req.ID,
				rpcKeyResult:       map[string]any{"subscriptionId": "t", "subscriberCount": 1},
			})
		case methodToolsCall:
			select {
			case toolsCallSeen <- struct{}{}:
			default:
			}
			// Simulated handler latency (goroutine policy: no time.Sleep for waits).
			select {
			case <-time.After(delay):
			}
			writeJSON(bw, map[string]any{
				rpcKeyJSONRPC:      mcp.JSONRPCVersion,
				objects.FieldKeyID: req.ID,
				rpcKeyResult:       map[string]any{objects.FieldKeyContent: []any{}},
			})
		default:
			writeJSON(bw, map[string]any{
				rpcKeyJSONRPC:      mcp.JSONRPCVersion,
				objects.FieldKeyID: req.ID,
				rpcKeyError:        map[string]any{rpcKeyCode: mcp.MethodNotFound, rpcKeyMessage: "Method not found"},
			})
		}
	}
}
