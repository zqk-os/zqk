package ambient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Traceability Verification:
// Requirement: REQ-1793619495851164000-2ffc306d (Ambient Holon Mesh Telemetry & Unified Runtime CLI Dashboard)
// Criteria: CRIT-1793619520727910000-07ae32d3 (Static Floor: Ambient telemetry schema, holon metrics model, and nil-safety)

// TestTelemetry_NilSafety verifies that calling any method on a nil *AmbientTelemetryReport
// or nil ComponentHealth executes safely without panicking.
func TestTelemetry_NilSafety(t *testing.T) {
	var r *AmbientTelemetryReport

	// 1. IsHealthy on nil receiver
	if r.IsHealthy() {
		t.Errorf("expected r.IsHealthy() to be false on nil receiver")
	}

	// 2. ToJSON on nil receiver
	data, err := r.ToJSON()
	if err != nil {
		t.Fatalf("unexpected error on nil ToJSON: %v", err)
	}
	if string(data) != "null" {
		t.Errorf("expected ToJSON() on nil to return 'null', got %q", string(data))
	}

	// 3. SummaryTable on nil receiver
	table := r.SummaryTable()
	if table == "" || !strings.Contains(table, "<nil report>") {
		t.Errorf("expected SummaryTable() on nil to contain placeholder, got %q", table)
	}

	// 4. Getters on nil receiver
	if status := r.GetStatus(); status != TelemetryStatusStopped {
		t.Errorf("expected GetStatus() on nil to return %q, got %q", TelemetryStatusStopped, status)
	}
	if count := r.GetActiveHolonsCount(); count != 0 {
		t.Errorf("expected GetActiveHolonsCount() on nil to be 0, got %d", count)
	}
	if cb := r.GetCircuitBreakerStatus(); cb != "" {
		t.Errorf("expected GetCircuitBreakerStatus() on nil to be empty, got %q", cb)
	}
	if wm := r.GetRunwayWatermark(); wm != 0 {
		t.Errorf("expected GetRunwayWatermark() on nil to be 0, got %d", wm)
	}
	mesh := r.GetMeshMetrics()
	if mesh != (HolonMeshMetrics{}) {
		t.Errorf("expected GetMeshMetrics() on nil to be zero struct, got %+v", mesh)
	}
	comp, ok := r.GetComponent("unknown")
	if ok || comp.Name != "" {
		t.Errorf("expected GetComponent on nil to return false and zero struct")
	}

	// 5. Mutators on nil receiver should not panic
	r.SetStatus(TelemetryStatusHealthy)
	r.SetComponent(ComponentHealth{Name: "test"})

	// 6. Clone on nil receiver
	if clone := r.Clone(); clone != nil {
		t.Errorf("expected Clone() on nil to return nil, got %+v", clone)
	}

	// 7. ComponentHealth receiver nil safety
	var ch ComponentHealth
	if ch.IsHealthy() {
		t.Errorf("expected uninitialized ComponentHealth.IsHealthy() to be false")
	}
}

// TestTelemetry_SerializationRoundtrip tests full JSON serialization and deserialization,
// ensuring field integrity and support for both numeric and string uptime representations.
func TestTelemetry_SerializationRoundtrip(t *testing.T) {
	now := time.Date(2026, 11, 2, 11, 30, 0, 0, time.UTC)
	report := NewAmbientTelemetryReport(
		WithTimestamp(now),
		WithStatus(TelemetryStatusHealthy),
		WithUptime(12500*time.Millisecond),
		WithActiveHolonsCount(7),
		WithMeshMetrics(HolonMeshMetrics{
			IngestRate:       142.5,
			ConvergenceScore: 99.8,
			DivergenceCount:  0,
			EventThroughput:  310.2,
			StreamLatencyMs:  1.45,
		}),
		WithComponent(ComponentHealth{
			Name:          "stream_hydrator",
			Status:        TelemetryStatusHealthy,
			Message:       "buffer steady",
			LastHeartbeat: now.Add(-100 * time.Millisecond),
		}),
		WithComponent(ComponentHealth{
			Name:          "river_convergence",
			Status:        TelemetryStatusHealthy,
			Message:       "zero delta",
			LastHeartbeat: now.Add(-50 * time.Millisecond),
		}),
		WithCircuitBreakerStatus(CircuitBreakerStatusClosed),
		WithRunwayWatermark(4200),
	)

	// Direct ToJSON
	data, err := report.ToJSON()
	if err != nil {
		t.Fatalf("failed to serialize report to JSON: %v", err)
	}

	// Verify standard json.Unmarshal also works
	var direct AmbientTelemetryReport
	if err := json.Unmarshal(data, &direct); err != nil {
		t.Fatalf("failed to unmarshal with standard json: %v", err)
	}
	if direct.Status != report.Status {
		t.Errorf("direct unmarshal status mismatch: got %q, want %q", direct.Status, report.Status)
	}

	// FromJSON roundtrip
	restored, err := FromJSON(data)
	if err != nil {
		t.Fatalf("failed to decode report from JSON: %v", err)
	}

	if !restored.Timestamp.Equal(report.Timestamp) {
		t.Errorf("timestamp mismatch: got %v, want %v", restored.Timestamp, report.Timestamp)
	}
	if restored.Status != report.Status {
		t.Errorf("status mismatch: got %q, want %q", restored.Status, report.Status)
	}
	if restored.Uptime != report.Uptime {
		t.Errorf("uptime mismatch: got %v, want %v", restored.Uptime, report.Uptime)
	}
	if restored.ActiveHolonsCount != report.ActiveHolonsCount {
		t.Errorf("active holons mismatch: got %d, want %d", restored.ActiveHolonsCount, report.ActiveHolonsCount)
	}
	if restored.MeshMetrics != report.MeshMetrics {
		t.Errorf("mesh metrics mismatch: got %+v, want %+v", restored.MeshMetrics, report.MeshMetrics)
	}
	if restored.CircuitBreakerStatus != report.CircuitBreakerStatus {
		t.Errorf("circuit breaker mismatch: got %q, want %q", restored.CircuitBreakerStatus, report.CircuitBreakerStatus)
	}
	if restored.RunwayWatermark != report.RunwayWatermark {
		t.Errorf("runway watermark mismatch: got %d, want %d", restored.RunwayWatermark, report.RunwayWatermark)
	}
	if len(restored.Components) != 2 {
		t.Fatalf("expected 2 components, got %d", len(restored.Components))
	}
	if c, ok := restored.GetComponent("stream_hydrator"); !ok || c.Status != TelemetryStatusHealthy {
		t.Errorf("stream_hydrator component missing or degraded in restored report: %+v", c)
	}
}

// TestTelemetry_StringDurationAndEdgeCases tests string uptime decoding and FromJSON edges.
func TestTelemetry_StringDurationAndEdgeCases(t *testing.T) {
	// Test string duration deserialization
	jsonWithStringDuration := []byte(`{
		"status": "healthy",
		"uptime": "15s",
		"active_holons_count": 3,
		"circuit_breaker_status": "closed"
	}`)
	fromString, err := FromJSON(jsonWithStringDuration)
	if err != nil {
		t.Fatalf("failed to unmarshal JSON with string duration: %v", err)
	}
	if fromString.Uptime != 15*time.Second {
		t.Errorf("expected uptime 15s, got %v", fromString.Uptime)
	}

	// Test FromJSON edge cases
	nullReport, err := FromJSON([]byte("null"))
	if err != nil || nullReport != nil {
		t.Errorf("expected null FromJSON to yield nil report and nil err, got rep=%v err=%v", nullReport, err)
	}
	emptyReport, err := FromJSON([]byte(""))
	if err != nil || emptyReport != nil {
		t.Errorf("expected empty FromJSON to yield nil report and nil err, got rep=%v err=%v", emptyReport, err)
	}
}

// TestTelemetry_StatusAssertions exercises healthy, degraded, stopped, and edge states.
func TestTelemetry_StatusAssertions(t *testing.T) {
	t.Run("fully healthy report", func(t *testing.T) {
		r := NewAmbientTelemetryReport(
			WithStatus(TelemetryStatusHealthy),
			WithCircuitBreakerStatus(CircuitBreakerStatusClosed),
			WithComponent(ComponentHealth{Name: "c1", Status: TelemetryStatusHealthy}),
		)
		if !r.IsHealthy() {
			t.Errorf("expected fully healthy report to return true from IsHealthy()")
		}
	})

	t.Run("degraded report status", func(t *testing.T) {
		r := NewAmbientTelemetryReport(
			WithStatus(TelemetryStatusDegraded),
			WithCircuitBreakerStatus(CircuitBreakerStatusClosed),
		)
		if r.IsHealthy() {
			t.Errorf("expected degraded report to return false from IsHealthy()")
		}
	})

	t.Run("stopped report status", func(t *testing.T) {
		r := NewAmbientTelemetryReport(
			WithStatus(TelemetryStatusStopped),
		)
		if r.IsHealthy() {
			t.Errorf("expected stopped report to return false from IsHealthy()")
		}
	})

	t.Run("healthy report with tripped circuit breaker", func(t *testing.T) {
		r := NewAmbientTelemetryReport(
			WithStatus(TelemetryStatusHealthy),
			WithCircuitBreakerStatus(CircuitBreakerStatusTripped),
		)
		if r.IsHealthy() {
			t.Errorf("expected report with tripped circuit breaker to return false from IsHealthy()")
		}
	})

	t.Run("healthy report with open circuit breaker", func(t *testing.T) {
		r := NewAmbientTelemetryReport(
			WithStatus(TelemetryStatusHealthy),
			WithCircuitBreakerStatus(CircuitBreakerStatusOpen),
		)
		if r.IsHealthy() {
			t.Errorf("expected report with open circuit breaker to return false from IsHealthy()")
		}
	})

	t.Run("healthy report with degraded component", func(t *testing.T) {
		r := NewAmbientTelemetryReport(
			WithStatus(TelemetryStatusHealthy),
			WithCircuitBreakerStatus(CircuitBreakerStatusClosed),
			WithComponent(ComponentHealth{Name: "good", Status: TelemetryStatusHealthy}),
			WithComponent(ComponentHealth{Name: "bad", Status: TelemetryStatusDegraded}),
		)
		if r.IsHealthy() {
			t.Errorf("expected report with degraded component to return false from IsHealthy()")
		}
	})

	t.Run("healthy report with critical component", func(t *testing.T) {
		r := NewAmbientTelemetryReport(
			WithStatus(TelemetryStatusHealthy),
			WithCircuitBreakerStatus(CircuitBreakerStatusClosed),
			WithComponent(ComponentHealth{Name: "bad", Status: TelemetryStatusCritical}),
		)
		if r.IsHealthy() {
			t.Errorf("expected report with critical component to return false from IsHealthy()")
		}
	})

	t.Run("healthy report with stopped component", func(t *testing.T) {
		r := NewAmbientTelemetryReport(
			WithStatus(TelemetryStatusHealthy),
			WithCircuitBreakerStatus(CircuitBreakerStatusClosed),
			WithComponent(ComponentHealth{Name: "bad", Status: TelemetryStatusStopped}),
		)
		if r.IsHealthy() {
			t.Errorf("expected report with stopped component to return false from IsHealthy()")
		}
	})
}

// TestTelemetry_SummaryTable verifies tabular output formatting, column headers,
// deterministic component sorting, and fallback handling.
func TestTelemetry_SummaryTable(t *testing.T) {
	t.Run("non-empty components", func(t *testing.T) {
		r := NewAmbientTelemetryReport(
			WithStatus(TelemetryStatusHealthy),
			WithUptime(3600*time.Second),
			WithActiveHolonsCount(12),
			WithMeshMetrics(HolonMeshMetrics{
				IngestRate:       250.5,
				ConvergenceScore: 100.0,
				DivergenceCount:  0,
				EventThroughput:  500.0,
				StreamLatencyMs:  0.8,
			}),
			WithComponent(ComponentHealth{
				Name:          "beta_engine",
				Status:        TelemetryStatusHealthy,
				Message:       "operational",
				LastHeartbeat: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC),
			}),
			WithComponent(ComponentHealth{
				Name:          "alpha_stream",
				Status:        TelemetryStatusHealthy,
				Message:       "streaming",
				LastHeartbeat: time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC),
			}),
			WithCircuitBreakerStatus(CircuitBreakerStatusClosed),
			WithRunwayWatermark(999),
		)

		table := r.SummaryTable()

		// Verify section headers
		expectedHeaders := []string{
			"AMBIENT RUNTIME TELEMETRY REPORT",
			"Status:",
			"Active Holons:",
			"Circuit Breaker:",
			"Runway Watermark:",
			"MESH METRICS",
			"Ingest Rate:",
			"Convergence Score:",
			"COMPONENTS",
		}
		for _, hdr := range expectedHeaders {
			if !strings.Contains(table, hdr) {
				t.Errorf("expected SummaryTable to contain %q, but was missing", hdr)
			}
		}

		// Verify alphabetical order: alpha_stream should appear before beta_engine
		alphaIdx := strings.Index(table, "alpha_stream")
		betaIdx := strings.Index(table, "beta_engine")
		if alphaIdx == -1 || betaIdx == -1 || alphaIdx >= betaIdx {
			t.Errorf("expected components to be sorted alphabetically: alphaIdx=%d, betaIdx=%d", alphaIdx, betaIdx)
		}
	})

	t.Run("empty components", func(t *testing.T) {
		r := NewAmbientTelemetryReport(
			WithStatus(TelemetryStatusStopped),
			WithComponents(nil),
		)
		table := r.SummaryTable()
		if !strings.Contains(table, "(no registered components)") {
			t.Errorf("expected table for empty components to indicate no registered components, got: %s", table)
		}
	})
}

// TestTelemetry_Clone verifies deep isolation when cloning an AmbientTelemetryReport.
func TestTelemetry_Clone(t *testing.T) {
	orig := NewAmbientTelemetryReport(
		WithStatus(TelemetryStatusHealthy),
		WithComponent(ComponentHealth{Name: "c1", Status: TelemetryStatusHealthy}),
	)

	cloned := orig.Clone()
	if cloned == nil {
		t.Fatalf("expected non-nil clone")
	}

	// Mutate clone and assert original is unmodified
	cloned.SetStatus(TelemetryStatusDegraded)
	cloned.SetComponent(ComponentHealth{Name: "c1", Status: TelemetryStatusDegraded})
	cloned.SetComponent(ComponentHealth{Name: "c2", Status: TelemetryStatusDegraded})

	if orig.GetStatus() != TelemetryStatusHealthy {
		t.Errorf("original report status mutated after clone modification")
	}
	origC1, hasC1 := orig.GetComponent("c1")
	if !hasC1 || origC1.Status != TelemetryStatusHealthy {
		t.Errorf("original report component mutated after clone modification")
	}
	if _, ok := orig.GetComponent("c2"); ok {
		t.Errorf("original report gained component added to clone")
	}
}

// TestTelemetry_ConcurrencySafety runs simultaneous readers and writers to verify
// thread safety under the Go race detector (-race).
func TestTelemetry_ConcurrencySafety(t *testing.T) {
	report := NewAmbientTelemetryReport(
		WithStatus(TelemetryStatusHealthy),
		WithActiveHolonsCount(10),
		WithComponent(ComponentHealth{Name: "core", Status: TelemetryStatusHealthy}),
	)

	const goroutines = 16
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		workerID := i
		goroutinelabels.NewGoroutine("telemetry_test_concurrency", "test concurrency safety").StartSimple(func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				switch j % 6 {
				case 0:
					if report.IsHealthy() {
						workerID++
					}
				case 1:
					if b, err := report.ToJSON(); err == nil && len(b) > 0 {
						workerID++
					}
				case 2:
					if table := report.SummaryTable(); len(table) > 0 {
						workerID++
					}
				case 3:
					report.SetStatus(fmt.Sprintf("status_%d", workerID))
				case 4:
					report.SetComponent(ComponentHealth{
						Name:   fmt.Sprintf("comp_%d", workerID),
						Status: TelemetryStatusHealthy,
					})
				case 5:
					if c := report.Clone(); c != nil {
						workerID++
					}
				}
			}
		})
	}

	wg.Wait()
}

// Traceability Verification:
// Requirement: REQ-1793619495851164000-2ffc306d (Ambient Holon Mesh Telemetry & Unified Runtime CLI Dashboard)
// Criteria: CRIT-1793619535703058000-587624a3 (Negative Boundary: Graceful degradation on offline daemon, corrupt state files, and channel timeouts)

// TestTelemetry_ResilientDegradation_CorruptStateFile verifies that SafeLoadTelemetryReport
// safely degrades without panicking when reading corrupt, truncated, or malformed state files.
func TestTelemetry_ResilientDegradation_CorruptStateFile(t *testing.T) {
	tempDir := t.TempDir()

	scenarios := []struct {
		name        string
		fileContent string
	}{
		{
			name:        "truncated JSON",
			fileContent: `{"status": "healthy", "uptime": 12000, "active_holons_count":`,
		},
		{
			name:        "random binary garbage",
			fileContent: "\x00\xff\xfe\x01\x10\x7f\x80\x90garbage-binary-data",
		},
		{
			name:        "invalid field types",
			fileContent: `{"mesh_metrics": "invalid-string-not-an-object", "components": 12345}`,
		},
		{
			name:        "malformed syntax",
			fileContent: `{"status": "healthy",, "uptime": 500}`,
		},
		{
			name:        "unterminated string array",
			fileContent: `{"warnings": ["unterminated`,
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			filePath := filepath.Join(tempDir, fmt.Sprintf("corrupt_%s.json", strings.ReplaceAll(sc.name, " ", "_")))
			if err := fileutil.WriteFile(filePath, []byte(sc.fileContent), paths.FilePerm600); err != nil {
				t.Fatalf("failed to write test fixture %s: %v", filePath, err)
			}

			// Must not panic under any corrupted payload
			var report *AmbientTelemetryReport
			var loadErr error
			didPanic := func() (panicked bool) {
				defer func() {
					if r := recover(); r != nil {
						panicked = true
					}
				}()
				report, loadErr = SafeLoadTelemetryReport(filePath)
				return false
			}()

			if didPanic {
				t.Fatalf("SafeLoadTelemetryReport panicked on %s", sc.name)
			}

			// Must return non-nil degraded report
			if report == nil {
				t.Fatalf("expected non-nil fallback report on %s, got nil", sc.name)
			}

			// Degraded status invariant
			if report.GetStatus() != TelemetryStatusDegraded {
				t.Errorf("expected status %q, got %q", TelemetryStatusDegraded, report.GetStatus())
			}

			// Circuit breaker must be tripped
			if report.GetCircuitBreakerStatus() != CircuitBreakerStatusTripped {
				t.Errorf("expected circuit breaker %q, got %q", CircuitBreakerStatusTripped, report.GetCircuitBreakerStatus())
			}

			// Must return ErrCorruptAmbientState
			if !errors.Is(loadErr, ErrCorruptAmbientState) {
				t.Errorf("expected ErrCorruptAmbientState on %s, got: %v", sc.name, loadErr)
			}

			// Must have diagnostics in warnings
			warnings := report.GetWarnings()
			if len(warnings) == 0 {
				t.Errorf("expected non-empty warnings on %s", sc.name)
			}

			// Degraded report must fail IsHealthy check
			if report.IsHealthy() {
				t.Errorf("expected report.IsHealthy() to be false for corrupt file %s", sc.name)
			}

			// Summary table must render without panicking
			table := report.SummaryTable()
			if !strings.Contains(table, "WARNINGS") {
				t.Errorf("expected SummaryTable to display WARNINGS section for %s", sc.name)
			}
		})
	}
}

// TestTelemetry_ResilientDegradation_MissingFile verifies graceful degradation when
// the ambient daemon is stopped and the telemetry state file does not exist or has an empty path.
func TestTelemetry_ResilientDegradation_MissingFile(t *testing.T) {
	t.Run("non-existent file", func(t *testing.T) {
		tempDir := t.TempDir()
		missingPath := filepath.Join(tempDir, "does-not-exist", "ambient_telemetry.json")

		report, err := SafeLoadTelemetryReport(missingPath)
		if report == nil {
			t.Fatalf("expected non-nil degraded report on missing file")
		}
		if !errors.Is(err, ErrStateFileNotFound) {
			t.Errorf("expected ErrStateFileNotFound on missing file, got: %v", err)
		}
		if report.GetStatus() != TelemetryStatusStopped {
			t.Errorf("expected status %q for missing daemon state file, got %q", TelemetryStatusStopped, report.GetStatus())
		}
		if report.GetCircuitBreakerStatus() != CircuitBreakerStatusTripped {
			t.Errorf("expected circuit breaker tripped, got %q", report.GetCircuitBreakerStatus())
		}
		if report.IsHealthy() {
			t.Errorf("expected IsHealthy() to be false for missing file")
		}
		if len(report.GetWarnings()) == 0 {
			t.Errorf("expected warnings on missing state file")
		}
	})

	t.Run("empty path string", func(t *testing.T) {
		report, err := SafeLoadTelemetryReport("")
		if report == nil {
			t.Fatalf("expected non-nil degraded report on empty path")
		}
		if !errors.Is(err, ErrStatePathRequired) {
			t.Errorf("expected ErrStatePathRequired, got: %v", err)
		}
		if report.GetStatus() != TelemetryStatusDegraded {
			t.Errorf("expected status %q, got %q", TelemetryStatusDegraded, report.GetStatus())
		}
		if report.IsHealthy() {
			t.Errorf("expected IsHealthy() false on empty path")
		}
	})

	t.Run("whitespace-only path string", func(t *testing.T) {
		report, err := SafeLoadTelemetryReport("   \t  \n  ")
		if report == nil {
			t.Fatalf("expected non-nil degraded report on whitespace path")
		}
		if !errors.Is(err, ErrStatePathRequired) {
			t.Errorf("expected ErrStatePathRequired, got: %v", err)
		}
	})
}

// TestTelemetry_ResilientDegradation_EmptyFile verifies graceful degradation when
// state files are 0-byte truncated or contain only whitespace.
func TestTelemetry_ResilientDegradation_EmptyFile(t *testing.T) {
	tempDir := t.TempDir()

	scenarios := []struct {
		name    string
		content []byte
	}{
		{
			name:    "zero byte file",
			content: []byte(""),
		},
		{
			name:    "whitespace only file",
			content: []byte("   \n\t\r\n   "),
		},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			filePath := filepath.Join(tempDir, fmt.Sprintf("empty_%s.json", strings.ReplaceAll(sc.name, " ", "_")))
			if err := fileutil.WriteFile(filePath, sc.content, paths.FilePerm600); err != nil {
				t.Fatalf("failed to write empty file fixture: %v", err)
			}

			report, err := SafeLoadTelemetryReport(filePath)
			if report == nil {
				t.Fatalf("expected non-nil degraded report on %s", sc.name)
			}
			if !errors.Is(err, ErrEmptyAmbientState) {
				t.Errorf("expected ErrEmptyAmbientState on %s, got: %v", sc.name, err)
			}
			if report.GetStatus() != TelemetryStatusDegraded {
				t.Errorf("expected status %q, got %q", TelemetryStatusDegraded, report.GetStatus())
			}
			if report.GetCircuitBreakerStatus() != CircuitBreakerStatusTripped {
				t.Errorf("expected circuit breaker tripped, got %q", report.GetCircuitBreakerStatus())
			}
			if report.IsHealthy() {
				t.Errorf("expected IsHealthy() to be false for %s", sc.name)
			}
			if len(report.GetWarnings()) == 0 {
				t.Errorf("expected warnings on %s", sc.name)
			}
		})
	}
}

func newSampleReport() *AmbientTelemetryReport {
	return NewAmbientTelemetryReport(
		WithStatus(TelemetryStatusHealthy),
		WithActiveHolonsCount(5),
	)
}

// TestTelemetry_ResilientDegradation_TimeoutAndChannelClosure verifies resilient
// channel emission boundaries, timeout failure, and panic-free handling on closed channels.
func TestTelemetry_ResilientDegradation_TimeoutAndChannelClosure(t *testing.T) {
	ctx := context.Background()
	sampleReport := newSampleReport()

	t.Run("nil channel safety", func(t *testing.T) {
		if err := EmitTelemetryReport(ctx, nil, sampleReport); !errors.Is(err, ErrNilTelemetryChannel) {
			t.Errorf("expected ErrNilTelemetryChannel on non-blocking emit, got %v", err)
		}
		if err := EmitTelemetryReportWithTimeout(ctx, nil, sampleReport, 50*time.Millisecond); !errors.Is(err, ErrNilTelemetryChannel) {
			t.Errorf("expected ErrNilTelemetryChannel on timeout emit, got %v", err)
		}
		if err := SafeEmitTelemetryReport(ctx, nil, sampleReport, 50*time.Millisecond); !errors.Is(err, ErrNilTelemetryChannel) {
			t.Errorf("expected ErrNilTelemetryChannel on safe emit, got %v", err)
		}
	})

	t.Run("nil report safety", func(t *testing.T) {
		ch := make(chan *AmbientTelemetryReport, 1)
		if err := EmitTelemetryReport(ctx, ch, nil); !errors.Is(err, ErrNilTelemetryReport) {
			t.Errorf("expected ErrNilTelemetryReport, got %v", err)
		}
		if err := EmitTelemetryReportWithTimeout(ctx, ch, nil, 50*time.Millisecond); !errors.Is(err, ErrNilTelemetryReport) {
			t.Errorf("expected ErrNilTelemetryReport, got %v", err)
		}
		if err := SafeEmitTelemetryReport(ctx, ch, nil, 50*time.Millisecond); !errors.Is(err, ErrNilTelemetryReport) {
			t.Errorf("expected ErrNilTelemetryReport, got %v", err)
		}
	})

	t.Run("channel buffer full non-blocking rejection", func(t *testing.T) {
		ch := make(chan *AmbientTelemetryReport, 1)
		// First emit should succeed
		if err := EmitTelemetryReport(ctx, ch, sampleReport); err != nil {
			t.Fatalf("first emit failed: %v", err)
		}
		// Second emit to full channel must return ErrTelemetryChannelFull immediately
		if err := EmitTelemetryReport(ctx, ch, sampleReport); !errors.Is(err, ErrTelemetryChannelFull) {
			t.Errorf("expected ErrTelemetryChannelFull on saturated channel, got: %v", err)
		}
	})

	t.Run("channel timeout on full buffer", func(t *testing.T) {
		ch := make(chan *AmbientTelemetryReport, 1)
		ch <- sampleReport // saturate buffer

		timeout := 25 * time.Millisecond
		start := time.Now()
		err := EmitTelemetryReportWithTimeout(ctx, ch, sampleReport, timeout)
		duration := time.Since(start)

		if !errors.Is(err, ErrTelemetryTimeout) {
			t.Errorf("expected ErrTelemetryTimeout on saturated buffer, got %v", err)
		}
		if duration < timeout {
			t.Errorf("expected emit to wait at least %v, returned after %v", timeout, duration)
		}
	})

	t.Run("context cancellation aborts emit", func(t *testing.T) {
		ch := make(chan *AmbientTelemetryReport, 1)
		ch <- sampleReport // saturate

		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		err := EmitTelemetryReportWithTimeout(cancelledCtx, ch, sampleReport, 500*time.Millisecond)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}

// TestTelemetry_ResilientDegradation_SafeEmitClosedChannel verifies panic recovery and delivery on closed channels.
func TestTelemetry_ResilientDegradation_SafeEmitClosedChannel(t *testing.T) {
	ctx := context.Background()
	sampleReport := newSampleReport()

	t.Run("safe emit panic containment on closed channel", func(t *testing.T) {
		ch := make(chan *AmbientTelemetryReport, 1)
		close(ch)

		// Regular Go send on closed channel panics. SafeEmitTelemetryReport MUST catch it.
		var err error
		didPanic := func() (panicked bool) {
			defer func() {
				if r := recover(); r != nil {
					panicked = true
				}
			}()
			err = SafeEmitTelemetryReport(ctx, ch, sampleReport, 50*time.Millisecond)
			return false
		}()

		if didPanic {
			t.Fatalf("SafeEmitTelemetryReport panicked on closed channel!")
		}
		if !errors.Is(err, ErrClosedTelemetryChannel) {
			t.Errorf("expected ErrClosedTelemetryChannel on closed channel, got: %v", err)
		}
	})

	t.Run("successful delivery through SafeEmitTelemetryReport", func(t *testing.T) {
		ch := make(chan *AmbientTelemetryReport, 2)
		if err := SafeEmitTelemetryReport(ctx, ch, sampleReport, 50*time.Millisecond); err != nil {
			t.Fatalf("expected successful send, got %v", err)
		}
		received := <-ch
		if received.GetActiveHolonsCount() != sampleReport.GetActiveHolonsCount() {
			t.Errorf("received report mismatch")
		}
	})
}

// TestTelemetry_ResilientDegradation_SaveAndReload verifies disk persistence and reload boundaries.
func TestTelemetry_ResilientDegradation_SaveAndReload(t *testing.T) {
	sampleReport := newSampleReport()
	tempDir := t.TempDir()
	targetFile := filepath.Join(tempDir, "saved_telemetry.json")

	// Validation on empty path
	if err := SaveTelemetryReport("", sampleReport); !errors.Is(err, ErrStatePathRequired) {
		t.Errorf("expected ErrStatePathRequired on empty path, got %v", err)
	}

	// Validation on nil report
	if err := SaveTelemetryReport(targetFile, nil); !errors.Is(err, ErrNilTelemetryReport) {
		t.Errorf("expected ErrNilTelemetryReport on nil report, got %v", err)
	}

	// Valid save
	if err := SaveTelemetryReport(targetFile, sampleReport); err != nil {
		t.Fatalf("failed to save telemetry report: %v", err)
	}

	// Valid reload
	reloaded, err := SafeLoadTelemetryReport(targetFile)
	if err != nil {
		t.Fatalf("failed to load saved telemetry report: %v", err)
	}
	if reloaded.GetStatus() != sampleReport.GetStatus() {
		t.Errorf("reloaded status mismatch: got %q, want %q", reloaded.GetStatus(), sampleReport.GetStatus())
	}
	if reloaded.GetActiveHolonsCount() != sampleReport.GetActiveHolonsCount() {
		t.Errorf("reloaded holon count mismatch: got %d, want %d", reloaded.GetActiveHolonsCount(), sampleReport.GetActiveHolonsCount())
	}
}
