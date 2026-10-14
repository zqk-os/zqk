package agentfeed

import (
	"context"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
)

func setupTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := datacell.AgentChatChannelConfig{
		Enabled:               true,
		DeliveryMode:          datacell.DeliveryModeNotify,
		FeedID:                "FEED-TEST",
		ContractSchemaVersion: "2.0.0",
	}
	if err := datacell.WriteAgentChatChannelConfig(dir, cfg); err != nil {
		t.Fatalf("failed to write chat channel config: %v", err)
	}
	return dir
}

func TestNonInterruptingSteer_AppendAndDeliver(t *testing.T) {
	root := setupTestProject(t)

	// Append non-interrupting steering event
	res, err := AppendEvent(AppendEventInput{
		ProjectRoot:     root,
		Message:         "Focus on boundary verification and race safety",
		AgentID:         "human-steer",
		ToAgentID:       "worker-01",
		Sender:          FeedSenderHumanSteer,
		EventType:       FeedEventTypeSteering,
		NonInterrupting: true,
		InFlightRePrime: true,
	})
	if err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}
	if !res.NonInterrupting {
		t.Fatalf("expected res.NonInterrupting = true, got %v", res.NonInterrupting)
	}
	if !res.InFlightRePrime {
		t.Fatalf("expected res.InFlightRePrime = true, got %v", res.InFlightRePrime)
	}
	if nonInt, ok := res.Event[JSONFieldNonInterrupting].(bool); !ok || !nonInt {
		t.Fatalf("expected event non_interrupting = true in event map: %v", res.Event)
	}
	if reprime, ok := res.Event[JSONFieldInFlightRePrime].(bool); !ok || !reprime {
		t.Fatalf("expected event in_flight_reprime = true in event map: %v", res.Event)
	}

	// Deliver via non-interrupting wake options
	wake := WakePeerOpts(context.Background(), WakePeerOptions{
		ProjectRoot:     root,
		Message:         "Focus on boundary verification and race safety",
		InReplyTo:       res.EventID,
		FromAgentID:     "human-steer",
		ToAgentID:       "worker-01",
		NonInterrupting: true,
		InFlightRePrime: true,
	})

	if !wake.Attempted {
		t.Fatal("expected wake.Attempted = true")
	}
	if wake.Transport != TransportNonInterrupting {
		t.Fatalf("expected transport = %q, got %q", TransportNonInterrupting, wake.Transport)
	}
	if !wake.Live {
		t.Fatal("expected wake.Live = true for non-interrupting live update")
	}
	if !wake.NonInterrupting {
		t.Fatal("expected wake.NonInterrupting = true")
	}
	if !wake.InFlightRePrime {
		t.Fatal("expected wake.InFlightRePrime = true")
	}
	if !wake.DeliveryReceipt {
		t.Fatal("expected wake.DeliveryReceipt = true")
	}
	if wake.DeliveryEventID == "" {
		t.Fatal("expected non-empty DeliveryEventID")
	}
}

func TestInFlightSteering_ListAndFormat(t *testing.T) {
	root := setupTestProject(t)

	_, err := AppendEvent(AppendEventInput{
		ProjectRoot:     root,
		Message:         "Guidance 1: check error wrapping",
		AgentID:         "lead-architect",
		ToAgentID:       "worker-02",
		Sender:          FeedSenderHumanSteer,
		EventType:       FeedEventTypeSteering,
		NonInterrupting: true,
		InFlightRePrime: true,
	})
	if err != nil {
		t.Fatalf("AppendEvent 1 failed: %v", err)
	}

	_, err = AppendEvent(AppendEventInput{
		ProjectRoot:     root,
		Message:         "Guidance 2: check zero PII security",
		AgentID:         "lead-architect",
		ToAgentID:       "worker-02",
		Sender:          FeedSenderHumanSteer,
		EventType:       FeedEventTypeSteering,
		NonInterrupting: true,
		InFlightRePrime: true,
	})
	if err != nil {
		t.Fatalf("AppendEvent 2 failed: %v", err)
	}

	items, err := ListInFlightSteering(root, "worker-02")
	if err != nil {
		t.Fatalf("ListInFlightSteering failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 in-flight steering items, got %d", len(items))
	}
	if !strings.Contains(items[0].Message, "check error wrapping") {
		t.Fatalf("unexpected message in item 0: %s", items[0].Message)
	}
	if !strings.Contains(items[1].Message, "check zero PII security") {
		t.Fatalf("unexpected message in item 1: %s", items[1].Message)
	}

	formatted := FormatInFlightGuidance(items)
	if !strings.Contains(formatted, "In-Flight Steering Guidance (Non-Interrupting Re-Priming)") {
		t.Fatalf("expected header in formatted guidance, got: %s", formatted)
	}
	if !strings.Contains(formatted, "check error wrapping") || !strings.Contains(formatted, "check zero PII security") {
		t.Fatalf("missing expected messages in formatted guidance: %s", formatted)
	}
}

func TestRePrimeAgentContext_Lifecycle(t *testing.T) {
	root := setupTestProject(t)

	ev, err := AppendEvent(AppendEventInput{
		ProjectRoot:     root,
		Message:         "Maintain AST integrity while modifying feed code",
		AgentID:         "coordinator",
		ToAgentID:       "worker-03",
		Sender:          FeedSenderHumanSteer,
		EventType:       FeedEventTypeSteering,
		NonInterrupting: true,
		InFlightRePrime: true,
	})
	if err != nil {
		t.Fatalf("AppendEvent failed: %v", err)
	}

	baseContext := "Task: Implement feature X. Instructions: write unit tests."

	// 1. Re-prime context without auto-ack
	reprimeResult, err := RePrimeAgentContext(RePrimeInput{
		ProjectRoot:    root,
		AgentID:        "worker-03",
		PersonaRef:     "PER-DEVELOPER",
		CurrentContext: baseContext,
		AutoAck:        false,
	})
	if err != nil {
		t.Fatalf("RePrimeAgentContext failed: %v", err)
	}
	if !reprimeResult.Applied {
		t.Fatal("expected reprimeResult.Applied = true")
	}
	if !strings.Contains(reprimeResult.RePrimedContext, baseContext) {
		t.Fatalf("expected base context preserved in: %s", reprimeResult.RePrimedContext)
	}
	if !strings.Contains(reprimeResult.RePrimedContext, "Maintain AST integrity while modifying feed code") {
		t.Fatalf("expected guidance incorporated in: %s", reprimeResult.RePrimedContext)
	}
	if len(reprimeResult.AckedEventIDs) != 0 {
		t.Fatalf("expected 0 acked event IDs when AutoAck=false, got %d", len(reprimeResult.AckedEventIDs))
	}

	// Verify persistent state file exists and status is aligned
	state, err := LoadAgentInFlightState(root, "worker-03")
	if err != nil {
		t.Fatalf("LoadAgentInFlightState failed: %v", err)
	}
	if state.Status != InFlightStatusAligned {
		t.Fatalf("expected state status = %q, got %q", InFlightStatusAligned, state.Status)
	}
	if len(state.ActiveGuidance) != 1 {
		t.Fatalf("expected 1 active guidance in persistent state, got %d", len(state.ActiveGuidance))
	}

	// 2. Re-prime context with auto-ack to consume and clear the pending hourglass
	ackResult, err := RePrimeAgentContext(RePrimeInput{
		ProjectRoot:    root,
		AgentID:        "worker-03",
		PersonaRef:     "PER-DEVELOPER",
		CurrentContext: reprimeResult.RePrimedContext,
		AutoAck:        true,
	})
	if err != nil {
		t.Fatalf("RePrimeAgentContext with AutoAck failed: %v", err)
	}
	if len(ackResult.AckedEventIDs) != 1 || ackResult.AckedEventIDs[0] != ev.EventID {
		t.Fatalf("expected event %s acked, got %v", ev.EventID, ackResult.AckedEventIDs)
	}

	// Verify inbox is now drained
	remaining, err := ListInFlightSteering(root, "worker-03")
	if err != nil {
		t.Fatalf("ListInFlightSteering failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected 0 remaining unacked in-flight items, got %d", len(remaining))
	}
}

func TestRePrimeAgentContext_BoundaryAndErrors(t *testing.T) {
	root := setupTestProject(t)

	// Missing project root
	if _, err := RePrimeAgentContext(RePrimeInput{AgentID: "w-1"}); err == nil {
		t.Fatal("expected error with empty project root")
	}

	// Missing agent ID
	if _, err := RePrimeAgentContext(RePrimeInput{ProjectRoot: root}); err == nil {
		t.Fatal("expected error with empty agent ID")
	}

	// No pending steering -> Applied = false, unchanged context
	origContext := "Baseline context unchanged"
	res, err := RePrimeAgentContext(RePrimeInput{
		ProjectRoot:    root,
		AgentID:        "idle-worker",
		CurrentContext: origContext,
	})
	if err != nil {
		t.Fatalf("unexpected error for idle worker: %v", err)
	}
	if res.Applied {
		t.Fatal("expected res.Applied = false when no steering is pending")
	}
	if res.RePrimedContext != origContext {
		t.Fatalf("expected unchanged context %q, got %q", origContext, res.RePrimedContext)
	}
	if len(res.Guidance) != 0 {
		t.Fatalf("expected 0 guidance items, got %d", len(res.Guidance))
	}
}

func TestAgentInFlightState_PersistenceAndRecord(t *testing.T) {
	root := setupTestProject(t)

	// Empty agent ID errors
	if _, err := LoadAgentInFlightState(root, ""); err == nil {
		t.Fatal("expected error on empty agent id")
	}
	if err := SaveAgentInFlightState(root, AgentInFlightState{}); err == nil {
		t.Fatal("expected error on empty agent id")
	}

	// Load non-existent state returns default aligned state
	initial, err := LoadAgentInFlightState(root, "worker-04")
	if err != nil {
		t.Fatalf("expected clean initial load, got: %v", err)
	}
	if initial.Status != InFlightStatusAligned {
		t.Fatalf("expected status = %q, got %q", InFlightStatusAligned, initial.Status)
	}

	// Record in-flight steering
	item := InFlightSteering{
		EventID:         "AFE-100",
		FromAgentID:     "coordinator",
		ToAgentID:       "worker-04",
		Message:         "Optimize memory allocations in hot loop",
		NonInterrupting: true,
		InFlightRePrime: true,
	}
	if err := RecordInFlightSteering(root, "worker-04", item); err != nil {
		t.Fatalf("RecordInFlightSteering failed: %v", err)
	}

	// Verify loaded state reflects recorded steering
	loaded, err := LoadAgentInFlightState(root, "worker-04")
	if err != nil {
		t.Fatalf("LoadAgentInFlightState failed: %v", err)
	}
	if loaded.Status != InFlightStatusPending {
		t.Fatalf("expected status = %q, got %q", InFlightStatusPending, loaded.Status)
	}
	if len(loaded.ActiveGuidance) != 1 {
		t.Fatalf("expected 1 active guidance, got %d", len(loaded.ActiveGuidance))
	}
	if loaded.ActiveGuidance[0].EventID != "AFE-100" {
		t.Fatalf("expected event id AFE-100, got %s", loaded.ActiveGuidance[0].EventID)
	}

	// Recording duplicate event ID does not duplicate in ActiveGuidance
	if err := RecordInFlightSteering(root, "worker-04", item); err != nil {
		t.Fatalf("duplicate RecordInFlightSteering failed: %v", err)
	}
	reloaded, err := LoadAgentInFlightState(root, "worker-04")
	if err != nil {
		t.Fatalf("LoadAgentInFlightState failed: %v", err)
	}
	if len(reloaded.ActiveGuidance) != 1 {
		t.Fatalf("expected deduplication to keep 1 item, got %d", len(reloaded.ActiveGuidance))
	}
}
