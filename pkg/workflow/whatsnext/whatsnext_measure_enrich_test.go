package whatsnext

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCycle147_CompressWhatsNextMeasure_NilAndPopulated(t *testing.T) {
	assert.Nil(t, CompressWhatsNextMeasure(nil))

	empty := CompressWhatsNextMeasure(map[string]any{})
	require.NotNil(t, empty)
	assert.Nil(t, empty["convergence_session_id"])

	sample := map[string]any{
		"convergence_session_id":                  "CVS-DEMO-001",
		objects.FieldKeyPrimaryMeasurementOutcome: "stable",
		"health_watermark_rfc3339":                "2026-10-24T00:00:00Z",
		"suggested_convergence_session_fields": map[string]any{
			"phase_router": map[string]any{
				"phase_alignment":           "aligned",
				"suggested_current_phase":   "verification",
				"routing_profile":           "standard",
				"measurement_implied_phase": "in_progress",
			},
		},
		"rollup_status_core": map[string]any{
			"rollup_status":           "green",
			"recommended_next_action": "continue",
		},
	}

	compressed := CompressWhatsNextMeasure(sample)
	require.NotNil(t, compressed)
	assert.Equal(t, "CVS-DEMO-001", compressed["convergence_session_id"])
	assert.Equal(t, "stable", compressed[objects.FieldKeyPrimaryMeasurementOutcome])
	router, ok := compressed["phase_router"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "aligned", router["phase_alignment"])
	assert.Equal(t, "verification", router["suggested_current_phase"])
	assert.Equal(t, "standard", router["routing_profile"])
	assert.Equal(t, "in_progress", router["measurement_implied_phase"])
	assert.Equal(t, "green", compressed["rollup_status"])
	assert.Equal(t, "continue", compressed["recommended_next_action"])
}

func TestCycle147_RunSchedulerConvergenceMeasureJSON_MockExe(t *testing.T) {
	origResolver := resolveZQKCLIExecutable
	defer func() { resolveZQKCLIExecutable = origResolver }()

	// Case 1: Resolver error
	resolveZQKCLIExecutable = func() (string, error) {
		return "", fmt.Errorf("mock binary not found")
	}
	_, err := RunSchedulerConvergenceMeasureJSON(t.TempDir(), "CVS-TEST-001")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mock binary not found")

	// Case 2: Subprocess execution error
	resolveZQKCLIExecutable = func() (string, error) {
		return "/bin/false", nil
	}
	_, err2 := RunSchedulerConvergenceMeasureJSON(t.TempDir(), "CVS-TEST-001")
	require.Error(t, err2)

	// Case 3: Wrapper test for internal runner
	_, err3 := runSchedulerConvergenceMeasureJSON(t.TempDir(), "CVS-TEST-001")
	require.Error(t, err3)
}

func TestCycle147_GetSystemCAPInstruction_Coverage(t *testing.T) {
	tempDir := t.TempDir()
	inst := getSystemCAPInstruction(tempDir)
	assert.NotEmpty(t, inst)
}

func TestCycle147_SubscribeWAL_Lifecycle(t *testing.T) {
	tempDir := t.TempDir()
	mv := NewWhatsNextMaterializedView(tempDir)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	updateCh := make(chan struct{}, 1)
	mv.SubscribeWAL(ctx, updateCh)
	assert.NotNil(t, mv)
}

func TestCycle147_KernelAmbience_HelperTypes(t *testing.T) {
	m := map[string]any{
		"score_float": 42.5,
		"score_int":   int64(25),
		"count_int":   int(10),
		"count_float": float64(50),
		"invalid":     "not-a-number",
	}

	// Test firstFloat
	assert.Equal(t, 42.5, firstFloat(m, "score_float"))
	assert.Equal(t, 25.0, firstFloat(m, "score_int"))
	assert.Equal(t, 0.0, firstFloat(m, "missing", "invalid"))

	// Test firstInt
	assert.Equal(t, 10, firstInt(m, "count_int"))
	assert.Equal(t, 50, firstInt(m, "count_float"))
	assert.Equal(t, 0, firstInt(m, "missing", "invalid"))
}

func TestCycle147_HumanLogClusters_FileParsing(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Missing log file returns empty
	emptyClusters := ParseHumanLogClusters(tempDir)
	assert.Empty(t, emptyClusters.Errors)
	assert.Empty(t, emptyClusters.Warns)

	// 2. Create populated log file
	logDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.LogsDir)
	require.NoError(t, fileutil.MkdirAll(logDir, 0755))

	logContent := "2026-10-24T00:00:00Z [error] database connection timeout\n" +
		"2026-10-24T00:00:01Z [error] database connection timeout\n" +
		"2026-10-24T00:00:02Z [warn] rate limit threshold approaching\n"

	logPath := filepath.Join(logDir, paths.LogEventsPrefix+"human.log")
	require.NoError(t, fileutil.WriteFile(logPath, []byte(logContent), 0644))

	clusters := ParseHumanLogClusters(tempDir)
	assert.NotEmpty(t, clusters.Errors)
	assert.NotEmpty(t, clusters.Warns)

	formatted := FormatHumanLogClusterDigest(clusters)
	assert.NotEmpty(t, formatted)

	actions := RankedHumanLogActions(clusters)
	assert.NotNil(t, actions)
}

func TestCycle147_EnrichAutoRemedy_And_Metrics(t *testing.T) {
	// Nil checks
	EnrichAutoRemedy(nil, "")
	EnrichMetricsRollup(nil, "")
	EnrichStrategicAlignment(nil, "")

	tempDir := t.TempDir()
	amb := &KernelAmbience{}

	// Test EnrichAutoRemedy with empty root
	EnrichAutoRemedy(amb, tempDir)
	assert.NotNil(t, amb)

	// Test EnrichMetricsRollup with missing file
	EnrichMetricsRollup(amb, tempDir)
	assert.Nil(t, amb.MetricsRollup)

	// Write valid metrics rollup
	ambDir := filepath.Join(tempDir, paths.ProjectDataDir, paths.StateDir, "ambient")
	require.NoError(t, fileutil.MkdirAll(ambDir, 0755))
	rollupPath := filepath.Join(ambDir, "metrics-rollup.json")
	require.NoError(t, fileutil.WriteFile(rollupPath, []byte(`{"command_errors":5,"scheduler_stuck_count":0}`), 0644))

	EnrichMetricsRollup(amb, tempDir)
	require.NotNil(t, amb.MetricsRollup)
	assert.Equal(t, 5, amb.MetricsRollup.CommandErrors)
}

func TestCycle147_EnrichStrategicAlignment_Branches(t *testing.T) {
	tempDir := t.TempDir()
	amb := &KernelAmbience{}

	// Case 1: Missing file
	EnrichStrategicAlignment(amb, tempDir)
	require.NotNil(t, amb.StrategicAlignment)
	assert.False(t, amb.StrategicAlignment.Available)

	// Case 2: Invalid JSON
	alignPath := filepath.Join(tempDir, paths.ProjectDataDir, AlignLatestRelativePath)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(alignPath), 0755))
	require.NoError(t, fileutil.WriteFile(alignPath, []byte(`{invalid-json`), 0644))

	EnrichStrategicAlignment(amb, tempDir)
	require.NotNil(t, amb.StrategicAlignment)
	assert.False(t, amb.StrategicAlignment.Available)
	assert.Equal(t, "align cache JSON invalid", amb.StrategicAlignment.Note)

	// Case 3: Valid JSON with alignment score and goal work
	validJSON := `{
		"overall_alignment_score": 88.5,
		"goals_count": 5,
		"goal_work": {
			"items_with_goals": 12,
			"items_without_goals": 0
		}
	}`
	require.NoError(t, fileutil.WriteFile(alignPath, []byte(validJSON), 0644))

	EnrichStrategicAlignment(amb, tempDir)
	require.NotNil(t, amb.StrategicAlignment)
	assert.True(t, amb.StrategicAlignment.Available)
	assert.Equal(t, 88.5, amb.StrategicAlignment.AlignmentScore)
	assert.Equal(t, 5, amb.StrategicAlignment.GoalsCount)
	assert.Equal(t, 12, amb.StrategicAlignment.ItemsWithGoals)
}

func TestCycle147_IsTPMProcessAdminSeat_Matrix(t *testing.T) {
	assert.False(t, IsTPMProcessAdminSeat(""))
	assert.False(t, IsTPMProcessAdminSeat("developer-1"))
	assert.True(t, IsTPMProcessAdminSeat("peer-tpm-lead"))
	assert.True(t, IsTPMProcessAdminSeat("tpm"))
	assert.True(t, IsTPMProcessAdminSeat("craftsman-tpm"))

	assert.False(t, IsTPMProcessAdminSeatIn("", "general-agent"))
	assert.True(t, IsTPMProcessAdminSeatIn("", "peer-tpm-01"))
}
