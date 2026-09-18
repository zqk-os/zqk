package mcp

import (
	"bufio"
	"encoding/json"
	"io"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

// Disconnect must Unsubscribe connection-bound events/subscribe entries so
// GetSubscriberCount / feed doctor mcp_subscribers stay honest across reconnects.
func TestHandleClientDisconnect_unsubscribesEventSubscriptions(t *testing.T) {
	server := NewServer()
	server.multiClient.Store(true)
	server.eventEmitter = NewEventEmitter(8)

	w := bufio.NewWriter(io.Discard)
	server.clients = map[string]*ClientConnection{
		"ide-ide-proxy": {ID: "ide-ide-proxy", Writer: w, Format: &MessageFormat{IsRawJSON: true}},
	}
	server.transportWriter = w

	params, err := json.Marshal(EventsSubscribeParams{
		EventTypes: []string{string(EventTypeActionRequired)},
		ClientID:   "ide-ide-proxy",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	res, err := server.handleEventsSubscribe(pkgctx.NewSystemContext(), "events/subscribe", params)
	if err != nil {
		t.Fatalf("events/subscribe: %v", err)
	}
	subRes, ok := res.(EventsSubscribeResult)
	if !ok || subRes.SubscriptionID == "" {
		t.Fatalf("unexpected subscribe result: %#v", res)
	}
	if got := server.eventEmitter.GetSubscriberCount(); got != 1 {
		t.Fatalf("subscriber count after subscribe = %d, want 1", got)
	}

	sc := NewServeCoordinator(server, NewMessageProcessor(server, HandlerFunc(nil), NewDefaultTransport()), NewServerLifecycleBuilder(server))
	if err := sc.handleClientDisconnect(w); err != nil {
		t.Fatalf("handleClientDisconnect: %v", err)
	}
	if got := server.eventEmitter.GetSubscriberCount(); got != 0 {
		t.Fatalf("subscriber count after disconnect = %d, want 0 (must not lie)", got)
	}
}

func TestReleaseConnectionWriter_unsubscribesEventSubscriptions(t *testing.T) {
	server := NewServer()
	server.eventEmitter = NewEventEmitter(8)
	w := bufio.NewWriter(io.Discard)
	sub := NewMCPEventSubscriber("sub_test_drop", []EventType{EventTypeActionRequired}, func([]byte) error { return nil }, 0)
	server.eventEmitter.Subscribe(sub)
	server.trackWriterSubscription(w, sub.ID())
	if got := server.eventEmitter.GetSubscriberCount(); got != 1 {
		t.Fatalf("before release count=%d", got)
	}
	server.releaseConnectionWriter(w)
	if got := server.eventEmitter.GetSubscriberCount(); got != 0 {
		t.Fatalf("after releaseConnectionWriter count=%d, want 0", got)
	}
}
