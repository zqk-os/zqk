package mcp

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
)

func TestProxyDaemon(t *testing.T) {
	// Create a dummy daemon
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer listener.Close()

	tcpAddr := listener.Addr().String()

	// Create proxy
	logger := logging.GetLoggerFromProfile("")

	r, w, _ := os.Pipe()
	stdoutR, stdoutW, _ := os.Pipe()

	proxy := &ProxyDaemon{
		tcpAddr:   tcpAddr,
		logger:    logger,
		stdin:     r,
		stdout:    stdoutW,
		transport: NewDefaultTransport(),
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go proxy.Start(ctx)

	// Accept connection
	var conn net.Conn
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var err error
		conn, err = listener.Accept()
		if err != nil {
			return
		}
	}()

	// Give proxy time to connect
	time.Sleep(100 * time.Millisecond)

	// Send a message to stdin (format as raw JSON)
	msg := `{"jsonrpc":"2.0","method":"ping","id":1}` + "\n"
	_, err = w.Write([]byte(msg))
	if err != nil {
		t.Fatalf("Failed to write to stdin: %v", err)
	}

	wg.Wait()
	if conn == nil {
		t.Fatalf("Daemon connection not established")
	}

	// Read from daemon connection
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	reader := bufio.NewReader(conn)
	daemonMsg, _, err := proxy.transport.ReadMessage(reader)
	if err != nil {
		t.Fatalf("Failed to read from daemon: %v", err)
	}

	if !bytes.Contains(daemonMsg, []byte("ping")) {
		t.Errorf("Expected ping method, got: %s", string(daemonMsg))
	}

	// Clean up
	cancel()
	w.Close()
	stdoutW.Close()
	stdoutR.Close()
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
