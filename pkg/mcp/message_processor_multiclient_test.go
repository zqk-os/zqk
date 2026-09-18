package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestMultiClient_ProcessMessageDoesNotStealTransportWriter(t *testing.T) {
	server := NewServer()
	server.multiClient.Store(true)
	server.initialized.Store(true)

	ideBuf := &threadSafeBuf{}
	ideWriter := bufio.NewWriter(ideBuf)
	server.transportWriter = ideWriter
	server.transportFormat = &MessageFormat{IsRawJSON: true}

	ephemeralWriter := bufio.NewWriter(io.Discard)
	processor := NewMessageProcessor(server, server.setupHandlers(), NewDefaultTransport())
	tracker := NewOperationTracker()

	notif, _ := json.Marshal(map[string]any{
		"jsonrpc":              "2.0",
		objects.FieldKeyMethod: notificationMethodEvent,
		"params": map[string]any{
			objects.FieldKeyType: string(EventTypeActionRequired),
			"message":            "steer",
		},
	})
	if err := processor.ProcessMessage(context.Background(), notif, &MessageFormat{IsRawJSON: true}, ephemeralWriter, context.Background(), tracker); err != nil {
		t.Fatalf("ProcessMessage: %v", err)
	}

	if server.transportWriter != ideWriter {
		t.Fatal("multi-client ProcessMessage must not replace an existing transportWriter with an ephemeral dial writer")
	}
}

func TestMultiClient_UpdateClientConnectionRebindsWriter(t *testing.T) {
	server := NewServer()
	server.multiClient.Store(true)
	server.clientID = "ide-client"
	ideWriter := bufio.NewWriter(io.Discard)
	server.clients = map[string]*ClientConnection{
		"ide-client": {
			ID:     "ide-client",
			Writer: ideWriter,
			Format: &MessageFormat{IsRawJSON: true},
			Queue:  NewMessageQueue(ideWriter, &MessageFormat{IsRawJSON: true}, DefaultQueueConfig()),
		},
	}

	processor := NewMessageProcessor(server, HandlerFunc(nil), NewDefaultTransport())
	reconnect := bufio.NewWriter(io.Discard)
	processor.updateClientConnection(reconnect, &MessageFormat{IsRawJSON: true})

	got := server.clients["ide-client"].Writer
	if got != reconnect {
		t.Fatal("multi-client must rebind client Writer to the connection currently processing")
	}
}

func TestMultiClient_UpdateClientConnectionAllowsRebindAfterRelease(t *testing.T) {
	server := NewServer()
	server.multiClient.Store(true)
	server.clientID = "ide-client"
	deadWriter := bufio.NewWriter(io.Discard)
	server.clients = map[string]*ClientConnection{
		"ide-client": {
			ID:     "ide-client",
			Writer: deadWriter,
			Format: &MessageFormat{IsRawJSON: true},
			Queue:  NewMessageQueue(deadWriter, &MessageFormat{IsRawJSON: true}, DefaultQueueConfig()),
		},
	}
	server.transportWriter = deadWriter

	server.releaseConnectionWriter(deadWriter)
	if server.clients["ide-client"].Writer != nil {
		t.Fatal("releaseConnectionWriter must clear matching client Writer")
	}
	if server.transportWriter != nil {
		t.Fatal("releaseConnectionWriter must clear matching transportWriter")
	}

	processor := NewMessageProcessor(server, HandlerFunc(nil), NewDefaultTransport())
	reconnect := bufio.NewWriter(io.Discard)
	processor.updateClientConnection(reconnect, &MessageFormat{IsRawJSON: true})
	if server.clients["ide-client"].Writer != reconnect {
		t.Fatal("multi-client must allow Writer rebind after prior connection released")
	}
}

type threadSafeBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (t *threadSafeBuf) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.b.Write(p)
}
func (t *threadSafeBuf) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.b.String()
}
func (t *threadSafeBuf) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.b.Len()
}

func TestMultiClient_SendResponseDoesNotUseForeignQueue(t *testing.T) {
	server := NewServer()
	server.multiClient.Store(true)
	server.clientID = "ide-client"
	server.initialized.Store(true)

	ideBuf := &threadSafeBuf{}
	ideWriter := bufio.NewWriter(ideBuf)
	ephemeralBuf := &threadSafeBuf{}
	ephemeralWriter := bufio.NewWriter(ephemeralBuf)

	ideQueue := NewMessageQueue(ideWriter, &MessageFormat{IsRawJSON: true}, DefaultQueueConfig())
	defer ideQueue.Stop()
	server.clients = map[string]*ClientConnection{
		"ide-client": {
			ID:     "ide-client",
			Writer: ideWriter,
			Format: &MessageFormat{IsRawJSON: true},
			Queue:  ideQueue,
		},
	}
	server.writerQueues = map[*bufio.Writer]*MessageQueue{
		ideWriter: ideQueue,
	}

	processor := NewMessageProcessor(server, HandlerFunc(nil), NewDefaultTransport())
	resp := NewResponse(42)
	resp.Result = map[string]any{"ok": true}
	if err := processor.sendResponse(resp, &MessageFormat{IsRawJSON: true}, ephemeralWriter, nil); err != nil {
		t.Fatalf("sendResponse: %v", err)
	}
	// Allow per-writer queue to flush

	time.Sleep(50 * time.Millisecond)
	ephemeralQueue := server.queueForWriter(ephemeralWriter, nil)
	if ephemeralQueue != nil {
		ephemeralQueue.Abandon()
	}

	if ideBuf.Len() != 0 {
		t.Fatalf("ephemeral response leaked onto IDE writer: %q", ideBuf.String())
	}
	if ephemeralBuf.Len() == 0 {
		t.Fatal("ephemeral response must write on the request connection")
	}
}

func TestHandleNotificationEvent_EmitsAndSentinel(t *testing.T) {
	server := NewServer()
	server.eventEmitter = NewEventEmitter(10)
	sub := newMockEventSubscriber("ide", []EventType{EventTypeActionRequired})
	server.eventEmitter.Subscribe(sub)

	_, err := server.handleNotificationEvent(context.Background(), notificationMethodEvent, json.RawMessage(`{"type":"action.required","message":"hi","fields":{"event_id":"AFE-1"}}`))
	notificationSentinel := &NotificationSentinel{}
	if !errors.As(err, &notificationSentinel) {
		t.Fatalf("expected NotificationSentinel, got %T %v", err, err)
	}
	events := sub.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 emit, got %d", len(events))
	}
	if events[0].Message != "hi" {
		t.Fatalf("message=%q", events[0].Message)
	}
}
