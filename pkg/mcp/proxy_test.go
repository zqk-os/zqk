package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestProxyDaemon(t *testing.T) {
	// Create a dummy daemon
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	tcpAddr := listener.Addr().String()
	logger := logging.GetLoggerFromProfile("")

	r, w, _ := os.Pipe()
	stdoutR, stdoutW, _ := os.Pipe()
	defer r.Close()
	defer w.Close()
	defer stdoutR.Close()
	defer stdoutW.Close()

	proxy := &ProxyDaemon{
		tcpAddr:         tcpAddr,
		logger:          logger,
		stdin:           r,
		stdout:          stdoutW,
		stdioTransport:  NewDefaultTransport(),
		daemonTransport: NewDefaultTransport(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Accept one dial from the proxy (readFromDaemon owns the conn afterward).
	accepted := make(chan struct{}, 1)
	go func() {
		c, err := listener.Accept()
		if err != nil {
			return
		}
		accepted <- struct{}{}
		// Keep conn open until test ends so proxy does not spin-reconnect.
		<-ctx.Done()
		_ = c.Close()
	}()

	go proxy.Start(ctx)

	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not dial daemon")
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && !proxy.IsDaemonAlive() {
		time.Sleep(10 * time.Millisecond)
	}
	if !proxy.IsDaemonAlive() {
		t.Fatal("expected proxy to mark daemon alive after dial")
	}

	msg := `{"jsonrpc":"2.0","method":"ping","id":1}` + "\n"
	if _, err = w.Write([]byte(msg)); err != nil {
		t.Fatalf("Failed to write to stdin: %v", err)
	}
	// Allow forwardToDaemon to run; do not read the daemon conn (proxy owns it).
	time.Sleep(150 * time.Millisecond)
	_ = w.Close() // unblock Start's stdin ReadMessage
}

func TestStampIDEInitializeClientInfo_rewritesNonHuman(t *testing.T) {
	in := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"mcp","version":"1.0"}}}`)
	out := stampIDEInitializeClientInfo(in)
	var envelope map[string]any
	if err := json.Unmarshal(out, &envelope); err != nil {
		t.Fatal(err)
	}
	params := envelope["params"].(map[string]any)
	ci := params["clientInfo"].(map[string]any)
	if ci[objects.FieldKeyName] != ideProxySubscriberClientID {
		t.Fatalf("name=%v want %s", ci[objects.FieldKeyName], ideProxySubscriberClientID)
	}
	if ci["originalClientName"] != "mcp" {
		t.Fatalf("originalClientName=%v", ci["originalClientName"])
	}
	// already human — unchanged name
	in2 := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"clientInfo":{"name":"ide-seat-01"}}}`)
	out2 := stampIDEInitializeClientInfo(in2)
	var e2 map[string]any
	_ = json.Unmarshal(out2, &e2)
	ci2 := e2["params"].(map[string]any)["clientInfo"].(map[string]any)
	if ci2[objects.FieldKeyName] != "ide-seat-01" {
		t.Fatalf("expected preserve ide-seat-01, got %v", ci2[objects.FieldKeyName])
	}
}

func TestProxyDaemon_ensureIDEEventSubscription(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	type result struct {
		method string
		raw    string
	}
	got := make(chan result, 1)
	go func() {
		c, err := listener.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		br := bufio.NewReader(c)
		line, err := br.ReadBytes('\n')
		if err != nil {
			return
		}
		var rpc struct {
			Method string `json:"method"`
			Params struct {
				ClientID   string   `json:"clientId"`
				EventTypes []string `json:"eventTypes"`
			} `json:"params"`
		}
		_ = json.Unmarshal(line, &rpc)
		got <- result{method: rpc.Method, raw: string(line)}
		if rpc.Method == "events/subscribe" && rpc.Params.ClientID == ideProxySubscriberClientID {
			// ack so proxy swallow path can be exercised by a fuller test later
			_, _ = c.Write([]byte(`{"jsonrpc":"2.0","id":` + proxyEventsSubscribeID + `,"result":{"subscriptionId":"t"}}` + "\n"))
		}
	}()

	proxy := NewProxyDaemon(listener.Addr().String(), logging.GetLoggerFromProfile(""))
	conn, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	proxy.mu.Lock()
	proxy.conn = conn
	proxy.daemonAlive.Store(true)
	proxy.mu.Unlock()

	proxy.ensureIDEEventSubscription()
	if !proxy.ideSubscribed.Load() {
		t.Fatal("expected ideSubscribed after ensureIDEEventSubscription")
	}
	// idempotent
	proxy.ensureIDEEventSubscription()

	select {
	case r := <-got:
		if r.method != "events/subscribe" {
			t.Fatalf("method=%q want events/subscribe body=%s", r.method, r.raw)
		}
		if !bytes.Contains([]byte(r.raw), []byte(ideProxySubscriberClientID)) {
			t.Fatalf("missing clientId in %s", r.raw)
		}
		if !bytes.Contains([]byte(r.raw), []byte("action.required")) {
			t.Fatalf("missing action.required in %s", r.raw)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("daemon did not receive events/subscribe")
	}
}

func TestProxyDaemon_IsDaemonAliveAndTCPAddr(t *testing.T) {
	proxy := NewProxyDaemon("127.0.0.1:9999", logging.GetLoggerFromProfile(""))
	if proxy.GetTCPAddr() != "127.0.0.1:9999" {
		t.Errorf("Expected TCP addr 127.0.0.1:9999, got %s", proxy.GetTCPAddr())
	}
	if proxy.IsDaemonAlive() {
		t.Error("Expected IsDaemonAlive to be false initially")
	}
}

func TestProxyDaemon_forwardFailsWhenDaemonDown(t *testing.T) {
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdoutR.Close()
	defer stdoutW.Close()

	proxy := &ProxyDaemon{
		tcpAddr:         "127.0.0.1:1", // nothing listening
		logger:          logging.GetLoggerFromProfile(""),
		stdout:          stdoutW,
		stdioTransport:  NewDefaultTransport(),
		daemonTransport: NewDefaultTransport(),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	start := time.Now()
	done := make(chan struct{})
	go func() {
		proxy.forwardToDaemon(ctx, []byte(`{"jsonrpc":"2.0","id":7,"method":"initialize","params":{}}`), &MessageFormat{IsRawJSON: true})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("forwardToDaemon hung with daemon down; IDE would stay in loading")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("expected fail-fast on connection refused (~250ms dial), took %s", elapsed)
	}

	_ = stdoutW.Close()
	buf := make([]byte, 4096)
	n, _ := stdoutR.Read(buf)
	out := string(buf[:n])
	if !bytes.Contains([]byte(out), []byte(`"id":7`)) && !bytes.Contains([]byte(out), []byte(`"id": 7`)) {
		// framed body may be after Content-Length
		if !bytes.Contains([]byte(out), []byte("daemon unavailable")) {
			t.Fatalf("expected daemon-unavailable error on stdout, got %q", out)
		}
	}
	if !bytes.Contains([]byte(out), []byte("daemon unavailable")) {
		t.Fatalf("expected unavailable message, got %q", out)
	}
}

func TestProxyDaemon_ProbeDaemon(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	tcpAddr := listener.Addr().String()
	proxy := NewProxyDaemon(tcpAddr, logging.GetLoggerFromProfile(""))

	if !proxy.ProbeDaemon(500 * time.Millisecond) {
		t.Error("Expected ProbeDaemon to return true for listening address")
	}

	listener.Close()

	if proxy.ProbeDaemon(100 * time.Millisecond) {
		t.Error("Expected ProbeDaemon to return false after listener closed")
	}
}

func TestProxyDaemon_ProbeDaemonAndSync(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	tcpAddr := listener.Addr().String()
	proxy := NewProxyDaemon(tcpAddr, logging.GetLoggerFromProfile(""))

	if proxy.IsDaemonAlive() {
		t.Error("Expected initial IsDaemonAlive to be false")
	}

	if !proxy.ProbeDaemonAndSync(500 * time.Millisecond) {
		t.Error("Expected ProbeDaemonAndSync to return true for listening address")
	}

	if !proxy.IsDaemonAlive() {
		t.Error("Expected IsDaemonAlive to be true after successful sync probe")
	}

	listener.Close()

	if proxy.ProbeDaemonAndSync(100 * time.Millisecond) {
		t.Error("Expected ProbeDaemonAndSync to return false after listener closed")
	}

	if proxy.IsDaemonAlive() {
		t.Error("Expected IsDaemonAlive to be false after failed sync probe")
	}
}

func TestProxyDaemon_LifetimeCounters(t *testing.T) {
	var pd *ProxyDaemon
	hb, hbf, rec := pd.GetProxyDaemonStats()
	if hb != 0 || hbf != 0 || rec != 0 {
		t.Fatalf("expected nil ProxyDaemon stats (0, 0, 0), got hb=%d hbf=%d rec=%d", hb, hbf, rec)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	pd = NewProxyDaemon(listener.Addr().String(), logging.GetLoggerFromProfile(""))
	hb, hbf, rec = pd.GetProxyDaemonStats()
	if hb != 0 || hbf != 0 || rec != 0 {
		t.Fatalf("expected new ProxyDaemon stats (0, 0, 0), got hb=%d hbf=%d rec=%d", hb, hbf, rec)
	}
}

func TestProxyDaemon_PublishDaemonEvent(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	pd := NewProxyDaemon(listener.Addr().String(), logging.GetLoggerFromProfile(""))

	errCh := make(chan error, 1)
	msgCh := make(chan []byte, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		msg, _, rErr := pd.daemonTransport.ReadMessage(r)
		if rErr != nil {
			errCh <- rErr
			return
		}
		msgCh <- msg
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := pd.PublishDaemonEvent(ctx, "steer msg", "tpm", "AFE-100"); err != nil {
		t.Fatalf("PublishDaemonEvent: %v", err)
	}

	select {
	case err := <-errCh:
		t.Fatalf("listener error: %v", err)
	case msg := <-msgCh:
		if !bytes.Contains(msg, []byte("notifications/event")) {
			t.Errorf("expected notifications/event method, got: %s", string(msg))
		}
		if !bytes.Contains(msg, []byte("AFE-100")) {
			t.Errorf("expected event_id AFE-100, got: %s", string(msg))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for published daemon event")
	}
}

func TestProxyDaemon_IndestructibleConnection(t *testing.T) {
	// Create a dummy daemon
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}

	tcpAddr := listener.Addr().String()
	logger := logging.GetLoggerFromProfile("")

	r, w, _ := os.Pipe()
	stdoutR, stdoutW, _ := os.Pipe()
	defer r.Close()
	defer w.Close()
	defer stdoutR.Close()
	defer stdoutW.Close()

	proxy := &ProxyDaemon{
		tcpAddr:         tcpAddr,
		logger:          logger,
		stdin:           r,
		stdout:          stdoutW,
		stdioTransport:  NewDefaultTransport(),
		daemonTransport: NewDefaultTransport(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Accept one dial from the proxy
	connChan := make(chan net.Conn, 1)
	go func() {
		c, err := listener.Accept()
		if err == nil {
			connChan <- c
		}
	}()

	go proxy.Start(ctx)

	var firstConn net.Conn
	select {
	case firstConn = <-connChan:
	case <-time.After(2 * time.Second):
		t.Fatal("proxy did not dial daemon initially")
	}

	// Close the first connection to simulate daemon death
	firstConn.Close()

	// Proxy should attempt to reconnect. Start a new accept loop.
	go func() {
		c, err := listener.Accept()
		if err == nil {
			connChan <- c
		}
	}()

	// Verify the proxy reconnects (is indestructible)
	select {
	case secondConn := <-connChan:
		secondConn.Close()
	case <-time.After(3 * time.Second):
		t.Fatal("proxy did not reconnect after daemon death (not indestructible)")
	}

	listener.Close()
}
