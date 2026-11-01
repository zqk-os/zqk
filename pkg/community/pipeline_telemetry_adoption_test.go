package community_test

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const testRepoResolverPersonaID = "PER-1793541871378023000-273387ff"

// TestPipelineTelemetry_StaticFloorInvariant satisfies CRIT-1793541695675095000-417479fe.
// Verifies that the repo_resolver persona (PER-1793541871378023000-273387ff) is registered,
// bound to canonical skills, has required role definitions, and satisfies CAS index contracts.
func TestPipelineTelemetry_StaticFloorInvariant(t *testing.T) {
	repoRoot := findRepoRootForSDK(t)

	// 1. Verify persona exists in CAS and loads cleanly
	persona, err := community.LoadPersonaFromCAS(repoRoot, testRepoResolverPersonaID)
	require.NoError(t, err, "repo_resolver persona must exist and load from CAS index")
	require.NotNil(t, persona)

	// 2. Invariant assertions on persona fields
	assert.Equal(t, testRepoResolverPersonaID, persona.ID)
	assert.Equal(t, "persona", persona.Kind)
	assert.Equal(t, community.RepoResolverRole, persona.Role)
	assert.Equal(t, "approved", persona.Status)
	assert.Contains(t, persona.Title, "Repo-Resolver")
	assert.NotEmpty(t, persona.Description, "persona must declare non-empty description")
	assert.Contains(t, persona.Description, "convergence", "description must document convergence tracking")

	// 3. Verify bound skill references
	require.NotEmpty(t, persona.AgentSkillRefs, "must declare at least one bound agent skill")
	assert.Contains(t, persona.AgentSkillRefs, community.RepoResolverSkillRef, "must bind feed correspondence skill")
}

// TestPipelineTelemetry_OperationalDynamicProof satisfies CRIT-1793541695675096000-c62d5610.
// Verifies recording and tracking pipeline metrics (job duration, step outcomes, throughput)
// into the convergence tracking matrix with correct aggregations and export formatting.
func TestPipelineTelemetry_OperationalDynamicProof(t *testing.T) {
	engine := community.NewPipelineTelemetryEngine(500)
	require.NotNil(t, engine)

	now := time.Now().UTC()

	// 1. Record series of pipeline stages across multiple jobs
	stages := []community.PipelineStageMetric{
		{
			PipelineID:      "pipe-100",
			JobID:           "job-1",
			StepName:        "lint",
			Outcome:         community.OutcomeSuccess,
			Duration:        120 * time.Millisecond,
			StartedAt:       now,
			CompletedAt:     now.Add(120 * time.Millisecond),
			ThroughputItems: 42,
		},
		{
			PipelineID:      "pipe-100",
			JobID:           "job-1",
			StepName:        "build",
			Outcome:         community.OutcomeSuccess,
			Duration:        350 * time.Millisecond,
			StartedAt:       now.Add(120 * time.Millisecond),
			CompletedAt:     now.Add(470 * time.Millisecond),
			ThroughputItems: 1,
		},
		{
			PipelineID:      "pipe-100",
			JobID:           "job-1",
			StepName:        "unit-test",
			Outcome:         community.OutcomeFailed,
			Duration:        800 * time.Millisecond,
			StartedAt:       now.Add(470 * time.Millisecond),
			CompletedAt:     now.Add(1270 * time.Millisecond),
			ThroughputItems: 150,
			ErrorMessage:    "TestFoo failed: unexpected nil pointer",
		},
		{
			PipelineID:      "pipe-101",
			JobID:           "job-2",
			StepName:        "lint",
			Outcome:         community.OutcomeSuccess,
			Duration:        100 * time.Millisecond,
			StartedAt:       now.Add(2 * time.Second),
			CompletedAt:     now.Add(2100 * time.Millisecond),
			ThroughputItems: 42,
		},
		{
			PipelineID:      "pipe-101",
			JobID:           "job-2",
			StepName:        "build",
			Outcome:         community.OutcomeSuccess,
			Duration:        300 * time.Millisecond,
			StartedAt:       now.Add(2100 * time.Millisecond),
			CompletedAt:     now.Add(2400 * time.Millisecond),
			ThroughputItems: 1,
		},
		{
			PipelineID:      "pipe-101",
			JobID:           "job-2",
			StepName:        "unit-test",
			Outcome:         community.OutcomeSuccess,
			Duration:        750 * time.Millisecond,
			StartedAt:       now.Add(2400 * time.Millisecond),
			CompletedAt:     now.Add(3150 * time.Millisecond),
			ThroughputItems: 150,
		},
	}

	for _, stage := range stages {
		err := engine.RecordStage(stage)
		require.NoError(t, err)
	}

	assert.Equal(t, 6, engine.Count())

	// 2. Validate Convergence Matrix Calculations
	matrix := engine.GetConvergenceMatrix()
	require.NotNil(t, matrix)

	assert.Equal(t, 2, matrix.TotalJobs, "must aggregate across 2 distinct jobs")
	assert.Equal(t, 6, matrix.TotalSteps, "must count 6 total executed steps")
	assert.Equal(t, 5, matrix.SuccessCount, "must count 5 successful steps")
	assert.Equal(t, 1, matrix.FailureCount, "must count 1 failed step")
	assert.Equal(t, 386, matrix.ThroughputItems, "must sum total throughput items")
	assert.InDelta(t, 5.0/6.0, matrix.ConvergenceScore, 0.001, "convergence score must match success ratio")
	assert.Greater(t, matrix.TotalDuration, time.Duration(0))
	assert.Greater(t, matrix.AverageDuration, time.Duration(0))
	assert.Greater(t, matrix.P95Duration, time.Duration(0))

	// Verify step breakdowns
	require.Contains(t, matrix.StepBreakdown, "lint")
	require.Contains(t, matrix.StepBreakdown, "build")
	require.Contains(t, matrix.StepBreakdown, "unit-test")

	lintMetrics := matrix.StepBreakdown["lint"]
	assert.Equal(t, 2, lintMetrics.TotalExecutions)
	assert.Equal(t, 2, lintMetrics.SuccessCount)
	assert.Equal(t, 0, lintMetrics.FailureCount)
	assert.Equal(t, 110*time.Millisecond, lintMetrics.AverageDuration)

	unitTestMetrics := matrix.StepBreakdown["unit-test"]
	assert.Equal(t, 2, unitTestMetrics.TotalExecutions)
	assert.Equal(t, 1, unitTestMetrics.SuccessCount)
	assert.Equal(t, 1, unitTestMetrics.FailureCount)

	// 3. Verify Community MetricEvent Export
	events := engine.ExportMetricEvents("cluster-hash-abc")
	require.Len(t, events, 6)
	assert.Equal(t, "pipeline", events[0].Category)
	assert.Equal(t, "lint", events[0].Action)
	assert.Equal(t, "cluster-hash-abc", events[0].ClusterHash)
	assert.Equal(t, "ci-pipeline", events[0].ExecutionMode)
	assert.Equal(t, "job-1", events[0].Attributes["job_id"])

	// 4. Verify Concurrent Execution Safety
	concurrencyEngine := community.NewPipelineTelemetryEngine(1000)
	var wg sync.WaitGroup
	workers := 10
	itemsPerWorker := 25

	for w := 0; w < workers; w++ {
		wg.Add(1)
		workerID := w
		goroutinelabels.NewGoroutine("pipeline_telemetry_worker", fmt.Sprintf("worker-%d", workerID)).StartSimple(func() {
			defer wg.Done()
			for i := 0; i < itemsPerWorker; i++ {
				_ = concurrencyEngine.RecordStage(community.PipelineStageMetric{
					PipelineID:      fmt.Sprintf("pipe-%d", workerID),
					JobID:           fmt.Sprintf("job-w%d", workerID),
					StepName:        "bench",
					Outcome:         community.OutcomeSuccess,
					Duration:        time.Duration(i+1) * time.Millisecond,
					ThroughputItems: 10,
				})
			}
		})
	}

	wg.Wait()
	assert.Equal(t, workers*itemsPerWorker, concurrencyEngine.Count())
	concurrentMatrix := concurrencyEngine.GetConvergenceMatrix()
	assert.Equal(t, workers*itemsPerWorker, concurrentMatrix.TotalSteps)
	assert.Equal(t, 1.0, concurrentMatrix.ConvergenceScore)
}

// TestPipelineTelemetry_NegativeAdversarialBoundary satisfies CRIT-1793541695675097000-299ab64e.
// Verifies strict fail-closed safety, corruption rejection, and bounded memory behavior
// against malformed, out-of-order, or excessively large telemetry payloads.
func TestPipelineTelemetry_NegativeAdversarialBoundary(t *testing.T) {
	engine := community.NewPipelineTelemetryEngine(10)
	require.NotNil(t, engine)

	// 1. Rejection of empty or malformed payload bytes
	_, err := engine.RecordPayload(nil)
	assert.ErrorIs(t, err, community.ErrMalformedPayload)

	_, err = engine.RecordPayload([]byte(""))
	assert.ErrorIs(t, err, community.ErrMalformedPayload)

	_, err = engine.RecordPayload([]byte("{corrupt-json-no-closing"))
	assert.ErrorIs(t, err, community.ErrMalformedPayload)

	// 2. Rejection of oversized payloads (> 256KB)
	hugePayload := make([]byte, community.DefaultMaxMetricPayloadBytes+1024)
	for i := range hugePayload {
		hugePayload[i] = 'a'
	}
	_, err = engine.RecordPayload(hugePayload)
	assert.ErrorIs(t, err, community.ErrPayloadTooLarge)

	// 3. Invariant: Missing required fields (job_id or step_name)
	missingJob := community.PipelineStageMetric{
		StepName: "build",
		Duration: 100 * time.Millisecond,
	}
	assert.ErrorIs(t, engine.RecordStage(missingJob), community.ErrMissingRequiredField)

	missingStep := community.PipelineStageMetric{
		JobID:    "job-123",
		Duration: 100 * time.Millisecond,
	}
	assert.ErrorIs(t, engine.RecordStage(missingStep), community.ErrMissingRequiredField)

	// 4. Invariant: Negative duration
	negativeDuration := community.PipelineStageMetric{
		JobID:    "job-123",
		StepName: "test",
		Duration: -5 * time.Second,
	}
	assert.ErrorIs(t, engine.RecordStage(negativeDuration), community.ErrInvalidDuration)

	// 5. Invariant: Inverted time sequence (CompletedAt before StartedAt)
	tNow := time.Now().UTC()
	invertedTimes := community.PipelineStageMetric{
		JobID:       "job-123",
		StepName:    "compile",
		StartedAt:   tNow,
		CompletedAt: tNow.Add(-10 * time.Second),
	}
	assert.ErrorIs(t, engine.RecordStage(invertedTimes), community.ErrInvalidTimeSequence)

	// 6. Unknown outcome string
	invalidOutcome := community.PipelineStageMetric{
		JobID:    "job-123",
		StepName: "deploy",
		Outcome:  "catastrophic_meltdown",
	}
	assert.ErrorIs(t, engine.RecordStage(invalidOutcome), community.ErrMalformedPayload)

	// 7. Nil engine safety (zero panics)
	var nilEngine *community.PipelineTelemetryEngine
	assert.NotPanics(t, func() {
		assert.Error(t, nilEngine.RecordStage(community.PipelineStageMetric{JobID: "j", StepName: "s"}))
		_, err := nilEngine.RecordPayload([]byte(`{"job_id":"j","step_name":"s"}`))
		assert.Error(t, err)
		assert.Equal(t, 0, nilEngine.Count())
		matrix := nilEngine.GetConvergenceMatrix()
		assert.NotNil(t, matrix)
		events := nilEngine.ExportMetricEvents("test")
		assert.Nil(t, events)
	})

	// 8. Bounded ring buffer: verify engine never exceeds max capacity
	smallEngine := community.NewPipelineTelemetryEngine(3)
	for i := 1; i <= 10; i++ {
		validJSON, _ := json.Marshal(community.PipelineStageMetric{
			JobID:    fmt.Sprintf("job-%d", i),
			StepName: "step",
			Duration: time.Duration(i) * time.Millisecond,
		})
		_, err := smallEngine.RecordPayload(validJSON)
		require.NoError(t, err)
	}
	assert.Equal(t, 3, smallEngine.Count(), "engine capacity must remain strictly bounded at 3")
}
