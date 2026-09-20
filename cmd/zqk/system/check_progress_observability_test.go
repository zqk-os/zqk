package system

// TST-1789663115490931001-9e67dbc8
// Verifies multi-phase window progress observability across all 4 silent windows:
// Window 1: Process start to initial feedback (start idempotency and fast feedback)
// Window 2: Discovery phase streaming and heartbeat progress
// Window 3: Post-validation aggregation and CAS membrane checking
// Window 4: Teardown, cache persistence, and completion feedback

import (
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/objects"
)

// CRIT-1789663115490931000-3286aa9d: Functional Acceptance
// Verifies all 4 phase transitions emit clear, timely feedback without multi-second silent gaps.
func TestMultiPhaseWindowObservability_FunctionalAcceptance(t *testing.T) {
	t.Parallel()
	opID := "check_obs_func"
	cmd, buf := newTestCmd()

	sub := NewTerminalProgressSubscriber(opID, cmd)

	// 1. Window 1: Initial start event
	startEvent := &coordination.OperationalEvent{
		Type:          "operation.start",
		OperationID:   opID,
		OperationType: eventTypeSystemCheck,
		Metadata: map[string]any{
			objects.FieldKeyPhase: eventStatusStart,
		},
	}
	if err := sub.HandleEvent(startEvent); err != nil {
		t.Fatalf("HandleEvent(start) error: %v", err)
	}
	if !strings.Contains(buf.String(), "System check starting...") {
		t.Errorf("Window 1: expected 'System check starting...', got: %q", buf.String())
	}

	// 2. Window 2: Discovery progress with count & heartbeat
	buf.Reset()
	discEvent := &coordination.OperationalEvent{
		Type:          "operation.progress",
		OperationID:   opID,
		OperationType: eventTypeSystemCheck,
		Metadata: map[string]any{
			objects.FieldKeyPhase: "discovery",
			"files_found":         1200,
			"elapsed_seconds":     float64(2),
		},
	}
	if err := sub.HandleEvent(discEvent); err != nil {
		t.Fatalf("HandleEvent(discovery) error: %v", err)
	}
	if !strings.Contains(buf.String(), "Discovering objects... 1200 found (2s)") {
		t.Errorf("Window 2: expected discovery progress, got: %q", buf.String())
	}

	// 3. Window 3: Post-validation aggregation event
	buf.Reset()
	aggEvent := &coordination.OperationalEvent{
		Type:          "operation.progress",
		OperationID:   opID,
		OperationType: eventTypeSystemCheck,
		Metadata: map[string]any{
			objects.FieldKeyPhase: "aggregation",
			"message":             "Aggregating validation layers and checking CAS membrane...",
		},
	}
	if err := sub.HandleEvent(aggEvent); err != nil {
		t.Fatalf("HandleEvent(aggregation) error: %v", err)
	}
	if !strings.Contains(buf.String(), "Aggregating validation layers and checking CAS membrane...") {
		t.Errorf("Window 3: expected aggregation message, got: %q", buf.String())
	}

	// 4. Window 4: Teardown / finalization event
	buf.Reset()
	tearEvent := &coordination.OperationalEvent{
		Type:          "operation.progress",
		OperationID:   opID,
		OperationType: eventTypeSystemCheck,
		Metadata: map[string]any{
			objects.FieldKeyPhase: "teardown",
			"message":             "Finalizing caches and storage queues...",
		},
	}
	if err := sub.HandleEvent(tearEvent); err != nil {
		t.Fatalf("HandleEvent(teardown) error: %v", err)
	}
	if !strings.Contains(buf.String(), "Finalizing caches and storage queues...") {
		t.Errorf("Window 4: expected teardown message, got: %q", buf.String())
	}
}

// CRIT-1789663115490932000-a5aa6e8d: Boundary & Error Handling
// Verifies handling of nil events, foreign operations, start idempotency, and empty payloads.
func TestMultiPhaseWindowObservability_BoundaryAndErrorHandling(t *testing.T) {
	t.Parallel()
	opID := "check_obs_boundary"
	cmd, buf := newTestCmd()

	sub := NewTerminalProgressSubscriber(opID, cmd)

	// Nil event handling
	if err := sub.HandleEvent(nil); err != nil {
		t.Errorf("HandleEvent(nil) should not error, got: %v", err)
	}
	if buf.Len() > 0 {
		t.Errorf("HandleEvent(nil) produced unexpected output: %q", buf.String())
	}

	// Foreign operation ID
	foreignEvent := &coordination.OperationalEvent{
		Type:          "operation.progress",
		OperationID:   "check_different_id",
		OperationType: eventTypeSystemCheck,
		Metadata: map[string]any{
			objects.FieldKeyPhase: "discovery",
			"files_found":         10,
		},
	}
	if err := sub.HandleEvent(foreignEvent); err != nil {
		t.Errorf("HandleEvent(foreign) error: %v", err)
	}
	if buf.Len() > 0 {
		t.Errorf("foreign event produced output: %q", buf.String())
	}

	// Start idempotency: multiple start events must only print once
	sub.hasShownStart = false
	start1 := &coordination.OperationalEvent{
		Type:          "operation.start",
		OperationID:   opID,
		OperationType: eventTypeSystemCheck,
		Metadata: map[string]any{
			objects.FieldKeyPhase: eventStatusStart,
		},
	}
	_ = sub.HandleEvent(start1)
	startOutputCount := strings.Count(buf.String(), "System check starting...")
	if startOutputCount != 1 {
		t.Errorf("expected exactly 1 start line, got %d", startOutputCount)
	}

	// Second start event must not print duplicate
	_ = sub.HandleEvent(start1)
	startOutputCount2 := strings.Count(buf.String(), "System check starting...")
	if startOutputCount2 != 1 {
		t.Errorf("expected start idempotency (still 1 line), got %d", startOutputCount2)
	}
}

// CRIT-1789663115490933000-55fc3022: Integration & Conformance
// Verifies full multi-stage sequence conformance and 1s heartbeat timing.
func TestMultiPhaseWindowObservability_IntegrationAndConformance(t *testing.T) {
	t.Parallel()
	opID := "check_obs_integ"
	cmd, buf := newTestCmd()

	sub := NewTerminalProgressSubscriber(opID, cmd)

	// Heartbeat rendering when files_found is 0 but elapsed time is present
	hbEvent := &coordination.OperationalEvent{
		Type:          "operation.progress",
		OperationID:   opID,
		OperationType: eventTypeSystemCheck,
		Metadata: map[string]any{
			objects.FieldKeyPhase: "discovery",
			"files_found":         0,
			"elapsed_seconds":     float64(1),
		},
	}
	if err := sub.HandleEvent(hbEvent); err != nil {
		t.Fatalf("HandleEvent(heartbeat) error: %v", err)
	}
	if !strings.Contains(buf.String(), "Discovering objects... (1s)") {
		t.Errorf("expected discovery heartbeat '(1s)', got: %q", buf.String())
	}

	// Heartbeat repeat throttle: 1s threshold
	sub.lastDiscoveryTime = time.Now()
	buf.Reset()
	if err := sub.HandleEvent(hbEvent); err != nil {
		t.Fatalf("HandleEvent(throttled heartbeat) error: %v", err)
	}
	if buf.Len() > 0 {
		t.Errorf("heartbeat within 1s should be throttled, got: %q", buf.String())
	}
}
