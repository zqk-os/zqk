package ambient

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/paths"
)

// CoachHeuristics observes the ambient event hub for coachable moments.
type CoachHeuristics struct {
	hub         EventHub
	projectRoot string
}

// NewCoachHeuristics initializes a new CoachHeuristics and subscribes it to the hub.
func NewCoachHeuristics(hub EventHub) *CoachHeuristics {
	return NewCoachHeuristicsWithRoot(hub, "")
}

// NewCoachHeuristicsWithRoot initializes CoachHeuristics with a project root for tips persistence.
func NewCoachHeuristicsWithRoot(hub EventHub, projectRoot string) *CoachHeuristics {
	h := &CoachHeuristics{hub: hub, projectRoot: projectRoot}
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
	if source == "fswatcher" && strings.Contains(op, "WRITE") && strings.Contains(target, paths.ProcessDir+"/") && strings.HasSuffix(target, ".yaml") {
		if h.projectRoot != "" {
			h.appendTip(paths.RewriteCanonicalCLIInvocations(fmt.Sprintf("ZQK Ambient Warning: Direct edit on %s detected. Once past the CAS membrane, edits must go through the CLI (zqk object update|promote).", target)))
		}
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

	// Heuristic: Catch grep pipes or lookup commands to hint zqk grep
	if (strings.Contains(cmd, "|") && strings.Contains(cmd, "grep")) || strings.HasPrefix(strings.TrimSpace(cmd), "grep ") {
		if h.projectRoot != "" {
			h.appendTip(paths.RewriteCanonicalCLIInvocations("ZQK Observer Tip: Utilize 'zqk grep' for faster, indexed, and kernel/AST-aware code and object lookups instead of shell grep."))
		}
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

func (h *CoachHeuristics) appendTip(tip string) {
	if h.projectRoot == "" || tip == "" {
		return
	}
	existing := observer.ReadCachedTips(h.projectRoot)
	for _, t := range existing {
		if t == tip {
			return // already present
		}
	}
	// Keep at most 3 tips
	tips := append([]string{tip}, existing...)
	if len(tips) > 3 {
		tips = tips[:3]
	}
	_ = observer.WriteCachedTips(h.projectRoot, tips)
}
