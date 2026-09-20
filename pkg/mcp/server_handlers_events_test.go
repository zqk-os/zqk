package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestHandleNotificationEvent(t *testing.T) {
	s := NewServer()
	s.eventEmitter = NewEventEmitter(10)

	sub := newMockEventSubscriber("ide-seat-123", []EventType{EventTypeActionRequired})
	s.eventEmitter.Subscribe(sub)

	payload := map[string]any{
		objects.FieldKeyType: "action.required",
		"message":            "test message",
		"timestamp":          time.Now().Format(time.RFC3339),
		"fields":             map[string]any{objects.FieldKeyAgentID: "peer-agent-01"},
	}
	b, _ := json.Marshal(payload)

	res, err := s.handleNotificationEvent(context.Background(), "notifications/event", b)
	if err != nil {
		notificationSentinel := &NotificationSentinel{}
		if !errors.As(err, &notificationSentinel) {
			t.Fatalf("expected NotificationSentinel, got %v", err)
		}
	} else {
		t.Fatalf("expected error to be NotificationSentinel, got nil")
	}
	if res != nil {
		t.Fatalf("expected nil result")
	}

	events := sub.GetEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event in subscriber, got %d", len(events))
	}

	if events[0].Type != EventTypeActionRequired {
		t.Errorf("expected EventTypeActionRequired, got %v", events[0].Type)
	}
	if events[0].Message != "test message" {
		t.Errorf("expected 'test message', got %v", events[0].Message)
	}
}
