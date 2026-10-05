package ambient

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
)

type mockEventHub struct {
	handlers map[EventType][]EventHandler
	events   []Event
}

func newMockEventHub() *mockEventHub {
	return &mockEventHub{
		handlers: make(map[EventType][]EventHandler),
	}
}

func (h *mockEventHub) Subscribe(eventType EventType, handler EventHandler) {
	h.handlers[eventType] = append(h.handlers[eventType], handler)
}

func (h *mockEventHub) Publish(ctx context.Context, event Event) error {
	h.events = append(h.events, event)
	for _, handler := range h.handlers[event.Type] {
		if err := handler(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (h *mockEventHub) Status() string {
	return "mock"
}

func (h *mockEventHub) EnableEventSourcing(projectRoot string, secCtx *pkgctx.SecurityContext) {
}

func TestCoachHeuristics_ManualEdit(t *testing.T) {
	hub := newMockEventHub()
	NewCoachHeuristics(hub)

	err := hub.Publish(context.Background(), Event{
		Type: EventTypeFilesystem,
		Payload: map[string]any{
			objects.FieldKeySource:    "fswatcher",
			objects.FieldKeyTargetID:  filepath.Join(paths.ProcessGoalsDir, "my_goal.yaml"),
			objects.FieldKeyOperation: "WRITE",
		},
		Timestamp: time.Now(),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range hub.events {
		if e.Type == EventTypeSession {
			payload := e.Payload.(map[string]any)
			if payload[objects.FieldKeySource] == "coach_heuristics" && payload["heuristic_type"] == "manual_edit" {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("Expected manual_edit telemetry event to be published")
	}
}

func TestCoachHeuristics_GrepPipe(t *testing.T) {
	hub := newMockEventHub()
	NewCoachHeuristics(hub)

	err := hub.Publish(context.Background(), Event{
		Type: EventTypeSession,
		Payload: map[string]any{
			objects.FieldKeySource:  "terminal",
			objects.FieldKeyCommand: "cat file.txt | grep something",
		},
		Timestamp: time.Now(),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range hub.events {
		if e.Type == EventTypeSession {
			payload := e.Payload.(map[string]any)
			if payload[objects.FieldKeySource] == "coach_heuristics" && payload["heuristic_type"] == "grep_pipe" {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("Expected grep_pipe telemetry event to be published")
	}
}

func TestCoachHeuristics_CommandTimeout(t *testing.T) {
	hub := newMockEventHub()
	NewCoachHeuristics(hub)

	err := hub.Publish(context.Background(), Event{
		Type: EventTypeSession,
		Payload: map[string]any{
			objects.FieldKeySource:  "terminal",
			objects.FieldKeyCommand: "object export --all",
			"timed_out":             true,
			"exit_code":             124,
			"error":                 "command timed out after 30s",
		},
		Timestamp: time.Now(),
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	found := false
	for _, e := range hub.events {
		if e.Type == EventTypeSession {
			payload, ok := e.Payload.(map[string]any)
			if ok && payload[objects.FieldKeySource] == "coach_heuristics" && payload["heuristic_type"] == "command_timeout" {
				found = true
				break
			}
		}
	}

	if !found {
		t.Errorf("Expected command_timeout telemetry event to be published")
	}
}
