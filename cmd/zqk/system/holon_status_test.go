package system

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/ambient"
)

func TestHolonStatusCommand_HumanOutput(t *testing.T) {
	var buf bytes.Buffer
	err := runHolonStatus(&buf, false)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Middle-Brain Holon Vitality & Telemetry Dashboard") {
		t.Fatalf("expected dashboard banner in output, got: %s", out)
	}
	if !strings.Contains(out, "ambient-stream-hydrator") {
		t.Fatalf("expected hydrator holon in output, got: %s", out)
	}
	if !strings.Contains(out, "ambient-river-convergence") {
		t.Fatalf("expected river engine in output, got: %s", out)
	}
	if !strings.Contains(out, "SYSTEM VITALS") {
		t.Fatalf("expected SYSTEM VITALS table in output, got: %s", out)
	}
	if !strings.Contains(out, "MIDDLE-BRAIN HOLONS") {
		t.Fatalf("expected MIDDLE-BRAIN HOLONS table in output, got: %s", out)
	}
}

func TestHolonStatusCommand_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	err := runHolonStatus(&buf, true)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	var snap ambient.MiddleBrainTelemetrySnapshot
	if err := json.Unmarshal(buf.Bytes(), &snap); err != nil {
		t.Fatalf("failed to decode JSON output: %v", err)
	}

	if snap.ActiveHolonCount < 2 {
		t.Fatalf("expected at least 2 active holons, got %d", snap.ActiveHolonCount)
	}
	if snap.HealthStatus == "" {
		t.Fatal("expected non-empty health status")
	}
}

func TestNewHolonStatusCmd(t *testing.T) {
	cmd := NewHolonStatusCmd()
	if cmd.Use != "holon-status" {
		t.Fatalf("expected use 'holon-status', got %s", cmd.Use)
	}
	if cmd.Flags().Lookup("json") == nil {
		t.Fatal("expected 'json' flag on command")
	}
	if cmd.Flags().Lookup("format") == nil {
		t.Fatal("expected 'format' flag on command")
	}
}

// TestHolonStatus_TableDrivenOutputFormats satisfies CRIT-1793589814624126000-aa123817
// by verifying table-driven format flags across both human-readable tabular and structured JSON.
func TestHolonStatus_TableDrivenOutputFormats(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		expectJSON   bool
		expectSubstr string
	}{
		{
			name:         "default format is table",
			args:         []string{},
			expectJSON:   false,
			expectSubstr: "Middle-Brain Holon Vitality & Telemetry Dashboard",
		},
		{
			name:         "explicit format table",
			args:         []string{"--format", "table"},
			expectJSON:   false,
			expectSubstr: "SYSTEM VITALS",
		},
		{
			name:         "explicit format json",
			args:         []string{"--format", "json"},
			expectJSON:   true,
			expectSubstr: `"health_status"`,
		},
		{
			name:         "shorthand format json (-f json)",
			args:         []string{"-f", "json"},
			expectJSON:   true,
			expectSubstr: `"active_holon_count"`,
		},
		{
			name:         "boolean json flag (--json)",
			args:         []string{"--json"},
			expectJSON:   true,
			expectSubstr: `"timestamp"`,
		},
		{
			name:         "case-insensitive format flag (--format JSON)",
			args:         []string{"--format", "JSON"},
			expectJSON:   true,
			expectSubstr: `"stream_count"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cmd := NewHolonStatusSubCmd()
			var buf bytes.Buffer
			cmd.SetOut(&buf)
			cmd.SetArgs(tc.args)

			if err := cmd.Execute(); err != nil {
				t.Fatalf("unexpected execute error: %v", err)
			}

			output := buf.String()
			if !strings.Contains(output, tc.expectSubstr) {
				t.Fatalf("expected output to contain %q, got:\n%s", tc.expectSubstr, output)
			}

			if tc.expectJSON {
				var snap ambient.MiddleBrainTelemetrySnapshot
				if err := json.Unmarshal(buf.Bytes(), &snap); err != nil {
					t.Fatalf("failed to decode JSON output: %v\nOutput: %s", err, output)
				}
				if snap.HealthStatus == "" {
					t.Fatalf("expected valid health status in JSON output")
				}
			}
		})
	}
}

// TestHolonStatus_BoundaryAndErrorRecovery satisfies CRIT-1793589814624126000-aa123817
// by verifying nil-safe error recovery when ambient holon telemetry providers or stream hydrators
// are uninitialized, nil, or empty.
func TestHolonStatus_BoundaryAndErrorRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	healthyReg := ambient.NewHolonLifecycleRegistry()
	_ = healthyReg.Register("test-worker", "Test worker holon")
	_ = healthyReg.Transition("test-worker", ambient.HolonPhaseStarting)
	_ = healthyReg.Transition("test-worker", ambient.HolonPhaseRunning)

	healthyHydrator := ambient.NewStreamHydrator(ambient.DefaultStreamCapacity, ambient.DefaultMinWeightFloor)

	healthyEngine, err := ambient.NewRiverConvergenceEngine(ctx, ambient.RiverConfig{
		RootVisionID:       "VIS-ROOT-TEST",
		RootVisionVector:   []float64{1.0, 0.0, 0.0},
		ConvergenceWindow:  ambient.DefaultConvergenceWindow,
		MinConsensusDrops:  1,
		AlignmentThreshold: 0.70,
	})
	if err == nil && healthyEngine != nil {
		defer healthyEngine.Close()
	}

	emptyReg := ambient.NewHolonLifecycleRegistry()
	emptyHydrator := ambient.NewStreamHydrator(ambient.DefaultStreamCapacity, ambient.DefaultMinWeightFloor)

	tests := []struct {
		name                string
		aggregator          *ambient.HolonTelemetryAggregator
		expectedHealth      string
		expectedWarningText string
		expectNoHolonsText  bool
	}{
		{
			name:                "nil aggregator receiver recovery",
			aggregator:          nil,
			expectedHealth:      ambient.TelemetryHealthQuiescent,
			expectedWarningText: "nil telemetry aggregator receiver",
			expectNoHolonsText:  true,
		},
		{
			name:                "all nil providers recovery",
			aggregator:          ambient.NewHolonTelemetryAggregator(nil, nil, nil),
			expectedHealth:      ambient.TelemetryHealthQuiescent,
			expectedWarningText: "holon registry not configured",
			expectNoHolonsText:  true,
		},
		{
			name:                "nil registry provider recovery",
			aggregator:          ambient.NewHolonTelemetryAggregator(nil, healthyHydrator, healthyEngine),
			expectedHealth:      ambient.TelemetryHealthQuiescent,
			expectedWarningText: "holon registry not configured",
			expectNoHolonsText:  true,
		},
		{
			name:                "nil hydrator provider recovery",
			aggregator:          ambient.NewHolonTelemetryAggregator(healthyReg, nil, healthyEngine),
			expectedHealth:      ambient.TelemetryHealthDegraded,
			expectedWarningText: "stream hydrator not configured",
			expectNoHolonsText:  false,
		},
		{
			name:                "nil river engine provider recovery",
			aggregator:          ambient.NewHolonTelemetryAggregator(healthyReg, healthyHydrator, nil),
			expectedHealth:      ambient.TelemetryHealthDegraded,
			expectedWarningText: "river convergence engine not configured",
			expectNoHolonsText:  false,
		},
		{
			name:                "empty initialized providers recovery",
			aggregator:          ambient.NewHolonTelemetryAggregator(emptyReg, emptyHydrator, nil),
			expectedHealth:      ambient.TelemetryHealthQuiescent,
			expectedWarningText: "river convergence engine not configured",
			expectNoHolonsText:  true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name+"_tabular", func(t *testing.T) {
			var buf bytes.Buffer
			err := runHolonStatusWithAggregator(&buf, false, tc.aggregator)
			if err != nil {
				t.Fatalf("expected nil error on boundary recovery, got %v", err)
			}

			output := buf.String()
			if !strings.Contains(output, "Middle-Brain Holon Vitality & Telemetry Dashboard") {
				t.Fatalf("expected dashboard banner in output, got: %s", output)
			}
			if !strings.Contains(output, tc.expectedHealth) {
				t.Fatalf("expected health status %q in output, got:\n%s", tc.expectedHealth, output)
			}
			if tc.expectedWarningText != "" && !strings.Contains(output, tc.expectedWarningText) {
				t.Fatalf("expected warning %q in output, got:\n%s", tc.expectedWarningText, output)
			}
			if tc.expectNoHolonsText && !strings.Contains(output, "(no holons registered)") {
				t.Fatalf("expected '(no holons registered)' in output, got:\n%s", output)
			}
		})

		t.Run(tc.name+"_json", func(t *testing.T) {
			var buf bytes.Buffer
			err := runHolonStatusWithAggregator(&buf, true, tc.aggregator)
			if err != nil {
				t.Fatalf("expected nil error on boundary recovery json, got %v", err)
			}

			var snap ambient.MiddleBrainTelemetrySnapshot
			if err := json.Unmarshal(buf.Bytes(), &snap); err != nil {
				t.Fatalf("failed to decode JSON output: %v\nOutput: %s", err, buf.String())
			}
			if snap.HealthStatus != tc.expectedHealth {
				t.Fatalf("expected health %q in snapshot, got %q", tc.expectedHealth, snap.HealthStatus)
			}
			if tc.expectedWarningText != "" {
				found := false
				for _, w := range snap.Warnings {
					if strings.Contains(w, tc.expectedWarningText) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected warning containing %q in warnings %v", tc.expectedWarningText, snap.Warnings)
				}
			}
		})
	}
}

// TestRenderHolonTelemetry_NilSnapshot tests that RenderHolonTelemetry handles a completely nil snapshot gracefully.
func TestRenderHolonTelemetry_NilSnapshot(t *testing.T) {
	var bufTable bytes.Buffer
	if err := RenderHolonTelemetry(&bufTable, nil, false); err != nil {
		t.Fatalf("expected nil error on nil snapshot tabular render, got: %v", err)
	}
	if !strings.Contains(bufTable.String(), "telemetry snapshot uninitialized") {
		t.Fatalf("expected fallback warning in tabular output, got: %s", bufTable.String())
	}

	var bufJSON bytes.Buffer
	if err := RenderHolonTelemetry(&bufJSON, nil, true); err != nil {
		t.Fatalf("expected nil error on nil snapshot json render, got: %v", err)
	}
	var snap ambient.MiddleBrainTelemetrySnapshot
	if err := json.Unmarshal(bufJSON.Bytes(), &snap); err != nil {
		t.Fatalf("failed to decode JSON from nil snapshot fallback: %v", err)
	}
	if snap.HealthStatus != ambient.TelemetryHealthQuiescent {
		t.Fatalf("expected quiescent health status, got: %s", snap.HealthStatus)
	}
}

// TestHolonCommands_HierarchyAndRegistration verifies the full command hierarchy
// `zqk system holon status`, `zqk system holon`, and `zqk system holon-status`.
func TestHolonCommands_HierarchyAndRegistration(t *testing.T) {
	parentCmd := NewHolonCmd()
	if parentCmd.Use != "holon" {
		t.Fatalf("expected use 'holon', got %s", parentCmd.Use)
	}

	statusSubCmd, _, err := parentCmd.Find([]string{"status"})
	if err != nil || statusSubCmd == nil {
		t.Fatalf("expected 'status' subcommand under 'holon', got err: %v", err)
	}
	if statusSubCmd.Use != "status" {
		t.Fatalf("expected subcommand use 'status', got %s", statusSubCmd.Use)
	}

	// Verify executing parent command directly
	var bufParent bytes.Buffer
	parentCmd.SetOut(&bufParent)
	parentCmd.SetArgs([]string{"--format", "json"})
	if err := parentCmd.Execute(); err != nil {
		t.Fatalf("failed to execute parent holon command: %v", err)
	}
	var snap ambient.MiddleBrainTelemetrySnapshot
	if err := json.Unmarshal(bufParent.Bytes(), &snap); err != nil {
		t.Fatalf("failed to decode json from parent command: %v", err)
	}

	// Verify executing status subcommand via parent
	var bufSub bytes.Buffer
	parentCmd.SetOut(&bufSub)
	parentCmd.SetArgs([]string{"status", "--format", "json"})
	if err := parentCmd.Execute(); err != nil {
		t.Fatalf("failed to execute status subcommand via parent: %v", err)
	}
	var snapSub ambient.MiddleBrainTelemetrySnapshot
	if err := json.Unmarshal(bufSub.Bytes(), &snapSub); err != nil {
		t.Fatalf("failed to decode json from status subcommand: %v", err)
	}

	// Verify standalone holon-status command
	statusCmd := NewHolonStatusCmd()
	var bufStatus bytes.Buffer
	statusCmd.SetOut(&bufStatus)
	statusCmd.SetArgs([]string{"--format", "json"})
	if err := statusCmd.Execute(); err != nil {
		t.Fatalf("failed to execute holon-status command: %v", err)
	}
	var snapStandalone ambient.MiddleBrainTelemetrySnapshot
	if err := json.Unmarshal(bufStatus.Bytes(), &snapStandalone); err != nil {
		t.Fatalf("failed to decode json from holon-status: %v", err)
	}
}

// TestNewSystemCmd_HolonRegistration verifies that NewSystemCmd registers both
// 'holon' and 'holon-status'.
func TestNewSystemCmd_HolonRegistration(t *testing.T) {
	sysCmd := NewSystemCmd()

	subCmd, _, err := sysCmd.Find([]string{"holon", "status"})
	if err != nil || subCmd == nil {
		t.Fatalf("expected to resolve 'system holon status', got err: %v", err)
	}

	compatCmd, _, err := sysCmd.Find([]string{"holon-status"})
	if err != nil || compatCmd == nil {
		t.Fatalf("expected to resolve 'system holon-status', got err: %v", err)
	}
}
