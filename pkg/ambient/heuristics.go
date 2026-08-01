package ambient

import (
	"context"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

// CoachHeuristics observes the ambient event hub for coachable moments.
type CoachHeuristics struct {
	hub EventHub
}

// NewCoachHeuristics initializes a new CoachHeuristics and subscribes it to the hub.
func NewCoachHeuristics(hub EventHub) *CoachHeuristics {
	h := &CoachHeuristics{hub: hub}
	hub.Subscribe(EventTypeFilesystem, h.handleFilesystem)
	hub.Subscribe(EventTypeSession, h.handleSession)
	return h
}

func (h *CoachHeuristics) handleFilesystem(ctx context.Context, event Event) error {
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return nil
	}

	target, _ := payload[objects.FieldKeyTargetID].(string)
	op, _ := payload[objects.FieldKeyOperation].(string)
	source, _ := payload[objects.FieldKeySource].(string)

	// Heuristic: Catch manual edits to process YAMLs
	if source == "fswatcher" && strings.Contains(op, "WRITE") && strings.Contains(target, "docs/architecture/") && strings.HasSuffix(target, ".yaml") {
		if err := h.hub.Publish(ctx, Event{
			Type: EventTypeSession,
			Payload: map[string]any{
				objects.FieldKeySource:      "coach_heuristics",
				"heuristic_type":            "manual_edit",
				objects.FieldKeyDescription: "Manual edit detected on " + target,
			},
			Timestamp: time.Now(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (h *CoachHeuristics) handleSession(ctx context.Context, event Event) error {
	payload, ok := event.Payload.(map[string]any)
	if !ok {
		return nil
	}

	source, _ := payload[objects.FieldKeySource].(string)
	// Don't intercept our own telemetry
	if source == "coach_heuristics" {
		return nil
	}

	cmd, _ := payload[objects.FieldKeyCommand].(string)

	// Heuristic: Catch grep pipes
	if strings.Contains(cmd, "|") && strings.Contains(cmd, "grep") {
		if err := h.hub.Publish(ctx, Event{
			Type: EventTypeSession,
			Payload: map[string]any{
				objects.FieldKeySource:      "coach_heuristics",
				"heuristic_type":            "grep_pipe",
				objects.FieldKeyDescription: "Grep pipe detected: " + cmd,
			},
			Timestamp: time.Now(),
		}); err != nil {
			return err
		}
	}
	return nil
}
