package mcp

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
)

func TestProxyForwarder_Basic(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer l.Close()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		transport := NewDefaultTransport()
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)

		for {
			data, format, err := transport.ReadMessage(reader)
			if err != nil {
				return
			}
			resp := bytes.ReplaceAll(data, []byte("request"), []byte("response"))
			if err := transport.WriteMessage(writer, resp, format); err != nil {
				return
			}
		}
	}()

	clientIn, clientWriter := io.Pipe()
	clientReader, clientOut := io.Pipe()

	dialer := func(ctx context.Context) (net.Conn, error) {
		return net.Dial("tcp", l.Addr().String())
	}

	proxy := NewProxyForwarder(clientIn, clientOut, dialer, logging.GetLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxyDone := make(chan error, 1)
	go func() {
		proxyDone <- proxy.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	transport := NewDefaultTransport()
	outBuf := bufio.NewWriter(clientWriter)
	testMsg := []byte(`{"jsonrpc":"2.0","id":1,"method":"test_request"}`)
	err = transport.WriteMessage(outBuf, testMsg, &MessageFormat{IsRawJSON: true})
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	inBuf := bufio.NewReader(clientReader)
	data, _, err := transport.ReadMessage(inBuf)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if !bytes.Contains(data, []byte("test_response")) {
		t.Errorf("Expected response to contain test_response, got %s", string(data))
	}

	cancel()
	_ = clientWriter.Close()
	select {
	case <-proxyDone:
	case <-time.After(1 * time.Second):
		t.Error("Proxy didn't shut down gracefully")
	}
}

func TestProxyForwarder_ReconnectForwardsToolsCall(t *testing.T) {
	// Prove chat_send-style tools/call JSON-RPC survives daemon restart via proxy reconnect.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
		_ = l.Close()
	}()

	clientIn, clientWriter := io.Pipe()
	clientReader, clientOut := io.Pipe()

	var mu sync.Mutex
	getAddr := func() string { mu.Lock(); defer mu.Unlock(); return addr }
	setAddr := func(a string) { mu.Lock(); defer mu.Unlock(); addr = a }

	dialer := func(ctx context.Context) (net.Conn, error) {
		return net.Dial("tcp", getAddr())
	}

	proxy := NewProxyForwarder(clientIn, clientOut, dialer, logging.GetLogger())
	proxy.SetReconnectDelay(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = proxy.Start(ctx) }()
	time.Sleep(40 * time.Millisecond)

	l2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen2: %v", err)
	}
	defer l2.Close()
	setAddr(l2.Addr().String())

	got := make(chan []byte, 1)
	go func() {
		conn, err := l2.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		transport := NewDefaultTransport()
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		data, format, err := transport.ReadMessage(reader)
		if err != nil {
			return
		}
		got <- data
		_ = transport.WriteMessage(writer, []byte(`{"jsonrpc":"2.0","id":9,"result":{"ok":true}}`), format)
	}()

	time.Sleep(40 * time.Millisecond)

	transport := NewDefaultTransport()
	outBuf := bufio.NewWriter(clientWriter)
	toolsCall := []byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"zqk_chat_send","arguments":{"message":"via proxy"}}}`)
	if err := transport.WriteMessage(outBuf, toolsCall, &MessageFormat{IsRawJSON: true}); err != nil {
		t.Fatalf("write: %v", err)
	}

	select {
	case data := <-got:
		if !bytes.Contains(data, []byte("zqk_chat_send")) || !bytes.Contains(data, []byte("via proxy")) {
			t.Fatalf("daemon saw unexpected payload: %s", data)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tools/call on reconnected daemon")
	}

	cancel()
	_ = clientWriter.Close()
	_ = clientReader.Close()
}

func TestProxyForwarder_Reconnect(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	addr := l.Addr().String()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		_ = conn.Close()
		_ = l.Close()
	}()

	clientIn, clientWriter := io.Pipe()
	clientReader, clientOut := io.Pipe()

	var mu sync.Mutex
	getAddr := func() string { mu.Lock(); defer mu.Unlock(); return addr }
	setAddr := func(a string) { mu.Lock(); defer mu.Unlock(); addr = a }

	dialer := func(ctx context.Context) (net.Conn, error) {
		return net.Dial("tcp", getAddr())
	}

	proxy := NewProxyForwarder(clientIn, clientOut, dialer, logging.GetLogger())
	proxy.SetReconnectDelay(10 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	proxyDone := make(chan error, 1)
	go func() {
		proxyDone <- proxy.Start(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	l2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	defer l2.Close()

	setAddr(l2.Addr().String())

	go func() {
		conn, err := l2.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		transport := NewDefaultTransport()
		reader := bufio.NewReader(conn)
		writer := bufio.NewWriter(conn)
		for {
			data, format, err := transport.ReadMessage(reader)
			if err != nil {
				return
			}
			resp := bytes.ReplaceAll(data, []byte("request"), []byte("response"))
			_ = transport.WriteMessage(writer, resp, format)
		}
	}()

	time.Sleep(50 * time.Millisecond)

	transport := NewDefaultTransport()
	outBuf := bufio.NewWriter(clientWriter)
	testMsg := []byte(`{"jsonrpc":"2.0","id":2,"method":"test_request"}`)
	err = transport.WriteMessage(outBuf, testMsg, &MessageFormat{IsRawJSON: true})
	if err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	inBuf := bufio.NewReader(clientReader)
	data, _, err := transport.ReadMessage(inBuf)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if !bytes.Contains(data, []byte("test_response")) {
		t.Errorf("Expected response to contain test_response, got %s", string(data))
	}

	cancel()
	_ = clientWriter.Close()
}

func TestProxyForwarder_LifetimeCounters(t *testing.T) {
	var pf *ProxyForwarder
	f, r, e := pf.GetProxyForwarderStats()
	if f != 0 || r != 0 || e != 0 {
		t.Errorf("nil ProxyForwarder should return 0 stats, got f=%d r=%d e=%d", f, r, e)
	}

	clientIn, clientWriter := io.Pipe()
	defer clientWriter.Close()
	clientReader, clientOut := io.Pipe()
	defer clientReader.Close()

	dialer := func(ctx context.Context) (net.Conn, error) {
		c1, _ := net.Pipe()
		return c1, nil
	}

	pf = NewProxyForwarder(clientIn, clientOut, dialer, logging.GetLogger())
	f, r, e = pf.GetProxyForwarderStats()
	if f != 0 || r != 0 || e != 0 {
		t.Errorf("new ProxyForwarder should return 0 stats, got f=%d r=%d e=%d", f, r, e)
	}
}
