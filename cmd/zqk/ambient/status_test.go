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
// Backlog Item: BLI-1793625345977086000-f5c6d446
// Requirement:  REQ-1793625103975205000-90b23076
// Criteria:     CRIT-1793625127194880000-6df3a8f2 (Operational Proof: Live Consensus Task Dispatch & Reflex Evaluation Telemetry in CLI & JSON)
// Legacy Traceability:
// Requirement: REQ-1793619495851164000-2ffc306d (Ambient Holon Mesh Telemetry & Unified Runtime CLI Dashboard)
// Criteria:    CRIT-1793619529893485000-ef59301b (Operational Proof: CLI status commands emit live engine telemetry and JSON output)

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

func executeStatusJSON(t *testing.T) map[string]any {
	t.Helper()
	buf := executeStatusWithArgs(t, "--format", "json")
	var payload map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &payload))
	return payload
}

func persistTelemetryFile(t *testing.T, root string, report *ambient.AmbientTelemetryReport) {
	t.Helper()
	stateDir := filepath.Join(paths.StateDirPath(root), "ambient")
	require.NoError(t, fileutil.EnsureDir(stateDir))
	telemetryFile := filepath.Join(stateDir, "telemetry.json")
	require.NoError(t, ambient.SaveTelemetryReport(telemetryFile, report))
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
			assert.Equal(t, float64(6), payload["active_holons_count"])
			assert.Equal(t, float64(6), payload["active_holons"])
			assert.Equal(t, float64(0), payload["runway_watermark"])
			assert.Equal(t, "closed", payload["circuit_breaker_status"])
			assert.Equal(t, float64(0), payload["consensus_leases_active"])
			assert.Equal(t, float64(0), payload["consensus_committed"])
			assert.Equal(t, float64(0), payload["consensus_revoked"])
			assert.Equal(t, float64(0), payload["reflex_actions_total"])
			assert.Equal(t, float64(0), payload["reflex_realign_count"])

			// 3. Mesh metrics
			meshRaw, ok := payload["mesh_metrics"].(map[string]any)
			require.True(t, ok, "mesh_metrics should be a JSON object")
			assert.Contains(t, meshRaw, "convergence_score")
			assert.Contains(t, meshRaw, "divergence_count")
			assert.Contains(t, meshRaw, "event_throughput")
			assert.Contains(t, meshRaw, "consensus_leases_active")
			assert.Contains(t, meshRaw, "consensus_committed")
			assert.Contains(t, meshRaw, "consensus_revoked")
			assert.Contains(t, meshRaw, "reflex_actions_total")
			assert.Contains(t, meshRaw, "reflex_realign_count")

			// 4. Subcomponents
			componentsRaw, ok := payload["components"].(map[string]any)
			require.True(t, ok, "components should be a map")
			assert.Contains(t, componentsRaw, "intent_stream")
			assert.Contains(t, componentsRaw, "epoch_synchronizer")
			assert.Contains(t, componentsRaw, "backpressure_regulator")
			assert.Contains(t, componentsRaw, "fault_isolator")
			assert.Contains(t, componentsRaw, "consensus_arbiter")
			assert.Contains(t, componentsRaw, "reflex_loop")
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
	assert.Contains(t, out, "Active Holons:          6")
	assert.Contains(t, out, "Circuit Breaker:        closed")
	assert.Contains(t, out, "Runway Watermark:       0")
	assert.Contains(t, out, "MESH METRICS")
	assert.Contains(t, out, "Consensus Leases:     0 active")
	assert.Contains(t, out, "Consensus Committed:  0")
	assert.Contains(t, out, "Consensus Revoked:    0")
	assert.Contains(t, out, "Reflex Actions:       0 total")
	assert.Contains(t, out, "Reflex Realignments:  0")
	assert.Contains(t, out, "COMPONENTS")
	assert.Contains(t, out, "intent_stream")
	assert.Contains(t, out, "epoch_synchronizer")
	assert.Contains(t, out, "backpressure_regulator")
	assert.Contains(t, out, "fault_isolator")
	assert.Contains(t, out, "consensus_arbiter")
	assert.Contains(t, out, "reflex_loop")
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
		ambient.WithConsensusLeasesActive(5),
		ambient.WithConsensusCommitted(22),
		ambient.WithConsensusRevoked(3),
		ambient.WithReflexActionsTotal(40),
		ambient.WithReflexRealignCount(8),
		ambient.WithMeshMetrics(ambient.HolonMeshMetrics{
			IngestRate:            88.5,
			ConvergenceScore:      99.2,
			DivergenceCount:       1,
			EventThroughput:       250.0,
			StreamLatencyMs:       2.1,
			ConsensusLeasesActive: 5,
			ConsensusCommitted:    22,
			ConsensusRevoked:      3,
			ReflexActionsTotal:    40,
			ReflexRealignCount:    8,
		}),
	)
	persistTelemetryFile(t, root, persistedReport)

	payload := executeStatusJSON(t)
	assert.Equal(t, float64(6), payload["active_holons_count"])
	assert.Equal(t, float64(42), payload["runway_watermark"])
	assert.Equal(t, "half-open", payload["circuit_breaker_status"])
	assert.Equal(t, float64(5), payload["consensus_leases_active"])
	assert.Equal(t, float64(22), payload["consensus_committed"])
	assert.Equal(t, float64(3), payload["consensus_revoked"])
	assert.Equal(t, float64(40), payload["reflex_actions_total"])
	assert.Equal(t, float64(8), payload["reflex_realign_count"])

	meshRaw := payload["mesh_metrics"].(map[string]any)
	assert.Equal(t, 88.5, meshRaw["ingest_rate"])
	assert.Equal(t, 99.2, meshRaw["convergence_score"])
	assert.Equal(t, float64(1), meshRaw["divergence_count"])
	assert.Equal(t, float64(5), meshRaw["consensus_leases_active"])
	assert.Equal(t, float64(22), meshRaw["consensus_committed"])
	assert.Equal(t, float64(3), meshRaw["consensus_revoked"])
	assert.Equal(t, float64(40), meshRaw["reflex_actions_total"])
	assert.Equal(t, float64(8), meshRaw["reflex_realign_count"])

	bufTable := executeStatusWithArgs(t, "--format", "table")
	outTable := bufTable.String()
	assert.Contains(t, outTable, "Consensus Leases:     5 active")
	assert.Contains(t, outTable, "Consensus Committed:  22")
	assert.Contains(t, outTable, "Consensus Revoked:    3")
	assert.Contains(t, outTable, "Reflex Actions:       40 total")
	assert.Contains(t, outTable, "Reflex Realignments:  8")
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

// TestAmbientStatus_OperationalProof_LiveEngineConsensusReflexTelemetry verifies live telemetry query and CLI rendering.
func TestAmbientStatus_OperationalProof_LiveEngineConsensusReflexTelemetry(t *testing.T) {
	cfg := ambient.DefaultAmbientEngineConfig()
	cfg.TickInterval = 10 * time.Millisecond
	cfg.GracefulStopTimeout = time.Second
	engine, err := ambient.NewAmbientEngine(cfg)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, engine.Start(ctx))
	defer func() { require.NoError(t, engine.Stop()) }()

	arb := engine.ConsensusArbiter()
	require.NotNil(t, arb)
	_, err = arb.ProposeTask("live-task-1", []byte("payload-1"))
	require.NoError(t, err)
	_, err = arb.LeaseTask("live-task-1", "worker-live-1", 5*time.Second)
	require.NoError(t, err)

	_, err = arb.ProposeTask("live-task-2", []byte("payload-2"))
	require.NoError(t, err)
	_, err = arb.LeaseTask("live-task-2", "worker-live-1", 5*time.Second)
	require.NoError(t, err)
	require.NoError(t, arb.CommitTask("live-task-2", "worker-live-1"))

	_, err = arb.ProposeTask("live-task-3", []byte("payload-3"))
	require.NoError(t, err)
	require.NoError(t, arb.RevokeTask("live-task-3", "test revocation"))

	reflex := engine.ReflexLoop()
	require.NotNil(t, reflex)
	require.NoError(t, reflex.TriggerAction(ctx, ambient.ReflexAction{
		ID:        "act-live-1",
		Type:      ambient.ReflexActionRealignIntent,
		Target:    "intent_stream",
		Timestamp: time.Now().UTC(),
	}))

	report := engine.TelemetryReport()
	require.NotNil(t, report)
	assert.Equal(t, 1, report.GetConsensusLeasesActive())
	assert.Equal(t, uint64(1), report.GetConsensusCommitted())
	assert.Equal(t, uint64(1), report.GetConsensusRevoked())
	assert.Equal(t, uint64(1), report.GetReflexActionsTotal())
	assert.Equal(t, uint64(1), report.GetReflexRealignCount())

	root, daemonPID := setupRunningDaemonEnv(t)
	assert.Positive(t, daemonPID)
	persistTelemetryFile(t, root, report)

	payload := executeStatusJSON(t)
	assert.Equal(t, float64(1), payload["consensus_leases_active"])
	assert.Equal(t, float64(1), payload["consensus_committed"])
	assert.Equal(t, float64(1), payload["consensus_revoked"])
	assert.Equal(t, float64(1), payload["reflex_actions_total"])
	assert.Equal(t, float64(1), payload["reflex_realign_count"])

	bufTable := executeStatusWithArgs(t, "--format", "table")
	outTable := bufTable.String()
	assert.Contains(t, outTable, "Consensus Leases:     1 active")
	assert.Contains(t, outTable, "Consensus Committed:  1")
	assert.Contains(t, outTable, "Consensus Revoked:    1")
	assert.Contains(t, outTable, "Reflex Actions:       1 total")
	assert.Contains(t, outTable, "Reflex Realignments:  1")
	assert.Contains(t, outTable, "consensus_arbiter")
	assert.Contains(t, outTable, "reflex_loop")
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
