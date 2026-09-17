package mcp

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestHandleReadError_connectionResetEndsConnectionOnly(t *testing.T) {
	server := NewServer()
	handler := HandlerFunc(nil)
	transport := NewDefaultTransport()
	processor := NewMessageProcessor(server, handler, transport)
	lifecycle := NewServerLifecycleBuilder(server)
	sc := NewServeCoordinator(server, processor, lifecycle)

	writer := bufio.NewWriter(io.Discard)
	err := sc.handleReadError(
		errors.New("read tcp 127.0.0.1:8443->127.0.0.1:56924: read: connection reset by peer"),
		nil,
		writer,
		false,
		nil,
	)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF sentinel for connection-only teardown, got %v", err)
	}
	if server.shutdownFlag.Load() == 1 {
		t.Fatal("connection reset must not shut down the shared MCP daemon")
	}
}

func TestHandleReadError_eofEndsConnectionOnly(t *testing.T) {
	server := NewServer()
	sc := NewServeCoordinator(server, NewMessageProcessor(server, HandlerFunc(nil), NewDefaultTransport()), NewServerLifecycleBuilder(server))
	err := sc.handleReadError(io.EOF, nil, bufio.NewWriter(io.Discard), false, nil)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected io.EOF, got %v", err)
	}
	if server.shutdownFlag.Load() == 1 {
		t.Fatal("EOF must not shut down the shared MCP daemon")
	}
}

func TestIsBrokenPipeError_connectionReset(t *testing.T) {
	err := errors.New("read tcp 127.0.0.1:8443->127.0.0.1:1: read: connection reset by peer")
	if !isBrokenPipeError(err) {
		t.Fatalf("expected connection reset to match isBrokenPipeError; got false for %q", err)
	}
	if isBrokenPipeError(errors.New(strings.ToLower("some other failure"))) {
		t.Fatal("unrelated error should not match")
	}
}

func TestRequestShutdown_multiClientDoesNotArmProcessShutdown(t *testing.T) {
	server := NewServer()
	server.multiClient.Store(true)
	server.RequestShutdown("client_shutdown from IDE reload")
	if server.IsShutdownRequested() {
		t.Fatal("multi-client RequestShutdown must not arm process shutdown")
	}
	server.RequestProcessShutdown("received OS signal: terminated")
	if !server.IsShutdownRequested() {
		t.Fatal("RequestProcessShutdown must arm process shutdown on multi-client daemon")
	}
}

func TestShutdownOnConnectionFault_multiClientSkipsShutdown(t *testing.T) {
	server := NewServer()
	server.multiClient.Store(true)
	server.shutdownOnConnectionFault("write error: simulated")
	if server.shutdownFlag.Load() == 1 {
		t.Fatal("multi-client write/marshal fault must not shut down daemon")
	}
	server.multiClient.Store(false)
	server.shutdownOnConnectionFault("write error: simulated stdio")
	if server.shutdownFlag.Load() != 1 {
		t.Fatal("stdio connection fault should shut down")
	}
}

func TestEndConnectionOnly_multiClientSkipsShutdown(t *testing.T) {
	server := NewServer()
	server.multiClient.Store(true)
	sc := NewServeCoordinator(server, NewMessageProcessor(server, HandlerFunc(nil), NewDefaultTransport()), NewServerLifecycleBuilder(server))
	if !sc.endConnectionOnly("test", bufio.NewWriter(io.Discard)) {
		t.Fatal("expected multi-client endConnectionOnly true")
	}
	if server.shutdownFlag.Load() == 1 {
		t.Fatal("multi-client connection end must not set shutdownFlag")
	}
}

func TestHandleClientDisconnect_multiClientPreservesTools(t *testing.T) {
	server := NewServer()
	server.multiClient.Store(true)
	server.RegisterTool("keep_me", "x", map[string]any{}, nil)
	server.SetInitialized(true)
	sc := NewServeCoordinator(server, NewMessageProcessor(server, HandlerFunc(nil), NewDefaultTransport()), NewServerLifecycleBuilder(server))
	if err := sc.handleClientDisconnect(bufio.NewWriter(io.Discard)); err != nil {
		t.Fatalf("handleClientDisconnect: %v", err)
	}
	if !server.IsInitialized() {
		t.Fatal("multi-client disconnect must not clear initialized")
	}
	if server.getToolCount() == 0 {
		t.Fatal("multi-client disconnect must not clear tools")
	}
}

func TestHandleClientDisconnect_multiClientReleasesWriter(t *testing.T) {
	server := NewServer()
	server.multiClient.Store(true)
	server.clientID = "ide-client"
	w := bufio.NewWriter(io.Discard)
	server.clients = map[string]*ClientConnection{
		"ide-client": {ID: "ide-client", Writer: w, Format: &MessageFormat{IsRawJSON: true}},
	}
	server.transportWriter = w
	sc := NewServeCoordinator(server, NewMessageProcessor(server, HandlerFunc(nil), NewDefaultTransport()), NewServerLifecycleBuilder(server))
	if err := sc.handleClientDisconnect(w); err != nil {
		t.Fatalf("handleClientDisconnect: %v", err)
	}
	if server.clients["ide-client"].Writer != nil {
		t.Fatal("multi-client disconnect must release the connection Writer")
	}
	if server.transportWriter != nil {
		t.Fatal("multi-client disconnect must clear transportWriter for that connection")
	}
}
