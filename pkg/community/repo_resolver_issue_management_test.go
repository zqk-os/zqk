package community_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/community"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

// TestRepoResolverIssueManagement_StaticFloorInvariant satisfies CRIT-1793769968219368000-9b6882e3
// and BLI-1793769968219368500-93acba54.
// Verifies validation rules, bounds checks, nil-receiver safety, and invariant enforcement.
func TestRepoResolverIssueManagement_StaticFloorInvariant(t *testing.T) {
	// 1. GitHubIssue validation
	t.Run("GitHubIssue_Validation", func(t *testing.T) {
		// Nil issue
		var nilIssue *community.GitHubIssue
		assert.ErrorIs(t, nilIssue.Validate(), community.ErrNilGitHubIssue)

		// Zero and negative issue numbers
		zeroIssue := &community.GitHubIssue{IssueNumber: 0, Title: "Fix bug", State: community.GitHubIssueStateOpen}
		assert.ErrorIs(t, zeroIssue.Validate(), community.ErrInvalidIssueNumber)

		negIssue := &community.GitHubIssue{IssueNumber: -5, Title: "Fix bug", State: community.GitHubIssueStateOpen}
		assert.ErrorIs(t, negIssue.Validate(), community.ErrInvalidIssueNumber)

		// Empty and whitespace-only title
		emptyTitleIssue := &community.GitHubIssue{IssueNumber: 1, Title: "", State: community.GitHubIssueStateOpen}
		assert.ErrorIs(t, emptyTitleIssue.Validate(), community.ErrEmptyIssueTitle)

		wsTitleIssue := &community.GitHubIssue{IssueNumber: 1, Title: "   \t\n", State: community.GitHubIssueStateOpen}
		assert.ErrorIs(t, wsTitleIssue.Validate(), community.ErrEmptyIssueTitle)

		// Invalid issue state
		invalidStateIssue := &community.GitHubIssue{IssueNumber: 1, Title: "Valid title", State: "in_progress"}
		assert.ErrorIs(t, invalidStateIssue.Validate(), community.ErrInvalidIssueState)

		emptyStateIssue := &community.GitHubIssue{IssueNumber: 1, Title: "Valid title", State: ""}
		assert.ErrorIs(t, emptyStateIssue.Validate(), community.ErrInvalidIssueState)

		// Valid open and closed issues
		validOpen := &community.GitHubIssue{IssueNumber: 10, Title: "Fix bug", State: community.GitHubIssueStateOpen}
		assert.NoError(t, validOpen.Validate())

		validClosed := &community.GitHubIssue{IssueNumber: 11, Title: "Closed bug", State: community.GitHubIssueStateClosed}
		assert.NoError(t, validClosed.Validate())
	})

	// 2. GitHubIssueResolution validation
	t.Run("GitHubIssueResolution_Validation", func(t *testing.T) {
		// Nil resolution
		var nilRes *community.GitHubIssueResolution
		assert.ErrorIs(t, nilRes.Validate(), community.ErrNilGitHubIssueResolution)

		// Zero and negative issue numbers
		zeroRes := &community.GitHubIssueResolution{IssueNumber: 0, ResolutionStatus: community.IssueResolutionResolved}
		assert.ErrorIs(t, zeroRes.Validate(), community.ErrInvalidIssueNumber)

		negRes := &community.GitHubIssueResolution{IssueNumber: -1, ResolutionStatus: community.IssueResolutionResolved}
		assert.ErrorIs(t, negRes.Validate(), community.ErrInvalidIssueNumber)

		// Invalid resolution status
		invalidStatusRes := &community.GitHubIssueResolution{IssueNumber: 5, ResolutionStatus: "abandoned"}
		assert.ErrorIs(t, invalidStatusRes.Validate(), community.ErrInvalidResolutionStatus)

		emptyStatusRes := &community.GitHubIssueResolution{IssueNumber: 5, ResolutionStatus: ""}
		assert.ErrorIs(t, emptyStatusRes.Validate(), community.ErrInvalidResolutionStatus)

		// Valid resolution statuses
		validStatuses := []string{
			community.IssueResolutionResolved,
			community.IssueResolutionDeferred,
			community.IssueResolutionDuplicate,
			community.IssueResolutionWontFix,
		}
		for _, s := range validStatuses {
			res := &community.GitHubIssueResolution{IssueNumber: 42, ResolutionStatus: s}
			assert.NoError(t, res.Validate(), "status %s must be valid", s)
		}
	})

	// 3. IsCritical classification
	t.Run("IsCritical_Evaluation", func(t *testing.T) {
		var nilIssue *community.GitHubIssue
		assert.False(t, nilIssue.IsCritical())

		// Critical labels
		criticalLabels := []string{"critical", "blocker", "p0", "p1", "security", "bug"}
		for _, label := range criticalLabels {
			issue := &community.GitHubIssue{
				IssueNumber: 1,
				Title:       "Sample Issue",
				Labels:      []string{label},
			}
			assert.True(t, issue.IsCritical(), "label %q must be recognized as critical", label)
		}

		// Critical titles
		critTitle1 := &community.GitHubIssue{IssueNumber: 2, Title: "[CRITICAL] Out of memory crash"}
		assert.True(t, critTitle1.IsCritical())

		critTitle2 := &community.GitHubIssue{IssueNumber: 3, Title: "Unexpected kernel panic in scheduler"}
		assert.True(t, critTitle2.IsCritical())

		// Non-critical issues
		nonCrit := &community.GitHubIssue{
			IssueNumber: 4,
			Title:       "Update documentation typos",
			Labels:      []string{"docs", "enhancement"},
		}
		assert.False(t, nonCrit.IsCritical())
	})

	// 4. Nil receiver safety across all session methods
	t.Run("NilReceiver_Safety", func(t *testing.T) {
		var nilSession *community.RepoResolverConvergenceSession

		assert.ErrorIs(t, nilSession.IngestGitHubIssue(community.GitHubIssue{IssueNumber: 1, Title: "A", State: community.GitHubIssueStateOpen}), community.ErrNilConvergenceSession)
		assert.ErrorIs(t, nilSession.IngestGitHubIssues([]community.GitHubIssue{{IssueNumber: 1, Title: "A", State: community.GitHubIssueStateOpen}}), community.ErrNilConvergenceSession)
		assert.ErrorIs(t, nilSession.ResolveGitHubIssue(community.GitHubIssueResolution{IssueNumber: 1, ResolutionStatus: community.IssueResolutionResolved}), community.ErrNilConvergenceSession)
		assert.Nil(t, nilSession.ListPendingGitHubIssues())
		assert.Nil(t, nilSession.ListResolvedGitHubIssues())

		res, err := nilSession.GetIssueResolution(1)
		assert.ErrorIs(t, err, community.ErrNilConvergenceSession)
		assert.Nil(t, res)

		assert.Equal(t, 0, nilSession.CountPendingCriticalIssues())
	})
}

// TestRepoResolverIssueManagement_OperationalDynamicProof satisfies CRIT-1793769968219368000-9b6882e3
// and BLI-1793769968219368500-93acba54.
// Verifies lifecycle operations: ingestion, pending queries, resolution transitions, and convergence steering impact.
func TestRepoResolverIssueManagement_OperationalDynamicProof(t *testing.T) {
	session, err := community.NewRepoResolverConvergenceSession("session-ops-001", "role-resolver", nil)
	require.NoError(t, err)
	require.NotNil(t, session)

	now := time.Now().UTC()

	// 1. Batch ingestion of diverse issues
	issues := []community.GitHubIssue{
		{
			IssueNumber: 101,
			Title:       "Kernel memory leak under load",
			State:       community.GitHubIssueStateOpen,
			Labels:      []string{"critical", "performance"},
			Author:      "octocat",
			CreatedAt:   now.Add(-2 * time.Hour),
			UpdatedAt:   now.Add(-1 * time.Hour),
		},
		{
			IssueNumber: 102,
			Title:       "Update CLI help strings",
			State:       community.GitHubIssueStateOpen,
			Labels:      []string{"docs"},
			Author:      "contributor",
			CreatedAt:   now.Add(-3 * time.Hour),
			UpdatedAt:   now.Add(-2 * time.Hour),
		},
		{
			IssueNumber: 103,
			Title:       "Already closed historical defect",
			State:       community.GitHubIssueStateClosed,
			Labels:      []string{"bug"},
			Author:      "reviewer",
			CreatedAt:   now.Add(-10 * time.Hour),
			UpdatedAt:   now.Add(-5 * time.Hour),
		},
	}

	err = session.IngestGitHubIssues(issues)
	require.NoError(t, err)

	// Ingest single additional issue
	err = session.IngestGitHubIssue(community.GitHubIssue{
		IssueNumber: 104,
		Title:       "Flaky integration test in transceiver",
		State:       community.GitHubIssueStateOpen,
		Labels:      []string{"bug"},
		CreatedAt:   now,
	})
	require.NoError(t, err)

	// 2. Query pending issues (closed issue #103 should be excluded)
	pending := session.ListPendingGitHubIssues()
	require.Len(t, pending, 3, "pending issues must contain only open issues (101, 102, 104)")
	assert.Equal(t, 101, pending[0].IssueNumber)
	assert.Equal(t, 102, pending[1].IssueNumber)
	assert.Equal(t, 104, pending[2].IssueNumber)

	// Critical count should reflect #101 and #104
	assert.Equal(t, 2, session.CountPendingCriticalIssues())

	// 3. Resolve issue #101 as resolved with target commit
	err = session.ResolveGitHubIssue(community.GitHubIssueResolution{
		IssueNumber:      101,
		ResolutionStatus: community.IssueResolutionResolved,
		TargetCommitHash: "c0ffee1234567890",
		ResolutionNotes:  "Fixed pool leak by releasing connection in defer block",
		ResolvedAt:       now,
	})
	require.NoError(t, err)

	// Resolve issue #102 as deferred (must remain pending)
	err = session.ResolveGitHubIssue(community.GitHubIssueResolution{
		IssueNumber:      102,
		ResolutionStatus: community.IssueResolutionDeferred,
		ResolutionNotes:  "Deferred to documentation sprint in Cycle 36",
	})
	require.NoError(t, err)

	// Query pending again: #101 is resolved (excluded), #102 is deferred (included), #104 is open (included)
	pendingAfter := session.ListPendingGitHubIssues()
	require.Len(t, pendingAfter, 2)
	assert.Equal(t, 102, pendingAfter[0].IssueNumber)
	assert.Equal(t, 104, pendingAfter[1].IssueNumber)

	// Verify resolution retrieval
	res101, err := session.GetIssueResolution(101)
	require.NoError(t, err)
	require.NotNil(t, res101)
	assert.Equal(t, 101, res101.IssueNumber)
	assert.Equal(t, community.IssueResolutionResolved, res101.ResolutionStatus)
	assert.Equal(t, "c0ffee1234567890", res101.TargetCommitHash)

	// List all resolutions
	resolvedList := session.ListResolvedGitHubIssues()
	require.Len(t, resolvedList, 2)
	assert.Equal(t, 101, resolvedList[0].IssueNumber)
	assert.Equal(t, 102, resolvedList[1].IssueNumber)

	// 4. Dynamic steering integration with EvaluateConvergence()
	// Record telemetry: 100% passing steps across all pipeline stages
	stages := []community.PipelineStageMetric{
		{JobID: "job-1", StepName: "build", Outcome: community.OutcomeSuccess, Duration: 500 * time.Millisecond, StartedAt: now, CompletedAt: now.Add(500 * time.Millisecond)},
		{JobID: "job-1", StepName: "unit-test", Outcome: community.OutcomeSuccess, Duration: 800 * time.Millisecond, StartedAt: now.Add(500 * time.Millisecond), CompletedAt: now.Add(1300 * time.Millisecond)},
		{JobID: "job-1", StepName: "integration-test", Outcome: community.OutcomeSuccess, Duration: 1200 * time.Millisecond, StartedAt: now.Add(1300 * time.Millisecond), CompletedAt: now.Add(2500 * time.Millisecond)},
	}
	require.NoError(t, session.IngestMetrics(stages))

	// Because #104 is still an active critical bug, EvaluateConvergence must NOT advance cycle!
	evalWithCrit, err := session.EvaluateConvergence()
	require.NoError(t, err)
	require.NotNil(t, evalWithCrit)
	assert.Equal(t, community.ConvergenceStatusActionRequired, evalWithCrit.Status)
	assert.Equal(t, community.PolicyActionTriageFailures, evalWithCrit.RecommendedAction)
	assert.Contains(t, evalWithCrit.Rationale, "critical GitHub issue")

	// Now resolve the final critical issue #104
	err = session.ResolveGitHubIssue(community.GitHubIssueResolution{
		IssueNumber:      104,
		ResolutionStatus: community.IssueResolutionResolved,
		TargetCommitHash: "deadbeef98765432",
		ResolutionNotes:  "Stabilized transceiver channel synchronization",
	})
	require.NoError(t, err)
	assert.Equal(t, 0, session.CountPendingCriticalIssues())

	// Re-evaluate convergence: with zero pending critical issues and 100% telemetry, cycle must advance!
	evalConverged, err := session.EvaluateConvergence()
	require.NoError(t, err)
	require.NotNil(t, evalConverged)
	assert.Equal(t, community.ConvergenceStatusConverged, evalConverged.Status)
	assert.Equal(t, community.PolicyActionAdvanceCycle, evalConverged.RecommendedAction)
	assert.Contains(t, evalConverged.Rationale, "Pipeline converged")

	// 5. Verify ExportSessionSummary includes accurate issue counters
	summary := session.ExportSessionSummary()
	assert.Equal(t, 1, summary["pending_issues_count"])          // #102 is deferred
	assert.Equal(t, 3, summary["resolved_issues_count"])         // #101, #102, #104
	assert.Equal(t, 0, summary["pending_critical_issues_count"]) // 0 critical issues pending
}

// TestRepoResolverIssueManagement_NegativeAdversarialBoundary satisfies CRIT-1793769968219369000-7838632f
// and BLI-1793769968219369000-08d4053e.
// Verifies boundary errors, invalid lookups, and concurrent thread-safety under load.
func TestRepoResolverIssueManagement_NegativeAdversarialBoundary(t *testing.T) {
	session, err := community.NewRepoResolverConvergenceSession("session-boundary-001", "role-resolver", nil)
	require.NoError(t, err)

	// 1. Resolve non-existent issue
	errResolveMissing := session.ResolveGitHubIssue(community.GitHubIssueResolution{
		IssueNumber:      9999,
		ResolutionStatus: community.IssueResolutionResolved,
	})
	assert.ErrorIs(t, errResolveMissing, community.ErrIssueNotFound)

	// 2. Query non-existent resolution
	resMissing, errGetMissing := session.GetIssueResolution(9999)
	assert.ErrorIs(t, errGetMissing, community.ErrIssueNotFound)
	assert.Nil(t, resMissing)

	// 3. Negative issue number lookups
	_, errNegLookup := session.GetIssueResolution(-10)
	assert.ErrorIs(t, errNegLookup, community.ErrInvalidIssueNumber)

	// 4. Batch ingestion with partial invalid issue aborts cleanly
	mixedBatch := []community.GitHubIssue{
		{IssueNumber: 201, Title: "Valid 1", State: community.GitHubIssueStateOpen},
		{IssueNumber: -2, Title: "Invalid", State: community.GitHubIssueStateOpen},
	}
	errBatchFail := session.IngestGitHubIssues(mixedBatch)
	assert.ErrorIs(t, errBatchFail, community.ErrInvalidIssueNumber)

	// 5. Concurrent Ingestion, Resolution, and Convergence Evaluation under high load
	// POL-CONCURRENCY: Absolutely no naked go func() calls; managed via goroutinelabels.
	t.Run("ConcurrentExecution_ThreadSafety", func(t *testing.T) {
		const workerCount = 20
		const iterations = 50

		var wg sync.WaitGroup

		// Seed initial issues
		for i := 1; i <= workerCount; i++ {
			_ = session.IngestGitHubIssue(community.GitHubIssue{
				IssueNumber: i,
				Title:       fmt.Sprintf("Initial Issue #%d", i),
				State:       community.GitHubIssueStateOpen,
				Labels:      []string{"bug"},
			})
		}

		// Ingestion workers
		for w := 0; w < workerCount; w++ {
			workerID := w
			wg.Add(1)
			goroutinelabels.NewGoroutine("test_issue_ingest_worker", "concurrently ingests github issues").
				StartSimple(func() {
					defer wg.Done()
					for iter := 0; iter < iterations; iter++ {
						num := 1000 + (workerID * iterations) + iter
						_ = session.IngestGitHubIssue(community.GitHubIssue{
							IssueNumber: num,
							Title:       fmt.Sprintf("Concurrent Issue %d-%d", workerID, iter),
							State:       community.GitHubIssueStateOpen,
							Labels:      []string{"critical"},
						})
					}
				})
		}

		// Resolution workers
		for w := 0; w < workerCount; w++ {
			workerID := w
			wg.Add(1)
			goroutinelabels.NewGoroutine("test_issue_resolve_worker", "concurrently resolves github issues").
				StartSimple(func() {
					defer wg.Done()
					for iter := 0; iter < iterations; iter++ {
						num := 1000 + (workerID * iterations) + iter
						_ = session.ResolveGitHubIssue(community.GitHubIssueResolution{
							IssueNumber:      num,
							ResolutionStatus: community.IssueResolutionResolved,
							TargetCommitHash: "abc123commit",
						})
					}
				})
		}

		// Read & Evaluation workers
		for w := 0; w < 5; w++ {
			wg.Add(1)
			goroutinelabels.NewGoroutine("test_issue_eval_worker", "concurrently reads and evaluates session").
				StartSimple(func() {
					defer wg.Done()
					for iter := 0; iter < iterations; iter++ {
						_ = session.ListPendingGitHubIssues()
						_ = session.ListResolvedGitHubIssues()
						_ = session.CountPendingCriticalIssues()
						_ = session.ExportSessionSummary()
						_, _ = session.EvaluateConvergence()
					}
				})
		}

		wg.Wait()

		// Post-concurrency sanity check
		summary := session.ExportSessionSummary()
		assert.NotNil(t, summary)
		assert.GreaterOrEqual(t, summary["resolved_issues_count"].(int), workerCount)
	})
}
