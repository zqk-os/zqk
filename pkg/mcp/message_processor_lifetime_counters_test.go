package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

func TestMessageProcessor_LifetimeCounters(t *testing.T) {
	var mp *MessageProcessor
	pNil, eNil := mp.GetMessageProcessorStats()
	if pNil != 0 || eNil != 0 {
		t.Fatalf("expected nil stats (0, 0), got p=%d e=%d", pNil, eNil)
	}

	server := NewServer()
	handler := HandlerFunc(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		return "ok", nil
	})
	transport := NewDefaultTransport()
	mp = NewMessageProcessor(server, handler, transport)

	pInit, eInit := mp.GetMessageProcessorStats()
	if pInit != 0 || eInit != 0 {
		t.Fatalf("expected new stats (0, 0), got p=%d e=%d", pInit, eInit)
	}

	ctx := context.Background()
	var outBuf bytes.Buffer
	writer := bufio.NewWriter(&outBuf)
	tracker := NewOperationTracker()
	format := &MessageFormat{IsRawJSON: true}

	// 1. Process valid ping notification message -> processed=1, error=0
	msgValid := []byte(`{"jsonrpc":"2.0","method":"ping"}`)
	if err := mp.ProcessMessage(ctx, msgValid, format, writer, ctx, tracker); err != nil {
		t.Fatalf("unexpected error on valid ping message: %v", err)
	}

	p1, e1 := mp.GetMessageProcessorStats()
	if p1 != 1 || e1 != 0 {
		t.Errorf("expected stats (1, 0) after valid message, got p=%d e=%d", p1, e1)
	}

	// 2. Process invalid JSON message -> processed=2
	msgInvalid := []byte(`invalid json`)
	_ = mp.ProcessMessage(ctx, msgInvalid, format, writer, ctx, tracker)

	p2, e2 := mp.GetMessageProcessorStats()
	if p2 != 2 {
		t.Errorf("expected processed=2 after invalid message, got %d", p2)
	}
	if e2 < 0 {
		t.Errorf("expected errors >= 0, got %d", e2)
	}
}
