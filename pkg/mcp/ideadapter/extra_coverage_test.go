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

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
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
	goroutinelabels.NewGoroutine("test.ideadapter.extra_run", "adapter Run in TestExtraConfigMergeLoadAndWriteHelpers").StartSimple(func() {
		done <- a.Run(ctx)
	})
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
	goroutinelabels.NewGoroutine("test.ideadapter.fake_daemon_run", "adapter Run in TestExtraFakeDaemonInitializeForwardAndHeartbeat").StartSimple(func() {
		errCh <- adapter.Run(ctx)
	})

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
	goroutinelabels.NewGoroutine("test.ideadapter.session_heartbeat", "heartbeat loop in test").StartSimple(func() {
		sess.HeartbeatLoop(hbCtx)
	})
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
	goroutinelabels.NewGoroutine("test.ideadapter.ln_accept_close", "accept and close connection in broken session test").StartSimple(func() {
		c, aerr := ln.Accept()
		if aerr == nil {
			_ = c.Close()
		}
	})
	broken := newDaemonSession(Config{DaemonTCP: ln.Addr().String(), RequestTimeout: 200 * time.Millisecond}, logger)
	_ = broken.EnsureConnected(context.Background())
	_ = ln.Close()
	broken.Close()

	dead := newDaemonSession(Config{DaemonTCP: "127.0.0.1:1", HeartbeatInterval: time.Millisecond, RequestTimeout: 50 * time.Millisecond}, logger)
	deadCtx, deadCancel := context.WithCancel(context.Background())
	goroutinelabels.NewGoroutine("test.ideadapter.dead_heartbeat", "dead heartbeat loop in test").StartSimple(func() {
		dead.HeartbeatLoop(deadCtx)
	})
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

func TestExtraForwardDaemonNotificationsAndKeepalive(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	a := New(DefaultConfig(), logger)

	// Test forwardDaemonNotifications
	ctx, cancel := context.WithCancel(context.Background())

	a.session.notifyCh <- []byte(`{"jsonrpc":"2.0","method":"notifications/event"}`)
	a.session.notifyCh <- []byte(`{"jsonrpc":"2.0","method":"notifications/custom","params":{"k":"v"}}`)

	var out bytes.Buffer
	w := bufio.NewWriter(&out)
	queue := mcp.NewMessageQueue(w, &mcp.MessageFormat{IsRawJSON: true}, mcp.DefaultQueueConfig())
	defer queue.Stop()

	a.queue = queue

	done := make(chan struct{})
	goroutinelabels.NewGoroutine("test.ideadapter.forward_notif", "forward notifications in test").StartSimple(func() {
		a.forwardDaemonNotifications(ctx)
		close(done)
	})

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	// Test stdioKeepaliveLoop
	a.stdioKeepaliveLoop(context.Background())

	// Test handleIDEMessage with invalid json
	_ = a.handleIDEMessage(context.Background(), w, []byte(`{invalid`), nil)
	// Response with empty method
	_ = a.handleIDEMessage(context.Background(), w, []byte(`{"jsonrpc":"2.0","id":99,"result":{}}`), nil)
}

func TestExtraForwardRPCAndWriteStdout(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	a := New(DefaultConfig(), logger)

	var out bytes.Buffer
	w := bufio.NewWriter(&out)
	format := &mcp.MessageFormat{IsRawJSON: true}

	// 1. writeResult and writeError
	_ = a.writeResult(w, format, json.RawMessage(`1`), map[string]any{"ok": true})
	_ = a.writeError(w, format, json.RawMessage(`2`), mcp.InvalidParams, "bad params")
	_ = a.writeError(w, nil, nil, mcp.InternalError, "internal")

	// 2. Queue full path in writeStdout
	cfgQ := mcp.DefaultQueueConfig()
	cfgQ.MaxQueueSize = 1
	var qBuf bytes.Buffer
	queue := mcp.NewMessageQueue(bufio.NewWriter(&qBuf), format, cfgQ)
	defer queue.Stop()

	// Fill queue
	queue.Enqueue([]byte("dummy"), format, "high", nil)
	a.queue = queue

	err := a.writeStdout(w, []byte("will be dropped"), format)
	if err == nil {
		t.Error("expected message dropped by queue error")
	}

	// 3. forwardRPC with invalid params
	ctx := context.Background()
	_ = a.forwardRPC(ctx, w, format, json.RawMessage(`3`), "test_method", json.RawMessage(`{invalid-json`))
	a.stdioKeepaliveLoop(ctx)

	// 5. isExpectedDaemonDisconnect & heartbeatDropAfterConsecutiveFails
	if !isExpectedDaemonDisconnect(io.EOF) {
		t.Error("expected true for EOF")
	}
	if !isExpectedDaemonDisconnect(net.ErrClosed) {
		t.Error("expected true for ErrClosed")
	}
	if isExpectedDaemonDisconnect(nil) {
		t.Error("expected false for nil")
	}
	if isExpectedDaemonDisconnect(errors.New("other err")) {
		t.Error("expected false for other err")
	}

	if heartbeatDropAfterConsecutiveFails(1) {
		t.Error("expected false for 1 fail")
	}
	if !heartbeatDropAfterConsecutiveFails(3) {
		t.Error("expected true for 3 fails")
	}
}

func TestExtraDaemonSessionReadLoopAndHeartbeat(t *testing.T) {
	logger := logging.GetLoggerFromProfile("system")
	cfg := DefaultConfig()
	ds := newDaemonSession(cfg, logger)

	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	ds.mu.Lock()
	ds.conn = c1
	ds.reader = bufio.NewReader(c1)
	ds.writer = bufio.NewWriter(c1)
	ds.xport = mcp.NewDefaultTransport()
	ds.mu.Unlock()

	// Pending request for matched response
	ch := make(chan json.RawMessage, 1)
	ds.pendingMu.Lock()
	ds.pending[123] = ch
	ds.pendingMu.Unlock()

	readDone := make(chan struct{})
	goroutinelabels.NewGoroutine("test.readloop", "test readloop").StartSimple(func() {
		ds.readLoop()
		close(readDone)
	})

	// 1. Matched response
	_, _ = c2.Write([]byte(`{"jsonrpc":"2.0","id":123,"result":{"ok":true}}` + "\n"))
	select {
	case <-ch:
	case <-time.After(time.Second):
	}

	// 2. Unmatched numeric id response
	_, _ = c2.Write([]byte(`{"jsonrpc":"2.0","id":999,"result":{}}` + "\n"))

	// 3. Non-numeric id
	_, _ = c2.Write([]byte(`{"jsonrpc":"2.0","id":"abc","result":{}}` + "\n"))

	// 4. Notification with null id
	_, _ = c2.Write([]byte(`{"jsonrpc":"2.0","id":null,"method":"test/notify"}` + "\n"))

	// 5. Fill notification channel and trigger drop
	for len(ds.notifyCh) < cap(ds.notifyCh) {
		ds.notifyCh <- []byte("{}")
	}
	_, _ = c2.Write([]byte(`{"jsonrpc":"2.0","method":"test/drop"}` + "\n"))

	// Drain notifyCh so it doesn't block anything
	for len(ds.notifyCh) > 0 {
		<-ds.notifyCh
	}

	// 6. Close c2 to end readLoop with EOF
	_ = c2.Close()
	select {
	case <-readDone:
	case <-time.After(2 * time.Second):
	}

	// 7. HeartbeatLoop: test tick when alive is false
	hbCfg := DefaultConfig()
	hbCfg.HeartbeatInterval = 5 * time.Millisecond
	hbDS := newDaemonSession(hbCfg, logger)
	hbDS.alive.Store(false)

	hbCtx, hbCancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer hbCancel()
	hbDone := make(chan struct{})
	goroutinelabels.NewGoroutine("test.hb", "test heartbeat").StartSimple(func() {
		hbDS.HeartbeatLoop(hbCtx)
		close(hbDone)
	})
	select {
	case <-hbDone:
	case <-time.After(2 * time.Second):
	}

	// HeartbeatLoop: test tick when alive is true and pendingCount > 0
	hbDS2 := newDaemonSession(hbCfg, logger)
	hbDS2.alive.Store(true)
	hbDS2.pendingMu.Lock()
	hbDS2.pending[1] = make(chan json.RawMessage, 1)
	hbDS2.pendingMu.Unlock()

	hbCtx2, hbCancel2 := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer hbCancel2()
	hbDone2 := make(chan struct{})
	goroutinelabels.NewGoroutine("test.hb2", "test heartbeat 2").StartSimple(func() {
		hbDS2.HeartbeatLoop(hbCtx2)
		close(hbDone2)
	})
	select {
	case <-hbDone2:
	case <-time.After(2 * time.Second):
	}
}
