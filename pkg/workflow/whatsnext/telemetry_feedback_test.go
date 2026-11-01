package whatsnext_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/workflow/whatsnext"
)

type mockStorageProvider struct {
	storage.ObjectStorageProvider
}

func (m *mockStorageProvider) List(ctx context.Context, sec *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	return &storage.QueryResult{Objects: nil}, nil
}

func (m *mockStorageProvider) Read(ctx context.Context, sec *pkgctx.SecurityContext, id string) (map[string]any, error) {
	return nil, nil
}

// TestPipelineTelemetry_StaticFloorInvariant satisfies CRIT-1793547039693804000-8b60a909.
// Verifies data structures, schema contract constants, JSON tagging, and invariant validation
// for whats-next pipeline telemetry feedback.
func TestPipelineTelemetry_StaticFloorInvariant(t *testing.T) {
	// 1. Verify schema and file constants
	assert.Equal(t, "zqk_pipeline_telemetry_feedback_v1", whatsnext.PipelineTelemetrySchema)
	assert.Equal(t, "pipeline_telemetry.json", whatsnext.PipelineTelemetryFileName)

	// 2. Verify failure category constants
	assert.Equal(t, "none", whatsnext.FailureCategoryNone)
	assert.Equal(t, "test_failure", whatsnext.FailureCategoryTest)
	assert.Equal(t, "lint_failure", whatsnext.FailureCategoryLint)
	assert.Equal(t, "build_failure", whatsnext.FailureCategoryBuild)
	assert.Equal(t, "timeout", whatsnext.FailureCategoryTimeout)
	assert.Equal(t, "payload_gate_failure", whatsnext.FailureCategoryPayloadGate)

	// 3. Verify recommended action constants
	assert.Equal(t, "none", whatsnext.ActionNone)
	assert.Equal(t, "advance_execution_cycle", whatsnext.ActionAdvanceCycle)
	assert.Equal(t, "triage_pipeline_failures", whatsnext.ActionTriageFailures)
	assert.Equal(t, "remediate_flaky_stages", whatsnext.ActionRemediateFlakes)
	assert.Equal(t, "optimize_stage_duration", whatsnext.ActionOptimizeDuration)

	// 4. Verify data structure serialization and deserialization
	summary := whatsnext.PipelineTelemetrySummary{
		Schema:              whatsnext.PipelineTelemetrySchema,
		RunID:               "run-ci-101",
		PipelineID:          "pipe-main",
		Status:              whatsnext.StatusPassed,
		Duration:            45 * time.Second,
		DurationMs:          45000,
		FailureCategory:     whatsnext.FailureCategoryNone,
		FailureDetails:      []string{"all suites green"},
		RecommendedAction:   whatsnext.ActionAdvanceCycle,
		TotalJobs:           5,
		TotalSteps:          18,
		FailedSteps:         0,
		ThroughputPerMinute: 24.0,
		ConvergenceScore:    1.0,
		Timestamp:           time.Date(2026, 11, 1, 15, 0, 0, 0, time.UTC),
	}

	bytes, err := json.Marshal(summary)
	require.NoError(t, err)

	parsed, err := whatsnext.ParsePipelineTelemetryBytes(bytes)
	require.NoError(t, err)
	require.NotNil(t, parsed)

	assert.Equal(t, summary.RunID, parsed.RunID)
	assert.Equal(t, summary.Status, parsed.Status)
	assert.Equal(t, summary.Duration, parsed.Duration)
	assert.Equal(t, summary.ConvergenceScore, parsed.ConvergenceScore)
	assert.Equal(t, summary.RecommendedAction, parsed.RecommendedAction)

	// 5. Invariant validation
	require.NoError(t, summary.Validate())
}

// TestPipelineTelemetry_OperationalDynamicProof satisfies CRIT-1793547042655018000-30a79ec6.
// Verifies dynamic ingestion, loading, convergence cue generation, and seamless integration
// with whats-next Execute workflow.
func TestPipelineTelemetry_OperationalDynamicProof(t *testing.T) {
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		whatsnext.WaitForReconcile(5 * time.Second)
	})

	// 1. Ingest passing pipeline summary
	passSummary := whatsnext.PipelineTelemetrySummary{
		RunID:             "ci-build-867",
		PipelineID:        "pipe-master",
		Status:            whatsnext.StatusPassed,
		Duration:          2 * time.Minute,
		FailureCategory:   whatsnext.FailureCategoryNone,
		RecommendedAction: whatsnext.ActionAdvanceCycle,
		TotalJobs:         4,
		TotalSteps:        12,
		FailedSteps:       0,
	}

	err := whatsnext.IngestPipelineTelemetry(tmpDir, &passSummary)
	require.NoError(t, err)

	// 2. Load and verify ingested telemetry from project state directory
	loaded, err := whatsnext.LoadPipelineTelemetrySummary(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, loaded)
	assert.Equal(t, "ci-build-867", loaded.RunID)
	assert.Equal(t, whatsnext.PipelineTelemetrySchema, loaded.Schema)
	assert.Equal(t, int64(120000), loaded.DurationMs)

	// 3. Verify convergence cue for passing run ready to advance cycle
	passCue := whatsnext.EvaluatePipelineConvergenceCue(loaded)
	assert.Contains(t, passCue, "Pipeline healthy")
	assert.Contains(t, passCue, "advance execution cycle")

	// 4. Ingest failing pipeline telemetry and evaluate alert cue
	failSummary := whatsnext.PipelineTelemetrySummary{
		RunID:             "ci-build-868",
		PipelineID:        "pipe-master",
		Status:            whatsnext.StatusFailed,
		Duration:          3 * time.Minute,
		FailureCategory:   whatsnext.FailureCategoryTest,
		FailureDetails:    []string{"TestFoo timed out", "panic in TestBar"},
		RecommendedAction: whatsnext.ActionTriageFailures,
		TotalJobs:         4,
		TotalSteps:        12,
		FailedSteps:       2,
	}

	err = whatsnext.IngestPipelineTelemetry(tmpDir, &failSummary)
	require.NoError(t, err)

	loadedFail, err := whatsnext.LoadPipelineTelemetrySummary(tmpDir)
	require.NoError(t, err)
	require.NotNil(t, loadedFail)

	failCue := whatsnext.EvaluatePipelineConvergenceCue(loadedFail)
	assert.Contains(t, failCue, "Pipeline alert")
	assert.Contains(t, failCue, whatsnext.FailureCategoryTest)
	assert.Contains(t, failCue, "run_id=ci-build-868")
	assert.Contains(t, failCue, "failed_steps=2")
	assert.Contains(t, failCue, whatsnext.ActionTriageFailures)

	// 5. Ingest degraded pipeline telemetry and evaluate optimization cue
	degradedSummary := whatsnext.PipelineTelemetrySummary{
		RunID:             "ci-build-869",
		PipelineID:        "pipe-master",
		Status:            whatsnext.StatusDegraded,
		Duration:          25 * time.Minute,
		RecommendedAction: whatsnext.ActionOptimizeDuration,
	}

	degradedCue := whatsnext.EvaluatePipelineConvergenceCue(&degradedSummary)
	assert.Contains(t, degradedCue, "Pipeline degraded")
	assert.Contains(t, degradedCue, whatsnext.ActionOptimizeDuration)

	// 6. End-to-end integration test with Execute()
	sp := &mockStorageProvider{}

	out, err := whatsnext.Execute(context.Background(), sp, tmpDir, &whatsnext.QueryRequest{
		SkipMeasure:       true,
		SkipPersonaFilter: true,
	})
	require.NoError(t, err)
	require.NotNil(t, out)

	// Verify WhatsNextOut carries the ingested pipeline telemetry and convergence cue
	assert.NotNil(t, out.PipelineFeedback)
	assert.Equal(t, "ci-build-868", out.PipelineFeedback.RunID)
	assert.Equal(t, failCue, out.PipelineConvergenceCue)
}

// TestPipelineTelemetry_NegativeAdversarialBoundary satisfies CRIT-1793547045381204000-bd4be113.
// Verifies graceful degradation, zero-panic guarantee, and corruption rejection when telemetry
// files are absent, zero-byte, corrupted, or have out-of-bound inputs.
func TestPipelineTelemetry_NegativeAdversarialBoundary(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Missing telemetry file on disk: returns nil, nil without error or panic
	missing, err := whatsnext.LoadPipelineTelemetrySummary(tmpDir)
	assert.NoError(t, err)
	assert.Nil(t, missing)

	// 2. Empty projectRoot: returns nil, nil
	emptyRoot, err := whatsnext.LoadPipelineTelemetrySummary("")
	assert.NoError(t, err)
	assert.Nil(t, emptyRoot)

	// 3. Zero-byte telemetry file: returns nil, nil gracefully
	stateDir := paths.StateDirPath(tmpDir)
	require.NoError(t, fileutil.MkdirAll(stateDir, paths.DirPerm755))
	telemetryFile := filepath.Join(stateDir, whatsnext.PipelineTelemetryFileName)

	require.NoError(t, fileutil.WriteFile(telemetryFile, []byte(""), paths.FilePerm644))
	zeroByte, err := whatsnext.LoadPipelineTelemetrySummary(tmpDir)
	assert.NoError(t, err)
	assert.Nil(t, zeroByte)

	// 4. Corrupted JSON file: returns ErrMalformedTelemetryFeedback without panic
	require.NoError(t, fileutil.WriteFile(telemetryFile, []byte("{invalid-json-content: true"), paths.FilePerm644))
	corrupt, err := whatsnext.LoadPipelineTelemetrySummary(tmpDir)
	assert.ErrorIs(t, err, whatsnext.ErrMalformedTelemetryFeedback)
	assert.Nil(t, corrupt)

	// 5. Negative duration validation
	negativeDuration := whatsnext.PipelineTelemetrySummary{
		RunID:    "run-neg",
		Duration: -10 * time.Second,
	}
	assert.ErrorIs(t, negativeDuration.Validate(), whatsnext.ErrNegativeDuration)

	err = whatsnext.IngestPipelineTelemetry(tmpDir, &negativeDuration)
	assert.ErrorIs(t, err, whatsnext.ErrNegativeDuration)

	// 6. Ingest nil summary or empty root safety
	assert.Error(t, whatsnext.IngestPipelineTelemetry("", &whatsnext.PipelineTelemetrySummary{}))
	assert.Error(t, whatsnext.IngestPipelineTelemetry(tmpDir, nil))

	// 7. Evaluate cue with nil summary: returns empty string safely
	assert.Equal(t, "", whatsnext.EvaluatePipelineConvergenceCue(nil))

	// 8. Zero panic verification on all edge cases
	assert.NotPanics(t, func() {
		_ = whatsnext.EvaluatePipelineConvergenceCue(&whatsnext.PipelineTelemetrySummary{})
		_, _ = whatsnext.ParsePipelineTelemetryBytes(nil)
		_, _ = whatsnext.ParsePipelineTelemetryBytes([]byte("   \n\t  "))
		_, _ = whatsnext.ParsePipelineTelemetryBytes([]byte("not valid json"))
		var nilSummary *whatsnext.PipelineTelemetrySummary
		_ = nilSummary.Validate()
	})

	// 9. Clean up test file
	_ = os.Remove(telemetryFile)
}
