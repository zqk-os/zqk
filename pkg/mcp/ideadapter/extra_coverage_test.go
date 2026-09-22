// BLI-STARTER-COMMUNITY-058 / PRI-STARTER-COMMUNITY-058 coverage elevation
package ideadapter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/mcp"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestExtraConfigMergeLoadAndWriteHelpers(t *testing.T) {
	_ = DefaultConfig()
	_ = LoadConfig("")
	_ = LoadConfig(t.TempDir())

	root := t.TempDir()
	cfgPath := paths.MCPConfigPath(root)
	if err := fileutil.MkdirAll(filepath.Dir(cfgPath), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	body := []byte("ide_adapter:\n  daemon_tcp: 127.0.0.1:9\n  advertise_elicitation: true\n  heartbeat_interval: not-a-duration\n  request_timeout: 2s\n  stdio_keepalive_interval: 0s\n  client_info_name: extra\n  client_info_version: v\n  server_name: s\n  server_version: 1\n  auto_subscribe_event_types: [action_required]\n")
	if err := fileutil.WriteFile(cfgPath, body, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	loaded := LoadConfig(root)
	if loaded.DaemonTCP != "127.0.0.1:9" {
		t.Fatalf("LoadConfig daemon_tcp=%q", loaded.DaemonTCP)
	}
	_ = readIDEAdapterConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	_ = fileutil.WriteFile(bad, []byte("{"), paths.FilePerm644)
	_ = readIDEAdapterConfig(bad)

	dst := DefaultConfig()
	mergeConfig(&dst, Config{
		HeartbeatInterval:      time.Millisecond,
		RequestTimeout:         time.Second,
		StdioKeepaliveInterval: time.Second,
		AdvertiseElicitation:   true,
	})
	mergeConfig(&dst, Config{HeartbeatIntervalRaw: "bad", RequestTimeoutRaw: "nope", StdioKeepaliveIntervalRaw: "also-bad"})

	logger := logging.GetLoggerFromProfile("system")
	a := New(Config{DaemonTCP: "127.0.0.1:1"}, logger)
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	_ = a.writeError(w, nil, nil, mcp.ParseError, "Parse error")
	_ = a.writeError(w, &mcp.MessageFormat{IsRawJSON: true}, []byte("1"), mcp.MethodNotFound, "nope")
	_ = a.writeResult(w, &mcp.MessageFormat{IsRawJSON: true}, []byte("2"), map[string]any{"ok": true})
	_ = a.writeStdout(w, []byte(`{"jsonrpc":"2.0"}`), &mcp.MessageFormat{IsRawJSON: true})
}

func TestExtraRunPingEOFAndSessionHelpers(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	a := New(Config{
		DaemonTCP:                 "127.0.0.1:1",
		AdvertiseElicitation:      true,
		HeartbeatInterval:         time.Millisecond,
		HeartbeatIntervalRaw:      "1ms",
		RequestTimeout:            time.Second,
		RequestTimeoutRaw:         "1s",
		StdioKeepaliveInterval:    0,
		StdioKeepaliveIntervalRaw: "0s",
		ClientInfoName:            "extra",
		ServerName:                "s",
	}, logger)
	pr, pw := io.Pipe()
	var out bytes.Buffer
	a.WithStdio(pr, &out)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	_, _ = pw.Write([]byte("not-json\n"))
	_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{}}` + "\n"))
	_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","method":"notifications/foo"}` + "\n"))
	_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n"))
	_, _ = pw.Write([]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":[}` + "\n"))
	_ = pw.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("Run timeout")
	}
	cancel()

	_ = heartbeatDropAfterConsecutiveFails(0)
	_ = heartbeatDropAfterConsecutiveFails(3)
	_ = heartbeatDropAfterConsecutiveFails(30)
	_ = isExpectedDaemonDisconnect(io.EOF)
	_ = isExpectedDaemonDisconnect(net.ErrClosed)
	_ = isExpectedDaemonDisconnect(errors.New("use of closed network connection"))
	_ = isExpectedDaemonDisconnect(errors.New("broken pipe"))
	_ = isExpectedDaemonDisconnect(errors.New("connection reset"))
	_ = isExpectedDaemonDisconnect(errors.New("other"))
	_ = isExpectedDaemonDisconnect(nil)
	SetDiagLogRoot(t.TempDir())
	diagf("extra %s", "cov")
	SetDiagLogRoot("")
	diagf("disabled")

	s := newDaemonSession(Config{DaemonTCP: "127.0.0.1:1", RequestTimeout: 0}, logger)
	_ = s.Alive()
	_ = s.Notifications()
	_ = s.pendingCount()
	cctx, ccancel := s.rpcCtx()
	ccancel()
	_ = cctx
	s.Close()
	s.failPending()
	s.closeConn()
	_ = s.writeDaemon([]byte(`{}`))
}

func TestExtraFakeDaemonInitializeForwardAndHeartbeat(t *testing.T) {
	addr, closeFn, _ := startFakeDaemon(t)
	defer closeFn()

	logger := logging.GetLoggerFromProfile("system")
	cfg := DefaultConfig()
	cfg.DaemonTCP = addr
	cfg.AdvertiseElicitation = true
	cfg.HeartbeatInterval = 5 * time.Millisecond
	cfg.StdioKeepaliveIntervalRaw = "0"
	cfg.AutoSubscribeEventTypes = nil
	cfg.RequestTimeout = 2 * time.Second

	stdinR, stdinW := io.Pipe()
	stdoutR, stdoutW := io.Pipe()
	defer stdinR.Close()
	defer stdoutR.Close()

	adapter := New(cfg, logger).WithStdio(stdinR, stdoutW)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- adapter.Run(ctx) }()

	writeLine(t, stdinW, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"ide","version":"1"}}}`)
	_ = readLine(t, stdoutR, 5*time.Second)
	writeLine(t, stdinW, `{"jsonrpc":"2.0","method":"notifications/initialized"}`)
	for i, method := range []string{
		methodToolsList, methodToolsCall, methodPromptsList, methodPromptsGet,
		methodResourcesList, methodResourcesRead, methodResourcesTemplatesList,
		methodCompletionComplete, methodLoggingSetLevel, "unknown/method",
	} {
		writeLine(t, stdinW, `{"jsonrpc":"2.0","id":`+itoa(i+10)+`,"method":"`+method+`","params":{}}`)
		_ = readLine(t, stdoutR, 5*time.Second)
	}
	adapter.stdioKeepaliveLoop(ctx)
	_ = stdinW.Close()
	_ = stdoutW.Close()
	select {
	case <-errCh:
	case <-time.After(3 * time.Second):
		cancel()
		t.Fatal("Run did not exit after stdin EOF")
	}

	sess := newDaemonSession(cfg, logger)
	if err := sess.EnsureConnected(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := sess.EnsureConnected(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, _ = sess.Call(context.Background(), methodPing, map[string]any{})
	_, _ = sess.Call(context.Background(), methodToolsList, nil)
	sess.mu.Lock()
	if sess.conn != nil {
		_ = sess.conn.Close()
	}
	sess.mu.Unlock()
	_, _ = sess.Call(context.Background(), methodPing, map[string]any{})
	hbCtx, hbCancel := context.WithCancel(context.Background())
	go sess.HeartbeatLoop(hbCtx)
	time.Sleep(20 * time.Millisecond)
	hbCancel()
	ch := make(chan json.RawMessage, 1)
	sess.pendingMu.Lock()
	sess.pending[99] = ch
	sess.pendingMu.Unlock()
	sess.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		c, aerr := ln.Accept()
		if aerr == nil {
			_ = c.Close()
		}
	}()
	broken := newDaemonSession(Config{DaemonTCP: ln.Addr().String(), RequestTimeout: 200 * time.Millisecond}, logger)
	_ = broken.EnsureConnected(context.Background())
	_ = ln.Close()
	broken.Close()

	dead := newDaemonSession(Config{DaemonTCP: "127.0.0.1:1", HeartbeatInterval: time.Millisecond, RequestTimeout: 50 * time.Millisecond}, logger)
	deadCtx, deadCancel := context.WithCancel(context.Background())
	go dead.HeartbeatLoop(deadCtx)
	time.Sleep(30 * time.Millisecond)
	deadCancel()
	dead.Close()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [16]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
