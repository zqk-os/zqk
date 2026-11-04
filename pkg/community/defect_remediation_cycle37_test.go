package community_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/cmd/zqk/workflow"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/predicate"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// TestDefectRemediationCycle37_StaticFloorInvariant verifies:
// 1. GH-921: Priority plan lifecycle precondition string in YAML and constants specifies 'planned'
// 2. Parity between schema vocabulary ('planned') and conversational vocabulary ('ready')
// 3. DSL compile predicates and GoValidator recognition of both variants
func TestDefectRemediationCycle37_StaticFloorInvariant(t *testing.T) {
	// Find repo root using fileutil (POL-CODE-007)
	cwd, err := fileutil.Getwd()
	require.NoError(t, err)

	repoRoot := cwd
	for {
		if _, err := fileutil.Stat(filepath.Join(repoRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(repoRoot)
		require.NotEqual(t, repoRoot, parent, "could not find repo root containing go.mod")
		repoRoot = parent
	}

	// 1. Verify priority_plan_lifecycle.yaml contains 'planned' in precondition string
	lifecyclePath := filepath.Join(repoRoot, "packs", "work", "lifecycles", "priority_plan_lifecycle.yaml")
	data, readErr := fileutil.ReadFile(lifecyclePath)
	require.NoError(t, readErr, "priority_plan_lifecycle.yaml must exist and be readable")
	lifecycleContent := string(data)

	const expectedPrecondition = "at least one planned backlog_item references this plan via priority_plan_ref"
	assert.Contains(t, lifecycleContent, expectedPrecondition,
		"priority_plan_lifecycle.yaml must declare 'planned' in status and transition preconditions")

	// 2. Verify priority_plan_builder.go declares 'planned' precondition
	builderPath := filepath.Join(repoRoot, "packs", "work", "bldr_lifecycle_v1", "priority_plan_builder.go")
	builderData, bReadErr := fileutil.ReadFile(builderPath)
	require.NoError(t, bReadErr, "priority_plan_builder.go must exist and be readable")
	assert.Contains(t, string(builderData), expectedPrecondition,
		"priority_plan_builder.go must declare 'planned' in Go status definitions")

	// 3. Verify validation.PrecondReadyBacklogReferencesPlan constant matches expected string
	assert.Equal(t, expectedPrecondition, validation.PrecondReadyBacklogReferencesPlan,
		"PrecondReadyBacklogReferencesPlan constant must use 'planned' schema vocabulary")

	// 4. Verify predicate DSL compiler recognizes both planned and legacy ready forms
	gatePlanned, okPlanned := predicate.CompilePrecondition(expectedPrecondition)
	assert.True(t, okPlanned, "predicate compiler must compile planned precondition")
	assert.Equal(t, "ready_backlog_references_plan", gatePlanned)

	const legacyPrecondition = "at least one ready backlog_item references this plan via priority_plan_ref"
	gateReady, okReady := predicate.CompilePrecondition(legacyPrecondition)
	assert.True(t, okReady, "predicate compiler must compile legacy ready precondition for backwards parity")
	assert.Equal(t, "ready_backlog_references_plan", gateReady)

	// 5. Verify GoValidator pipeline recognizes both tokens
	gv := validation.NewGoValidator()
	obj := map[string]any{objects.FieldKeyID: "PRI-STATIC-TEST"}
	opts := &validation.ValidationOptions{
		DependentsLookup:   func(string) []string { return []string{} },
		ObjectStatusLookup: func(string) (string, error) { return objects.ObjectStatusPlanned, nil },
	}

	// Token with 'planned'
	metPlanned, recPlanned := gv.EvaluatePrecondition(expectedPrecondition, obj, opts)
	assert.True(t, recPlanned, "GoValidator must recognize 'planned' precondition token")
	assert.False(t, metPlanned, "with 0 dependents, planned precondition should fail closed")

	// Token with 'ready' (backward compatibility)
	metReady, recReady := gv.EvaluatePrecondition(legacyPrecondition, obj, opts)
	assert.True(t, recReady, "GoValidator must recognize 'ready' precondition token")
	assert.False(t, metReady, "with 0 dependents, ready precondition should fail closed")
}

// TestDefectRemediationCycle37_OperationalDynamicProof verifies:
// 1. GH-913: Trace pipeline bundle generation inherits goal_refs from requirement map
// 2. Multi-facet criteria and 1:1 test case generation
// 3. Concurrency safety under managed goroutines (POL-CONCURRENCY)
func TestDefectRemediationCycle37_OperationalDynamicProof(t *testing.T) {
	// Case 1: Requirement with goal_refs as []string
	t.Run("InheritGoalRefs_SliceString", func(t *testing.T) {
		reqObj := map[string]any{
			"_kind":                  objects.KindRequirement,
			objects.FieldKeyTitle:    "Defect Remediation Hardening",
			objects.FieldKeyGoalRefs: []string{"GOAL-1793582863506864000-54c3acc7"},
		}

		bundle, allCriteria, needCriteria := workflow.GenerateTracePipelineBundle("REQ-CYCLE37-001", reqObj, nil)
		require.NotNil(t, bundle, "bundle must be generated")
		assert.True(t, needCriteria, "criteria should be flagged as needed")
		assert.Len(t, bundle.Objects.Criteria, 3, "must generate 3 base criteria")
		assert.Len(t, bundle.Objects.TestCases, 1, "must generate 1 unified test case")
		assert.Len(t, bundle.Objects.BacklogItems, 3, "must generate 3 atomic backlog items")
		assert.Len(t, allCriteria, 3)

		for idx, bli := range bundle.Objects.BacklogItems {
			assert.Len(t, bli.GoalRefs, 1, "backlog item %d must inherit goal_refs", idx)
			assert.Equal(t, "GOAL-1793582863506864000-54c3acc7", bli.GoalRefs[0])
			assert.Len(t, bli.CriteriaRefs, 1, "backlog item %d must link exactly 1 criterion", idx)
			assert.Len(t, bli.TestCaseRefs, 1, "backlog item %d must link unified test case", idx)
		}
	})

	// Case 2: Requirement with goal_refs as []any (e.g., from unmarshaled YAML / JSON)
	t.Run("InheritGoalRefs_SliceAny", func(t *testing.T) {
		reqObj := map[string]any{
			"_kind":                  objects.KindRequirement,
			objects.FieldKeyTitle:    "Dynamic Unmarshaled Payload",
			objects.FieldKeyGoalRefs: []any{"GOAL-ANY-999"},
		}

		bundle, _, _ := workflow.GenerateTracePipelineBundle("REQ-CYCLE37-002", reqObj, nil)
		require.NotNil(t, bundle)
		require.NotEmpty(t, bundle.Objects.BacklogItems)

		for idx, bli := range bundle.Objects.BacklogItems {
			assert.Len(t, bli.GoalRefs, 1, "backlog item %d must inherit goal_refs", idx)
			assert.Equal(t, "GOAL-ANY-999", bli.GoalRefs[0])
		}
	})

	// Case 3: Comma-separated goal_refs string
	t.Run("InheritGoalRefs_CommaSeparatedString", func(t *testing.T) {
		reqObj := map[string]any{
			"_kind":                  objects.KindRequirement,
			objects.FieldKeyTitle:    "Multi Goal Ingestion",
			objects.FieldKeyGoalRefs: "GOAL-ALPHA, GOAL-BETA",
		}

		bundle, _, _ := workflow.GenerateTracePipelineBundle("REQ-CYCLE37-003", reqObj, nil)
		require.NotNil(t, bundle)
		require.NotEmpty(t, bundle.Objects.BacklogItems)

		for idx, bli := range bundle.Objects.BacklogItems {
			assert.Len(t, bli.GoalRefs, 2, "backlog item %d must inherit all parsed goal_refs", idx)
			assert.Contains(t, bli.GoalRefs, "GOAL-ALPHA")
			assert.Contains(t, bli.GoalRefs, "GOAL-BETA")
		}
	})

	// Case 4: Singular goal_ref field
	t.Run("InheritGoalRefs_SingularGoalRef", func(t *testing.T) {
		reqObj := map[string]any{
			"_kind":               objects.KindRequirement,
			objects.FieldKeyTitle: "Singular Goal Ingestion",
			"goal_ref":            "GOAL-SOLO-400",
		}

		bundle, _, _ := workflow.GenerateTracePipelineBundle("REQ-CYCLE37-004", reqObj, nil)
		require.NotNil(t, bundle)
		require.NotEmpty(t, bundle.Objects.BacklogItems)

		for idx, bli := range bundle.Objects.BacklogItems {
			assert.Len(t, bli.GoalRefs, 1, "backlog item %d must inherit singular goal_ref", idx)
			assert.Equal(t, "GOAL-SOLO-400", bli.GoalRefs[0])
		}
	})

	// Case 5: Concurrent Bundle Generation Safety (POL-CONCURRENCY)
	t.Run("Concurrent_Bundle_Generation", func(t *testing.T) {
		const numWorkers = 8
		var wg sync.WaitGroup
		errs := make(chan error, numWorkers)

		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			workerIdx := i
			goroutinelabels.NewGoroutine("cycle37_trace_pipeline_worker", "tests concurrent bundle generation").
				StartSimple(func() {
					defer wg.Done()
					req := map[string]any{
						"_kind":                  objects.KindRequirement,
						objects.FieldKeyTitle:    "Concurrent Req",
						objects.FieldKeyGoalRefs: []string{"GOAL-CONCURRENT-001"},
					}
					b, _, _ := workflow.GenerateTracePipelineBundle("REQ-CONCURRENT", req, nil)
					if b == nil || len(b.Objects.BacklogItems) != 3 {
						errs <- assert.AnError
						return
					}
					if len(b.Objects.BacklogItems[0].GoalRefs) != 1 || b.Objects.BacklogItems[0].GoalRefs[0] != "GOAL-CONCURRENT-001" {
						errs <- assert.AnError
						return
					}
					_ = workerIdx
				})
		}

		done := make(chan struct{})
		goroutinelabels.NewGoroutine("cycle37_waiter", "waits for concurrent bundle tests").
			StartSimple(func() {
				wg.Wait()
				close(done)
			})

		select {
		case <-done:
			close(errs)
			for err := range errs {
				assert.NoError(t, err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("concurrent bundle generation timed out")
		}
	})
}

// TestDefectRemediationCycle37_NegativeAdversarialBoundary verifies:
// 1. Rejection of corrupt, nil, or empty goal_refs (no ghost "<nil>" or whitespace strings)
// 2. Deduplication and whitespace trimming of adversarial goal inputs
// 3. Priority plan lifecycle rejection when backlog items lack status 'planned'
func TestDefectRemediationCycle37_NegativeAdversarialBoundary(t *testing.T) {
	// Adversarial 1: Empty and nil goal_refs slice
	t.Run("EmptyAndNilGoalRefs_CleanlyHandled", func(t *testing.T) {
		reqObj := map[string]any{
			"_kind":                  objects.KindRequirement,
			objects.FieldKeyTitle:    "Adversarial Nil Refs",
			objects.FieldKeyGoalRefs: []any{nil, "", "   ", "<nil>"},
		}

		bundle, _, _ := workflow.GenerateTracePipelineBundle("REQ-ADV-001", reqObj, nil)
		require.NotNil(t, bundle)
		for idx, bli := range bundle.Objects.BacklogItems {
			assert.Empty(t, bli.GoalRefs, "corrupt/nil entries must not yield valid goal_refs on backlog item %d", idx)
		}
	})

	// Adversarial 2: Duplicate and untrimmed goal references
	t.Run("DuplicateAndUntrimmedGoalRefs_Deduplicated", func(t *testing.T) {
		reqObj := map[string]any{
			"_kind":                  objects.KindRequirement,
			objects.FieldKeyTitle:    "Adversarial Duplicate Refs",
			objects.FieldKeyGoalRefs: []string{"  GOAL-DUP-1  ", "GOAL-DUP-1", "   ", "GOAL-DUP-2", "GOAL-DUP-1"},
		}

		bundle, _, _ := workflow.GenerateTracePipelineBundle("REQ-ADV-002", reqObj, nil)
		require.NotNil(t, bundle)
		for _, bli := range bundle.Objects.BacklogItems {
			assert.Len(t, bli.GoalRefs, 2, "must deduplicate and trim whitespace")
			assert.Equal(t, []string{"GOAL-DUP-1", "GOAL-DUP-2"}, bli.GoalRefs)
		}
	})

	// Adversarial 3: Non-string values in goal_refs slice
	t.Run("NonStringValues_HandledSafely", func(t *testing.T) {
		reqObj := map[string]any{
			"_kind":                  objects.KindRequirement,
			objects.FieldKeyTitle:    "Adversarial Types",
			objects.FieldKeyGoalRefs: []any{12345, true, map[string]any{"bad": "data"}},
		}

		// Should not panic or crash
		bundle, _, _ := workflow.GenerateTracePipelineBundle("REQ-ADV-003", reqObj, nil)
		require.NotNil(t, bundle)
		for _, bli := range bundle.Objects.BacklogItems {
			assert.NotEmpty(t, bli.GoalRefs, "converted primitive string representation accepted")
			assert.Contains(t, bli.GoalRefs, "12345")
			assert.Contains(t, bli.GoalRefs, "true")
		}
	})

	// Adversarial 4: Priority Plan validation - Child status "ready" vs "planned"
	// Conversational "ready" is NOT a valid status for backlog_item; it MUST be "planned".
	t.Run("PriorityPlan_RejectsConversationalReadyStatus", func(t *testing.T) {
		gv := validation.NewGoValidator()
		planObj := map[string]any{
			objects.FieldKeyID:     "PRI-ADV-PLAN",
			objects.FieldKeyStatus: objects.ObjectStatusGrooming,
		}

		// Attempting promotion to active with a child whose status is "ready" (invalid in schema)
		invalidOpts := &validation.ValidationOptions{
			CurrentState:      objects.ObjectStatusGrooming,
			ValidateLifecycle: true,
			DependentsLookup: func(string) []string {
				return []string{"BLI-INVALID-READY"}
			},
			ObjectStatusLookup: func(string) (string, error) {
				return "ready", nil // "ready" is not "planned"
			},
		}

		errs, _ := gv.ValidateLifecycleState(context.Background(), objects.KindPriorityPlan, objects.ObjectStatusActive, objects.ObjectStatusGrooming, planObj, invalidOpts)
		assert.NotEmpty(t, errs, "promotion must fail when child has status 'ready' instead of canonical 'planned'")

		hasPreconditionError := false
		for _, e := range errs {
			if strings.Contains(e.Message, validation.PrecondReadyBacklogReferencesPlan) {
				hasPreconditionError = true
				break
			}
		}
		assert.True(t, hasPreconditionError, "must report precondition failure when zero children have status 'planned'")
	})

	// Adversarial 5: Priority Plan validation - Child status "in_progress" blocks seal to active
	t.Run("PriorityPlan_RejectsInProgressChildWhenSealingActive", func(t *testing.T) {
		gv := validation.NewGoValidator()
		planObj := map[string]any{
			objects.FieldKeyID:     "PRI-ADV-PLAN-2",
			objects.FieldKeyStatus: objects.ObjectStatusGrooming,
		}

		inProgressOpts := &validation.ValidationOptions{
			CurrentState:      objects.ObjectStatusGrooming,
			ValidateLifecycle: true,
			DependentsLookup: func(string) []string {
				return []string{"BLI-IN-PROGRESS"}
			},
			ObjectStatusLookup: func(string) (string, error) {
				return objects.ObjectStatusInProgress, nil
			},
		}

		errs, _ := gv.ValidateLifecycleState(context.Background(), objects.KindPriorityPlan, objects.ObjectStatusActive, objects.ObjectStatusGrooming, planObj, inProgressOpts)
		assert.NotEmpty(t, errs, "promotion to active must fail when children are already in_progress")
	})
}
