package ambient

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/ambient"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

// Traceability Verification:
// Requirement: REQ-1793619495851164000-2ffc306d (Ambient Holon Mesh Telemetry & Unified Runtime CLI Dashboard)
// Criteria: CRIT-1793619529893485000-ef59301b (Operational Proof: CLI status commands emit live engine telemetry and JSON output)

func setupRunningDaemonEnv(t *testing.T) (string, int) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	pid := os.Getpid()
	pidPath := ambientPIDFilePath(root)
	require.NoError(t, fileutil.EnsureDir(filepath.Dir(pidPath)))
	require.NoError(t, fileutil.WriteStandardFile(pidPath, []byte(strconv.Itoa(pid))))
	return root, pid
}

func executeStatusWithArgs(t *testing.T, args ...string) *bytes.Buffer {
	t.Helper()
	cmd := newStatusCmd()
	var buf bytes.Buffer
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return &buf
}

// TestAmbientStatus_OperationalProof_RunningTelemetryJSON verifies that zqk ambient status
// produces full runtime telemetry JSON when the ambient daemon is active.
func TestAmbientStatus_OperationalProof_RunningTelemetryJSON(t *testing.T) {
	root, pid := setupRunningDaemonEnv(t)

	formatArgs := [][]string{
		{"--format", "json"},
		{"-f", "json"},
		{"--json"},
	}

	for _, args := range formatArgs {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			buf := executeStatusWithArgs(t, args...)

			var payload map[string]any
			require.NoError(t, json.Unmarshal(buf.Bytes(), &payload), "output should be valid JSON: %s", buf.String())

			// 1. Core daemon attributes
			assert.Equal(t, "ambient", payload["component"])
			assert.Equal(t, "running", payload["status"])
			assert.Equal(t, float64(pid), payload["pid"])
			assert.Equal(t, root, payload["project_root"])

			// 2. Engine telemetry payload attributes
			assert.NotNil(t, payload["uptime"])
			assert.Equal(t, float64(4), payload["active_holons_count"])
			assert.Equal(t, float64(4), payload["active_holons"])
			assert.Equal(t, float64(0), payload["runway_watermark"])
			assert.Equal(t, "closed", payload["circuit_breaker_status"])

			// 3. Mesh metrics
			meshRaw, ok := payload["mesh_metrics"].(map[string]any)
			require.True(t, ok, "mesh_metrics should be a JSON object")
			assert.Contains(t, meshRaw, "convergence_score")
			assert.Contains(t, meshRaw, "divergence_count")
			assert.Contains(t, meshRaw, "event_throughput")

			// 4. Subcomponents
			componentsRaw, ok := payload["components"].(map[string]any)
			require.True(t, ok, "components should be a map")
			assert.Contains(t, componentsRaw, "intent_stream")
			assert.Contains(t, componentsRaw, "epoch_synchronizer")
			assert.Contains(t, componentsRaw, "backpressure_regulator")
			assert.Contains(t, componentsRaw, "fault_isolator")
		})
	}
}

// TestAmbientStatus_OperationalProof_RunningTelemetryTable verifies human-readable table output.
func TestAmbientStatus_OperationalProof_RunningTelemetryTable(t *testing.T) {
	_, pid := setupRunningDaemonEnv(t)
	buf := executeStatusWithArgs(t, "--format", "table")
	out := buf.String()

	assert.Contains(t, out, "AMBIENT RUNTIME TELEMETRY REPORT")
	assert.Contains(t, out, "running")
	assert.Contains(t, out, strconv.Itoa(pid))
	assert.Contains(t, out, "healthy")
	assert.Contains(t, out, "Active Holons:          4")
	assert.Contains(t, out, "Circuit Breaker:        closed")
	assert.Contains(t, out, "Runway Watermark:       0")
	assert.Contains(t, out, "MESH METRICS")
	assert.Contains(t, out, "COMPONENTS")
	assert.Contains(t, out, "intent_stream")
	assert.Contains(t, out, "epoch_synchronizer")
	assert.Contains(t, out, "backpressure_regulator")
	assert.Contains(t, out, "fault_isolator")
}

// TestAmbientStatus_OperationalProof_PersistedTelemetry verifies loading persisted telemetry from disk.
func TestAmbientStatus_OperationalProof_PersistedTelemetry(t *testing.T) {
	root, pid := setupRunningDaemonEnv(t)
	assert.Positive(t, pid)

	// Persist custom report
	persistedReport := ambient.NewAmbientTelemetryReport(
		ambient.WithStatus(ambient.TelemetryStatusHealthy),
		ambient.WithUptime(45*time.Second),
		ambient.WithActiveHolonsCount(6),
		ambient.WithCircuitBreakerStatus(ambient.CircuitBreakerStatusHalfOpen),
		ambient.WithRunwayWatermark(42),
		ambient.WithMeshMetrics(ambient.HolonMeshMetrics{
			IngestRate:       88.5,
			ConvergenceScore: 99.2,
			DivergenceCount:  1,
			EventThroughput:  250.0,
			StreamLatencyMs:  2.1,
		}),
	)
	stateDir := filepath.Join(paths.StateDirPath(root), "ambient")
	require.NoError(t, fileutil.EnsureDir(stateDir))
	telemetryFile := filepath.Join(stateDir, "telemetry.json")
	require.NoError(t, ambient.SaveTelemetryReport(telemetryFile, persistedReport))

	buf := executeStatusWithArgs(t, "--format", "json")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &payload))
	assert.Equal(t, float64(6), payload["active_holons_count"])
	assert.Equal(t, float64(42), payload["runway_watermark"])
	assert.Equal(t, "half-open", payload["circuit_breaker_status"])

	meshRaw := payload["mesh_metrics"].(map[string]any)
	assert.Equal(t, 88.5, meshRaw["ingest_rate"])
	assert.Equal(t, 99.2, meshRaw["convergence_score"])
	assert.Equal(t, float64(1), meshRaw["divergence_count"])
}

// TestAmbientStatus_OperationalProof_StoppedState verifies reporting when daemon is not running.
func TestAmbientStatus_OperationalProof_StoppedState(t *testing.T) {
	root := t.TempDir()
	t.Setenv("ZQK_PROJECT_ROOT", root)

	// 1. JSON
	bufJSON := executeStatusWithArgs(t, "--format", "json")
	var jsonPayload map[string]any
	require.NoError(t, json.Unmarshal(bufJSON.Bytes(), &jsonPayload))
	assert.Equal(t, "ambient", jsonPayload["component"])
	assert.Equal(t, "stopped", jsonPayload["status"])

	// 2. Table
	bufTable := executeStatusWithArgs(t)
	assert.Contains(t, bufTable.String(), "AMBIENT RUNTIME TELEMETRY REPORT")
	assert.Contains(t, bufTable.String(), "Status:                 stopped")
}

// TestAmbientStatus_ASTAudit verifies zero AST violations in ambient status code.
func TestAmbientStatus_ASTAudit(t *testing.T) {
	auditor := qa.NewASTAuditor()
	for _, file := range []string{"status.go", "status_test.go"} {
		violations, err := auditor.AuditFile(file)
		if err != nil {
			t.Fatalf("failed to audit %s: %v", file, err)
		}
		for _, v := range violations {
			if v.Severity == "high" || v.Severity == "medium" {
				t.Errorf("AST violation [%s] in %s at %s: %s", v.Severity, file, v.Pos, v.Message)
			}
		}
	}
}
