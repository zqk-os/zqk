package telemetry_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/telemetry"
)

// TestTelemetryCollector_FunctionalAcceptance satisfies CRIT-1789714024741956000-c4eb76e6.
// Verifies opt-in telemetry recording, anonymization of metadata attributes, and snapshot metrics collection.
func TestTelemetryCollector_FunctionalAcceptance(t *testing.T) {
	telemetry.SetOptIn(true)
	defer telemetry.SetOptIn(false)

	collector := telemetry.NewDiagnosticMetricsCollector()
	defer collector.Close()

	fakeToken := "ghp_" + "11223344556677889900aabbccddeeff0011"
	rawAttrs := map[string]any{
		"developer": "dev@internal.corp",
		"work_dir":  "/Users/lanceettl/workspace/zqk",
		"cli_cmd":   "zqk workflow whats-next",
		"token":     fakeToken,
	}

	ev, err := collector.RecordEvent("cli.execution", rawAttrs)
	if err != nil {
		t.Fatalf("expected successful event record, got error: %v", err)
	}

	if ev.EventName != "cli.execution" {
		t.Errorf("expected event name 'cli.execution', got %q", ev.EventName)
	}
	if ev.OS == "" || ev.Arch == "" {
		t.Errorf("expected non-empty OS and Arch in telemetry event")
	}

	// Verify anonymization applied on recorded attributes
	devAttr := ev.Metrics["developer"].(string)
	if strings.Contains(devAttr, "dev@internal.corp") {
		t.Errorf("email should be redacted from recorded telemetry, got: %s", devAttr)
	}
	if !strings.Contains(devAttr, "[REDACTED_EMAIL]") {
		t.Errorf("expected [REDACTED_EMAIL] token, got: %s", devAttr)
	}

	dirAttr := ev.Metrics["work_dir"].(string)
	if strings.Contains(dirAttr, "/Users/lanceettl") {
		t.Errorf("user directory should be redacted, got: %s", dirAttr)
	}

	tokenAttr := ev.Metrics["token"].(string)
	if strings.Contains(tokenAttr, "112233445566") {
		t.Errorf("github token should be redacted, got: %s", tokenAttr)
	}

	// Diagnostic Snapshot
	snap, err := collector.CaptureDiagnosticSnapshot()
	if err != nil {
		t.Fatalf("failed to capture diagnostic snapshot: %v", err)
	}
	if snap.NumCPU <= 0 || snap.NumGoroutine <= 0 {
		t.Errorf("expected positive NumCPU and NumGoroutine, got cpu=%d, goroutines=%d", snap.NumCPU, snap.NumGoroutine)
	}
}

// TestTelemetryCollector_BoundaryAndErrorHandling satisfies CRIT-1789714024741957000-bf803c3b.
// Verifies strict opt-out gate fail-closed behavior, empty event rejection, collector buffer limits, and close state.
func TestTelemetryCollector_BoundaryAndErrorHandling(t *testing.T) {
	t.Run("opt_out_fails_closed", func(t *testing.T) {
		telemetry.SetOptIn(false)
		collector := telemetry.NewDiagnosticMetricsCollector()
		defer collector.Close()

		_, err := collector.RecordEvent("ping", nil)
		if err != telemetry.ErrTelemetryOptedOut {
			t.Errorf("expected ErrTelemetryOptedOut when opt-in is false, got: %v", err)
		}

		_, err = collector.CaptureDiagnosticSnapshot()
		if err != telemetry.ErrTelemetryOptedOut {
			t.Errorf("expected ErrTelemetryOptedOut on snapshot when opt-in is false, got: %v", err)
		}
	})

	t.Run("empty_event_name_rejected", func(t *testing.T) {
		telemetry.SetOptIn(true)
		defer telemetry.SetOptIn(false)

		collector := telemetry.NewDiagnosticMetricsCollector()
		defer collector.Close()

		_, err := collector.RecordEvent("   ", nil)
		if err != telemetry.ErrEmptyTelemetryEvent {
			t.Errorf("expected ErrEmptyTelemetryEvent for whitespace name, got: %v", err)
		}
	})

	t.Run("buffer_cap_enforced", func(t *testing.T) {
		telemetry.SetOptIn(true)
		defer telemetry.SetOptIn(false)

		collector := telemetry.NewDiagnosticMetricsCollector(telemetry.WithMaxBuffered(3))
		defer collector.Close()

		for i := 1; i <= 5; i++ {
			_, _ = collector.RecordEvent("event.step", map[string]any{"step": i})
		}

		events := collector.Flush()
		if len(events) != 3 {
			t.Fatalf("expected buffer capped at 3, got %d", len(events))
		}
		if events[0].Metrics["step"] != 3 || events[2].Metrics["step"] != 5 {
			t.Errorf("expected oldest events pruned in buffer, got: %+v", events)
		}
	})

	t.Run("stopped_collector_rejects", func(t *testing.T) {
		telemetry.SetOptIn(true)
		defer telemetry.SetOptIn(false)

		collector := telemetry.NewDiagnosticMetricsCollector()
		collector.Close()

		_, err := collector.RecordEvent("test.after.close", nil)
		if err != telemetry.ErrCollectorStopped {
			t.Errorf("expected ErrCollectorStopped after close, got: %v", err)
		}
	})
}

// TestTelemetryCollector_IntegrationAndConformance satisfies CRIT-1789714024741958000-19fa37f8.
// Verifies file export formatting, asynchronous export pipeline, and flush/drain lifecycle.
func TestTelemetryCollector_IntegrationAndConformance(t *testing.T) {
	telemetry.SetOptIn(true)
	defer telemetry.SetOptIn(false)

	collector := telemetry.NewDiagnosticMetricsCollector()
	defer collector.Close()

	_, _ = collector.RecordEvent("session.start", map[string]any{"session_phase": "initialized"})
	_, _ = collector.RecordEvent("session.finish", map[string]any{"session_phase": "completed"})

	tmpDir := t.TempDir()
	exportPath := filepath.Join(tmpDir, "telemetry.json")

	flushed := collector.Flush()
	if len(flushed) != 2 {
		t.Fatalf("expected 2 flushed events, got %d", len(flushed))
	}

	if err := telemetry.ExportToFile(exportPath, flushed); err != nil {
		t.Fatalf("failed to export telemetry to file: %v", err)
	}

	fileBytes, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("failed reading exported file: %v", err)
	}
	content := string(fileBytes)
	if !strings.Contains(content, "session.start") || !strings.Contains(content, "session.finish") {
		t.Errorf("exported JSON missing expected event names: %s", content)
	}

	// Async export test
	var wg sync.WaitGroup
	wg.Add(1)
	_, _ = collector.RecordEvent("async.event", map[string]any{"async": true})

	collector.AsyncExport(context.Background(), func(events []telemetry.MetricEvent, err error) {
		defer wg.Done()
		if err != nil {
			t.Errorf("unexpected async export error: %v", err)
		}
		if len(events) != 1 {
			t.Errorf("expected 1 async event, got %d", len(events))
		}
	})

	wg.Wait()
}
