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

// TestRepoResolverConvergence_StaticFloorInvariant satisfies CRIT-1793554997116874000-464dc586
// and BLI-1793555256226957000-5d0d32ff.
// Verifies Tracking Matrix schema, policy option models, invariant contract validation,
// and default baseline policies.
func TestRepoResolverConvergence_StaticFloorInvariant(t *testing.T) {
	// 1. Verify schema contract
	assert.Equal(t, "zqk_repo_resolver_convergence_v1", community.RepoResolverConvergenceSchema)

	// 2. Verify default convergence policies completeness
	defaultPolicies := community.DefaultConvergencePolicies()
	require.NotEmpty(t, defaultPolicies, "default policies must not be empty")
	assert.GreaterOrEqual(t, len(defaultPolicies), 5, "must provide at least 5 default baseline policies")

	expectedActions := map[string]bool{
		community.PolicyActionAdvanceCycle:       false,
		community.PolicyActionTriageFailures:     false,
		community.PolicyActionRetryFlaky:         false,
		community.PolicyActionQuarantineTest:     false,
		community.PolicyActionOptimizeBottleneck: false,
		community.PolicyActionCacheArtifacts:     false,
	}

	for _, pol := range defaultPolicies {
		require.NoError(t, pol.Validate(), "each default policy must pass validation")
		assert.NotEmpty(t, pol.PolicyID, "policy_id must be declared")
		assert.NotEmpty(t, pol.Name, "policy name must be declared")
		assert.NotEmpty(t, pol.Description, "description must be declared")
		assert.GreaterOrEqual(t, pol.Weight, 0.0, "weight must be non-negative")
		assert.LessOrEqual(t, pol.Weight, 1.0, "weight must be <= 1.0")
		assert.True(t, pol.Enabled, "default policy should be enabled")
		if _, ok := expectedActions[pol.Action]; ok {
			expectedActions[pol.Action] = true
		}
	}

	for action, found := range expectedActions {
		assert.True(t, found, "expected default policy covering action %q", action)
	}

	// 3. Verify session instantiation and default bonding
	session, err := community.NewRepoResolverConvergenceSession("sess-test-static-01", "", nil)
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.Equal(t, "sess-test-static-01", session.SessionID)
	assert.Equal(t, community.RepoResolverConvergenceSchema, session.Schema)
	assert.Equal(t, community.RepoResolverRole, session.PersonaRef)
	assert.Equal(t, community.ConvergenceStatusPending, session.Status)
	assert.Equal(t, len(defaultPolicies), len(session.Policies))
	require.NoError(t, session.Validate())

	// 4. Verify policy registration / replacement
	customPolicy := community.ConvergencePolicyOption{
		PolicyID:       "pol-custom-fast-track",
		Name:           "Fast Track Small Changes",
		Description:    "Quick verification for non-critical code paths",
		Action:         community.PolicyActionAdvanceCycle,
		Impact:         community.PolicyImpactSafe,
		Weight:         0.90,
		Enabled:        true,
		ThresholdScore: 0.99,
	}
	require.NoError(t, session.RegisterPolicy(customPolicy))

	// Verify update to existing policy
	customPolicy.Weight = 0.95
	require.NoError(t, session.RegisterPolicy(customPolicy))

	// 5. Verify diagnostic export summary
	summary := session.ExportSessionSummary()
	require.NotNil(t, summary)
	assert.Equal(t, "sess-test-static-01", summary["session_id"])
	assert.Equal(t, community.RepoResolverConvergenceSchema, summary["schema"])
	assert.Equal(t, community.RepoResolverRole, summary["persona_ref"])
	assert.Equal(t, community.ConvergenceStatusPending, summary["status"])
	assert.Equal(t, len(defaultPolicies)+1, summary["policies_count"])
}

// TestRepoResolverConvergence_OperationalDynamicProof satisfies CRIT-1793555003161085000-3e9b4f69
// and BLI-1793555317425132000-1f436c36.
// Verifies dynamic policy evaluation against build/CI telemetry, candidate ranking,
// failure triage, and pipeline steering decisions.
func TestRepoResolverConvergence_OperationalDynamicProof(t *testing.T) {
	now := time.Now().UTC()

	t.Run("Converged_FullSuccess_AdvanceCycle", func(t *testing.T) {
		session, err := community.NewRepoResolverConvergenceSession("sess-converged-01", "custom-resolver", nil)
		require.NoError(t, err)

		metrics := []community.PipelineStageMetric{
			{JobID: "job-1", StepName: "lint", Outcome: community.OutcomeSuccess, Duration: 100 * time.Millisecond, StartedAt: now, CompletedAt: now.Add(100 * time.Millisecond)},
			{JobID: "job-1", StepName: "build", Outcome: community.OutcomeSuccess, Duration: 250 * time.Millisecond, StartedAt: now.Add(100 * time.Millisecond), CompletedAt: now.Add(350 * time.Millisecond)},
			{JobID: "job-1", StepName: "unit-test", Outcome: community.OutcomeSuccess, Duration: 400 * time.Millisecond, StartedAt: now.Add(350 * time.Millisecond), CompletedAt: now.Add(750 * time.Millisecond)},
			{JobID: "job-2", StepName: "lint", Outcome: community.OutcomeSuccess, Duration: 95 * time.Millisecond, StartedAt: now.Add(1 * time.Second), CompletedAt: now.Add(1095 * time.Millisecond)},
			{JobID: "job-2", StepName: "build", Outcome: community.OutcomeSuccess, Duration: 240 * time.Millisecond, StartedAt: now.Add(1095 * time.Millisecond), CompletedAt: now.Add(1335 * time.Millisecond)},
			{JobID: "job-2", StepName: "unit-test", Outcome: community.OutcomeSuccess, Duration: 380 * time.Millisecond, StartedAt: now.Add(1335 * time.Millisecond), CompletedAt: now.Add(1715 * time.Millisecond)},
		}
		require.NoError(t, session.IngestMetrics(metrics))

		eval, err := session.EvaluateConvergence()
		require.NoError(t, err)
		require.NotNil(t, eval)

		assert.Equal(t, community.ConvergenceStatusConverged, eval.Status)
		assert.Equal(t, 1.0, eval.ConvergenceScore)
		assert.Equal(t, community.PolicyActionAdvanceCycle, eval.RecommendedAction)
		require.NotNil(t, eval.SelectedPolicy)
		assert.Equal(t, "pol-advance-converged", eval.SelectedPolicy.PolicyID)
		assert.Contains(t, eval.Rationale, "Pipeline converged")
		assert.Empty(t, eval.TargetSteps, "no failing steps on full convergence")

		// Verify Tracking Matrix state was populated on session
		require.NotNil(t, session.TrackingMatrix)
		assert.Equal(t, 6, session.TrackingMatrix.TotalSteps)
		assert.Equal(t, 6, session.TrackingMatrix.SuccessCount)
		assert.Equal(t, 0, session.TrackingMatrix.FailureCount)
	})

	t.Run("FlakyStage_RetryRecommendation", func(t *testing.T) {
		session, err := community.NewRepoResolverConvergenceSession("sess-flaky-01", "", nil)
		require.NoError(t, err)

		// unit-test has mixed results across jobs (1 success, 1 failure)
		metrics := []community.PipelineStageMetric{
			{JobID: "job-1", StepName: "build", Outcome: community.OutcomeSuccess, Duration: 200 * time.Millisecond},
			{JobID: "job-1", StepName: "unit-test", Outcome: community.OutcomeSuccess, Duration: 300 * time.Millisecond},
			{JobID: "job-2", StepName: "build", Outcome: community.OutcomeSuccess, Duration: 210 * time.Millisecond},
			{JobID: "job-2", StepName: "unit-test", Outcome: community.OutcomeFailed, Duration: 310 * time.Millisecond, ErrorMessage: "flaky test flake"},
			{JobID: "job-3", StepName: "build", Outcome: community.OutcomeSuccess, Duration: 195 * time.Millisecond},
			{JobID: "job-3", StepName: "unit-test", Outcome: community.OutcomeSuccess, Duration: 290 * time.Millisecond},
		}
		require.NoError(t, session.IngestMetrics(metrics))

		eval, err := session.EvaluateConvergence()
		require.NoError(t, err)
		require.NotNil(t, eval)

		assert.Equal(t, community.PolicyActionRetryFlaky, eval.RecommendedAction)
		assert.Equal(t, community.ConvergenceStatusDegraded, eval.Status)
		require.NotNil(t, eval.SelectedPolicy)
		assert.Equal(t, "pol-retry-flaky-stages", eval.SelectedPolicy.PolicyID)
		assert.Contains(t, eval.Rationale, "flaky")
		assert.Contains(t, eval.TargetSteps, "unit-test")
	})

	t.Run("PersistentFailure_QuarantineOrTriage", func(t *testing.T) {
		session, err := community.NewRepoResolverConvergenceSession("sess-fail-01", "", nil)
		require.NoError(t, err)

		// unit-test consistently fails across all jobs
		metrics := []community.PipelineStageMetric{
			{JobID: "job-1", StepName: "lint", Outcome: community.OutcomeSuccess, Duration: 100 * time.Millisecond},
			{JobID: "job-1", StepName: "build", Outcome: community.OutcomeSuccess, Duration: 200 * time.Millisecond},
			{JobID: "job-1", StepName: "unit-test", Outcome: community.OutcomeFailed, Duration: 400 * time.Millisecond, ErrorMessage: "panic: nil pointer"},
			{JobID: "job-2", StepName: "lint", Outcome: community.OutcomeSuccess, Duration: 105 * time.Millisecond},
			{JobID: "job-2", StepName: "build", Outcome: community.OutcomeSuccess, Duration: 205 * time.Millisecond},
			{JobID: "job-2", StepName: "unit-test", Outcome: community.OutcomeFailed, Duration: 395 * time.Millisecond, ErrorMessage: "panic: nil pointer"},
		}
		require.NoError(t, session.IngestMetrics(metrics))

		eval, err := session.EvaluateConvergence()
		require.NoError(t, err)
		require.NotNil(t, eval)

		assert.Equal(t, community.ConvergenceStatusActionRequired, eval.Status)
		assert.Contains(t, []string{community.PolicyActionQuarantineTest, community.PolicyActionTriageFailures}, eval.RecommendedAction)
		assert.Contains(t, eval.TargetSteps, "unit-test")
	})

	t.Run("LatencyBottleneck_OptimizeRecommendation", func(t *testing.T) {
		session, err := community.NewRepoResolverConvergenceSession("sess-slow-01", "", nil)
		require.NoError(t, err)

		// Stage duration exceeds 10s budget
		metrics := []community.PipelineStageMetric{
			{JobID: "job-1", StepName: "lint", Outcome: community.OutcomeSuccess, Duration: 200 * time.Millisecond},
			{JobID: "job-1", StepName: "build", Outcome: community.OutcomeSuccess, Duration: 15 * time.Second},
			{JobID: "job-1", StepName: "unit-test", Outcome: community.OutcomeSuccess, Duration: 500 * time.Millisecond},
		}
		require.NoError(t, session.IngestMetrics(metrics))

		eval, err := session.EvaluateConvergence()
		require.NoError(t, err)
		require.NotNil(t, eval)

		assert.Contains(t, []string{community.PolicyActionOptimizeBottleneck, community.PolicyActionCacheArtifacts}, eval.RecommendedAction)
		assert.Contains(t, eval.Bottlenecks, "build")
	})

	t.Run("SteerPipelineConvergence_StatePersistence", func(t *testing.T) {
		session, err := community.NewRepoResolverConvergenceSession("sess-steer-01", "", nil)
		require.NoError(t, err)

		require.NoError(t, session.IngestMetric(community.PipelineStageMetric{
			JobID: "job-1", StepName: "smoke", Outcome: community.OutcomeSuccess, Duration: 50 * time.Millisecond,
		}))

		eval, err := session.SteerPipelineConvergence()
		require.NoError(t, err)
		assert.Equal(t, community.PolicyActionAdvanceCycle, eval.RecommendedAction)
		assert.Equal(t, community.ConvergenceStatusConverged, session.Status)
		assert.Len(t, session.Evaluations, 1)

		summary := session.ExportSessionSummary()
		assert.Equal(t, community.PolicyActionAdvanceCycle, summary["latest_action"])
	})

	t.Run("ConcurrentIngestion_ThreadSafety", func(t *testing.T) {
		session, err := community.NewRepoResolverConvergenceSession("sess-concurrent-01", "", nil)
		require.NoError(t, err)

		workers := 8
		itemsPerWorker := 30
		var wg sync.WaitGroup

		for w := 0; w < workers; w++ {
			wg.Add(1)
			wID := w
			goroutinelabels.NewGoroutine("convergence_eval_worker", fmt.Sprintf("worker-%d", wID)).StartSimple(func() {
				defer wg.Done()
				for i := 0; i < itemsPerWorker; i++ {
					_ = session.IngestMetric(community.PipelineStageMetric{
						JobID:    fmt.Sprintf("job-c%d", wID),
						StepName: "test-step",
						Outcome:  community.OutcomeSuccess,
						Duration: time.Duration(i+1) * time.Millisecond,
					})
				}
			})
		}
		wg.Wait()

		eval, err := session.EvaluateConvergence()
		require.NoError(t, err)
		assert.Equal(t, 1.0, eval.ConvergenceScore)
		assert.Equal(t, workers*itemsPerWorker, session.TrackingMatrix.TotalSteps)
	})
}

// TestRepoResolverConvergence_NegativeAdversarialBoundary satisfies CRIT-1793555010436591000-49fb1ea9
// and BLI-1793555385734701000-8cd1b26a.
// Verifies fail-safe handling against missing telemetry, malformed configs, corrupted payloads,
// invalid policy options, and nil receiver defenses with zero panics.
func TestRepoResolverConvergence_NegativeAdversarialBoundary(t *testing.T) {
	// 1. Missing / Empty telemetry returns safe pending status
	t.Run("EmptyTelemetry_GracefulDegradation", func(t *testing.T) {
		session, err := community.NewRepoResolverConvergenceSession("sess-empty-01", "", nil)
		require.NoError(t, err)

		eval, err := session.EvaluateConvergence()
		require.NoError(t, err)
		require.NotNil(t, eval)

		assert.Equal(t, community.ConvergenceStatusPending, eval.Status)
		assert.Equal(t, 0.0, eval.ConvergenceScore)
		assert.Equal(t, community.PolicyActionNone, eval.RecommendedAction)
		assert.Contains(t, eval.Rationale, "No pipeline telemetry")
	})

	// 2. Corrupted and malformed session bytes
	t.Run("CorruptedSessionBytes_Rejection", func(t *testing.T) {
		_, err := community.ParseConvergenceSessionBytes(nil)
		assert.ErrorIs(t, err, community.ErrMalformedPayload)

		_, err = community.ParseConvergenceSessionBytes([]byte(""))
		assert.ErrorIs(t, err, community.ErrMalformedPayload)

		_, err = community.ParseConvergenceSessionBytes([]byte("{corrupt-json-unclosed"))
		assert.ErrorIs(t, err, community.ErrMalformedPayload)

		// Valid JSON but missing session_id
		_, err = community.ParseConvergenceSessionBytes([]byte(`{"status":"pending"}`))
		assert.ErrorIs(t, err, community.ErrInvalidSessionID)
	})

	// 3. Oversized session payloads (> 256KB)
	t.Run("OversizedPayload_Rejection", func(t *testing.T) {
		hugeBytes := make([]byte, community.DefaultMaxMetricPayloadBytes+512)
		for i := range hugeBytes {
			hugeBytes[i] = ' '
		}
		_, err := community.ParseConvergenceSessionBytes(hugeBytes)
		assert.ErrorIs(t, err, community.ErrPayloadTooLarge)
	})

	// 4. Invalid policy option configurations
	t.Run("InvalidPolicyOptions_Rejection", func(t *testing.T) {
		// Empty policy ID
		p1 := community.ConvergencePolicyOption{Name: "Name", Action: community.PolicyActionNone, Weight: 0.5}
		assert.ErrorIs(t, p1.Validate(), community.ErrInvalidPolicyConfig)

		// Empty name
		p2 := community.ConvergencePolicyOption{PolicyID: "pol-1", Action: community.PolicyActionNone, Weight: 0.5}
		assert.ErrorIs(t, p2.Validate(), community.ErrInvalidPolicyConfig)

		// Negative weight
		p3 := community.ConvergencePolicyOption{PolicyID: "pol-1", Name: "Name", Weight: -0.1}
		assert.ErrorIs(t, p3.Validate(), community.ErrInvalidPolicyConfig)

		// Weight > 1.0
		p4 := community.ConvergencePolicyOption{PolicyID: "pol-1", Name: "Name", Weight: 1.5}
		assert.ErrorIs(t, p4.Validate(), community.ErrInvalidPolicyConfig)

		// Invalid action
		p5 := community.ConvergencePolicyOption{PolicyID: "pol-1", Name: "Name", Action: "explode_cluster", Weight: 0.5}
		assert.ErrorIs(t, p5.Validate(), community.ErrInvalidPolicyConfig)

		// Invalid impact
		p6 := community.ConvergencePolicyOption{PolicyID: "pol-1", Name: "Name", Impact: "catastrophic", Weight: 0.5}
		assert.ErrorIs(t, p6.Validate(), community.ErrInvalidPolicyConfig)

		// Register invalid policy into session
		session, err := community.NewRepoResolverConvergenceSession("sess-invalid-pol", "", nil)
		require.NoError(t, err)
		assert.Error(t, session.RegisterPolicy(p3))
	})

	// 5. Missing / Empty session ID on construction
	t.Run("EmptySessionID_ConstructionRejection", func(t *testing.T) {
		_, err := community.NewRepoResolverConvergenceSession("", "", nil)
		assert.ErrorIs(t, err, community.ErrInvalidSessionID)

		_, err = community.NewRepoResolverConvergenceSession("   ", "", nil)
		assert.ErrorIs(t, err, community.ErrInvalidSessionID)
	})

	// 6. Nil receiver safety (zero panics)
	t.Run("NilReceiver_ZeroPanics", func(t *testing.T) {
		var nilSession *community.RepoResolverConvergenceSession

		assert.NotPanics(t, func() {
			assert.ErrorIs(t, nilSession.Validate(), community.ErrNilConvergenceSession)
			assert.ErrorIs(t, nilSession.RegisterPolicy(community.ConvergencePolicyOption{}), community.ErrNilConvergenceSession)
			assert.ErrorIs(t, nilSession.IngestMetric(community.PipelineStageMetric{}), community.ErrNilConvergenceSession)
			assert.ErrorIs(t, nilSession.IngestMetrics(nil), community.ErrNilConvergenceSession)

			eval, err := nilSession.EvaluateConvergence()
			assert.ErrorIs(t, err, community.ErrNilConvergenceSession)
			assert.Nil(t, eval)

			steerEval, err := nilSession.SteerPipelineConvergence()
			assert.ErrorIs(t, err, community.ErrNilConvergenceSession)
			assert.Nil(t, steerEval)

			summary := nilSession.ExportSessionSummary()
			assert.NotNil(t, summary)
			assert.Equal(t, "nil_session", summary["status"])
		})
	})

	// 7. Safe JSON unmarshaling into RepoResolverConvergenceSession
	t.Run("ValidJSON_ParseSuccess", func(t *testing.T) {
		validSession := &community.RepoResolverConvergenceSession{
			SessionID:  "sess-json-01",
			Schema:     community.RepoResolverConvergenceSchema,
			PersonaRef: community.RepoResolverRole,
			Status:     community.ConvergenceStatusPending,
		}
		raw, err := json.Marshal(validSession)
		require.NoError(t, err)

		parsed, err := community.ParseConvergenceSessionBytes(raw)
		require.NoError(t, err)
		assert.Equal(t, "sess-json-01", parsed.SessionID)
		assert.Equal(t, community.RepoResolverConvergenceSchema, parsed.Schema)
	})
}
