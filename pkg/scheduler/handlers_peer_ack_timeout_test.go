package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/agentfeed"
)

func TestPeerAckTimeoutAuditHandler_Execute(t *testing.T) {
	root := t.TempDir()

	// Register a peer ack await in the temporary project root
	_, err := agentfeed.RegisterPeerAckAwait(root, agentfeed.PeerAckAwaitInput{
		EventID:     "AFE-123",
		FromAgentID: "tester",
		Action:      "wake",
	})
	if err != nil {
		t.Fatalf("failed to register await: %v", err)
	}

	// Wait briefly
	time.Sleep(10 * time.Millisecond)

	h := NewPeerAckTimeoutAuditHandler(root, nil)
	job := &ScheduledJob{}

	// Execute without enough time passing (should not expire)
	err = h.Execute(context.Background(), job)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	open, err := agentfeed.ListOpenPeerAckAwaits(root, "tester")
	if err != nil {
		t.Fatalf("list open failed: %v", err)
	}
	if len(open) != 1 {
		t.Fatalf("expected 1 open await, got %d", len(open))
	}
}

func TestPeerAckTimeoutAuditHandler_ExpiresWithShortMaxAge(t *testing.T) {
	root := t.TempDir()
	_, err := agentfeed.RegisterPeerAckAwait(root, agentfeed.PeerAckAwaitInput{
		EventID:     "AFE-expire",
		FromAgentID: "tester",
		Action:      "wake",
	})
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	time.Sleep(15 * time.Millisecond)

	h := NewPeerAckTimeoutAuditHandlerWithMaxAge(root, nil, time.Millisecond)
	if err := h.Execute(context.Background(), &ScheduledJob{}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	open, err := agentfeed.ListOpenPeerAckAwaits(root, "tester")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(open) != 0 {
		t.Fatalf("expected expiry, still open=%d", len(open))
	}
}
