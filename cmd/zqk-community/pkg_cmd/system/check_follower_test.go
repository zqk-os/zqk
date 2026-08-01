package system

import (
	"bytes"
	"testing"

	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/spf13/cobra"
)

// newTestCmd creates a minimal cobra.Command with stderr wired to a buffer.
func newTestCmd() (*cobra.Command, *bytes.Buffer) {
	cmd := &cobra.Command{}
	buf := &bytes.Buffer{}
	cmd.SetErr(buf)
	return cmd, buf
}

func TestTerminalProgressSubscriber_DiscoveryPhase(t *testing.T) {
	t.Parallel()
	opID := "check_123"
	cmd, buf := newTestCmd()

	sub := NewTerminalProgressSubscriber(opID, cmd)

	event := &coordination.OperationalEvent{
		Type:          "operation.progress",
		OperationID:   opID,
		OperationType: "system_check",
		Metadata: map[string]any{
			objects.FieldKeyPhase: "discovery",
			"files_found":         42,
			"elapsed_seconds":     float64(5),
			objects.FieldKeyKind:  "backlog_item",
		},
	}

	if err := sub.HandleEvent(event); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	out := buf.String()
	if out == emptyValue {
		t.Fatalf("expected discovery heartbeat output, got empty string")
	}

	if !bytes.Contains([]byte(out), []byte("Discovering backlog_item objects... 42 found")) {
		t.Errorf("unexpected discovery output: %q", out)
	}
	if !bytes.Contains([]byte(out), []byte("5s")) {
		t.Errorf("expected elapsed seconds in output, got: %q", out)
	}
}

// TestTerminalProgressSubscriber_DiscoveryHeartbeat_OverallZeroFiles verifies that
// when no files have been found yet but elapsed time is present, we still emit
// a heartbeat message so the user sees that discovery is in progress.
func TestTerminalProgressSubscriber_DiscoveryHeartbeat_OverallZeroFiles(t *testing.T) {
	t.Parallel()
	opID := "check_heartbeat_overall"
	cmd, buf := newTestCmd()

	sub := NewTerminalProgressSubscriber(opID, cmd)

	event := &coordination.OperationalEvent{
		Type:          "operation.progress",
		OperationID:   opID,
		OperationType: "system_check",
		Metadata: map[string]any{
			objects.FieldKeyPhase: "discovery",
			"files_found":         0,
			"elapsed_seconds":     float64(4),
			// no kind -> overall heartbeat
		},
	}

	if err := sub.HandleEvent(event); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	out := buf.String()
	if out == emptyValue {
		t.Fatalf("expected discovery heartbeat output for overall progress, got empty string")
	}

	if !bytes.Contains([]byte(out), []byte("Discovering objects...")) {
		t.Errorf("expected overall discovery heartbeat, got: %q", out)
	}
	if !bytes.Contains([]byte(out), []byte("4s")) {
		t.Errorf("expected elapsed seconds in overall heartbeat output, got: %q", out)
	}
}

// TestTerminalProgressSubscriber_DiscoverySkipUnknownCount verifies that
// discovery events with files_found < 0 (unknown) are ignored and do not
// spam the terminal with confusing "-1 found" messages.
func TestTerminalProgressSubscriber_DiscoverySkipUnknownCount(t *testing.T) {
	t.Parallel()
	opID := "check_skip_unknown"
	cmd, buf := newTestCmd()

	sub := NewTerminalProgressSubscriber(opID, cmd)

	event := &coordination.OperationalEvent{
		Type:          "operation.progress",
		OperationID:   opID,
		OperationType: "system_check",
		Metadata: map[string]any{
			objects.FieldKeyPhase: "discovery",
			"files_found":         -1,
			"elapsed_seconds":     float64(3),
			objects.FieldKeyKind:  "backlog_item",
		},
	}

	if err := sub.HandleEvent(event); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	out := buf.String()
	if out != emptyValue {
		t.Fatalf("expected no output for unknown file count, got: %q", out)
	}
}

func TestTerminalProgressSubscriber_DiscoveryStart(t *testing.T) {
	t.Parallel()
	opID := "check_789"
	cmd, buf := newTestCmd()

	sub := NewTerminalProgressSubscriber(opID, cmd)

	// Discovery start event (no files_found yet, just kind count)
	event := &coordination.OperationalEvent{
		Type:          "operation.start",
		OperationID:   opID,
		OperationType: "system_check",
		Metadata: map[string]any{
			objects.FieldKeyPhase: "discovery",
			"kinds_count":         15,
		},
	}

	if err := sub.HandleEvent(event); err != nil {
		t.Fatalf("HandleEvent returned error: %v", err)
	}

	out := buf.String()
	if out == emptyValue {
		t.Fatalf("expected discovery start heartbeat output, got empty string")
	}

	if !bytes.Contains([]byte(out), []byte("Starting discovery across 15 kinds")) {
		t.Errorf("unexpected discovery start output: %q", out)
	}
}

func TestTerminalProgressSubscriber_ValidationPhase(t *testing.T) {
	t.Parallel()
	opID := "check_456"
	cmd, buf := newTestCmd()

	sub := NewTerminalProgressSubscriber(opID, cmd)

	// First progress event (should render)
	event1 := &coordination.OperationalEvent{
		Type:          "operation.progress",
		OperationID:   opID,
		OperationType: "system_check",
		Metadata: map[string]any{
			"progress":    10,
			"total_tasks": 100,
			"queue_size":  5,
		},
	}

	if err := sub.HandleEvent(event1); err != nil {
		t.Fatalf("HandleEvent (event1) returned error: %v", err)
	}

	out1 := buf.String()
	if out1 == emptyValue {
		t.Fatalf("expected validation heartbeat output, got empty string")
	}
	if !bytes.Contains([]byte(out1), []byte("System check progress: 10.0% (10/100, queue: 5)")) {
		t.Errorf("unexpected validation output: %q", out1)
	}

	// Second progress event with no percentage change should be suppressed.
	buf.Reset()
	if err := sub.HandleEvent(event1); err != nil {
		t.Fatalf("HandleEvent (event1 again) returned error: %v", err)
	}
	out2 := buf.String()
	if out2 != emptyValue {
		t.Errorf("expected no additional output for unchanged progress, got: %q", out2)
	}
}
