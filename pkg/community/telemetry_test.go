package community

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestTelemetryCollector_FunctionalAcceptance verifies that the opt-in configuration toggle,
// diagnostic telemetry snapshot builder, anonymized event metrics collection,
// and file export mechanism work seamlessly (CRIT-1789714024741956000-c4eb76e6).
func TestTelemetryCollector_FunctionalAcceptance(t *testing.T) {
	collector := NewTelemetryCollector("test-community-cluster-123")

	// 1. Must be opted-out by default
	if collector.IsOptedIn() {
		t.Fatalf("expected collector to default to opt-out")
	}

	// 2. Toggle opt-in to enabled
	collector.SetOptIn(true)
	if !collector.IsOptedIn() {
		t.Fatalf("expected collector to be opted-in")
	}

	// 3. Record metric events
	err := collector.RecordEvent(MetricEvent{
		Category: "command_execution",
		Action:   "quickstart_run",
		Attributes: map[string]string{
			"outcome": "success",
			"mode":    "offline",
		},
		DurationMs: 120,
	})
	if err != nil {
		t.Fatalf("failed to record metric event: %v", err)
	}

	err = collector.RecordEvent(MetricEvent{
		Category: "workflow_transition",
		Action:   "promote_bli",
		Attributes: map[string]string{
			"outcome": "success",
		},
		DurationMs: 45,
	})
	if err != nil {
		t.Fatalf("failed to record second event: %v", err)
	}

	// 4. Capture diagnostic snapshot
	snapshot := collector.Snapshot(false)
	if snapshot.TotalEvents != 2 {
		t.Fatalf("expected 2 total events, got %d", snapshot.TotalEvents)
	}
	if snapshot.ClusterHash == "" {
		t.Fatalf("expected cluster hash to be generated")
	}
	if snapshot.Summary.Categories["command_execution"] != 1 {
		t.Errorf("expected 1 command_execution event, got %d", snapshot.Summary.Categories["command_execution"])
	}
	if snapshot.Summary.Categories["workflow_transition"] != 1 {
		t.Errorf("expected 1 workflow_transition event, got %d", snapshot.Summary.Categories["workflow_transition"])
	}

	// 5. Export to file
	tmpDir := t.TempDir()
	exportPath := filepath.Join(tmpDir, "diagnostics", "snapshot.json")
	if err := collector.ExportToFile(exportPath, snapshot); err != nil {
		t.Fatalf("failed to export snapshot to file: %v", err)
	}

	if !fileutil.Exists(exportPath) {
		t.Fatalf("exported diagnostic snapshot file does not exist at %s", exportPath)
	}

	content, err := fileutil.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed to read exported file: %v", err)
	}

	var parsed DiagnosticSnapshot
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("failed to unmarshal exported snapshot JSON: %v", err)
	}

	if parsed.SnapshotID != snapshot.SnapshotID {
		t.Errorf("expected snapshot ID %s, got %s", snapshot.SnapshotID, parsed.SnapshotID)
	}
	if parsed.TotalEvents != 2 {
		t.Errorf("expected 2 events in exported file, got %d", parsed.TotalEvents)
	}
}

// TestTelemetryCollector_BoundaryAndErrorHandling verifies negative testing, boundary cases,
// disabled opt-in zero recording, PII scrubbing on attributes, and malformed payload handling
// (CRIT-1789714024741957000-bf803c3b).
func TestTelemetryCollector_BoundaryAndErrorHandling(t *testing.T) {
	t.Run("opt_out_zero_transmission", func(t *testing.T) {
		collector := NewTelemetryCollector("test-cluster")
		collector.SetOptIn(false)

		// Recording when opted-out must succeed silently and discard
		err := collector.RecordEvent(MetricEvent{
			Category: "build",
			Action:   "compile",
		})
		if err != nil {
			t.Fatalf("expected nil error on opted-out record, got: %v", err)
		}

		snap := collector.Snapshot(false)
		if snap.TotalEvents != 0 {
			t.Fatalf("expected 0 events captured when opted-out, got %d", snap.TotalEvents)
		}
	})

	t.Run("missing_category_or_action", func(t *testing.T) {
		collector := NewTelemetryCollector("test-cluster")
		collector.SetOptIn(true)

		err := collector.RecordEvent(MetricEvent{
			Category: "",
			Action:   "compile",
		})
		if err == nil {
			t.Fatalf("expected error for empty category")
		}

		err = collector.RecordEvent(MetricEvent{
			Category: "build",
			Action:   "",
		})
		if err == nil {
			t.Fatalf("expected error for empty action")
		}
	})

	t.Run("pii_and_secrets_sanitization", func(t *testing.T) {
		collector := NewTelemetryCollector("test-cluster")
		collector.SetOptIn(true)

		fakeToken := "ghp_" + "1234567890abcdef1234567890abcdef1234"
		err := collector.RecordEvent(MetricEvent{
			Category: "network_request",
			Action:   "dispatch",
			Attributes: map[string]string{
				objects.FieldKeyEmail: "admin@company.internal",
				"ip":                  "10.0.0.42",
				"user_dir":            "/Users/engineer/work/repo",
				"auth_tok":            fakeToken,
				"safe_info":           "ok-value",
			},
		})
		if err != nil {
			t.Fatalf("failed to record event: %v", err)
		}

		snap := collector.Snapshot(true)
		if snap.TotalEvents != 1 {
			t.Fatalf("expected 1 event, got %d", snap.TotalEvents)
		}
		attrs := snap.Events[0].Attributes

		if strings.Contains(attrs[objects.FieldKeyEmail], "admin@company.internal") {
			t.Errorf("expected email to be redacted, got: %s", attrs[objects.FieldKeyEmail])
		}
		if !strings.Contains(attrs[objects.FieldKeyEmail], "[REDACTED_EMAIL]") {
			t.Errorf("expected [REDACTED_EMAIL] token, got: %s", attrs[objects.FieldKeyEmail])
		}

		if strings.Contains(attrs["ip"], "10.0.0.42") {
			t.Errorf("expected IP to be redacted, got: %s", attrs["ip"])
		}
		if !strings.Contains(attrs["ip"], "[REDACTED_IP]") {
			t.Errorf("expected [REDACTED_IP] token, got: %s", attrs["ip"])
		}

		if strings.Contains(attrs["user_dir"], "/Users/engineer") {
			t.Errorf("expected user dir to be redacted, got: %s", attrs["user_dir"])
		}
		if !strings.Contains(attrs["user_dir"], "[USER_HOME]") {
			t.Errorf("expected [USER_HOME] token, got: %s", attrs["user_dir"])
		}

		if strings.Contains(attrs["auth_tok"], fakeToken) {
			t.Errorf("expected secret token to be redacted, got: %s", attrs["auth_tok"])
		}
	})

	t.Run("empty_export_path_rejection", func(t *testing.T) {
		collector := NewTelemetryCollector("test-cluster")
		collector.SetOptIn(true)
		snap := collector.Snapshot(false)

		if err := collector.ExportToFile("", snap); err == nil {
			t.Fatalf("expected error when exporting to empty destination path")
		}
	})
}

// TestTelemetryCollector_IntegrationAndConformance verifies end-to-end integration,
// deterministic cluster hashing, drain snapshot semantics, and error rollup metrics
// (CRIT-1789714024741958000-19fa37f8).
func TestTelemetryCollector_IntegrationAndConformance(t *testing.T) {
	collector := NewTelemetryCollector("unique-cluster-node-99")
	collector.SetOptIn(true)

	// 1. Verify deterministic hashing
	hash1 := collector.AnonymizedClusterHash()
	hash2 := collector.AnonymizedClusterHash()
	if hash1 != hash2 || len(hash1) != 16 {
		t.Fatalf("expected deterministic 16-character hex hash, got %q vs %q", hash1, hash2)
	}

	// 2. Record success and error events
	for i := 0; i < 5; i++ {
		_ = collector.RecordEvent(MetricEvent{
			Category: "daemon_job",
			Action:   "tick",
			Attributes: map[string]string{
				"result": "success",
			},
		})
	}
	for i := 0; i < 2; i++ {
		_ = collector.RecordEvent(MetricEvent{
			Category: "daemon_job",
			Action:   "network_sync",
			Attributes: map[string]string{
				"result": "error",
				"error":  "connection refused",
			},
		})
	}

	// 3. Verify drain semantics
	snapshot1 := collector.Snapshot(true)
	if snapshot1.TotalEvents != 7 {
		t.Fatalf("expected 7 events, got %d", snapshot1.TotalEvents)
	}
	if snapshot1.Summary.TotalSuccess != 5 {
		t.Errorf("expected 5 success events, got %d", snapshot1.Summary.TotalSuccess)
	}
	if snapshot1.Summary.TotalErrors != 2 {
		t.Errorf("expected 2 error events, got %d", snapshot1.Summary.TotalErrors)
	}

	// Drained snapshot should now be empty
	snapshot2 := collector.Snapshot(false)
	if snapshot2.TotalEvents != 0 {
		t.Fatalf("expected buffer to be empty after drain, got %d events", snapshot2.TotalEvents)
	}
}
