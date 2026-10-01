package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/predicate"
)

// TestOverlayDSL_PreconditionConsolidation verifies CRI-CEF-DSL-PRECOND-CONSOLIDATION:
// Preconditions compile and evaluate accurately through the unified Kernel Predicate DSL.
func TestOverlayDSL_PreconditionConsolidation(t *testing.T) {
	gv := NewGoValidator()
	opts := &ValidationOptions{}

	t.Run("empty precondition succeeds trivially", func(t *testing.T) {
		handled, met := evalOverlayDSLStage(gv, "", nil, opts)
		assert.True(t, handled)
		assert.True(t, met)
	})

	t.Run("field_nonempty predicate", func(t *testing.T) {
		obj := map[string]any{
			"id":          "BLI-123",
			"title":       "Test Item",
			"description": "Valid non-empty description",
			"items":       []string{"a", "b"},
			"empty_list":  []string{},
			"empty_str":   "   ",
		}

		// Direct canonical expression
		handled, met := evalOverlayDSLStage(gv, "field_nonempty:title", obj, opts)
		assert.True(t, handled)
		assert.True(t, met)

		handled, met = evalOverlayDSLStage(gv, "field_nonempty:items", obj, opts)
		assert.True(t, handled)
		assert.True(t, met)

		// Missing field
		handled, met = evalOverlayDSLStage(gv, "field_nonempty:nonexistent_field", obj, opts)
		assert.True(t, handled)
		assert.False(t, met)

		// Empty string
		handled, met = evalOverlayDSLStage(gv, "field_nonempty:empty_str", obj, opts)
		assert.True(t, handled)
		assert.False(t, met)

		// Empty slice
		handled, met = evalOverlayDSLStage(gv, "field_nonempty:empty_list", obj, opts)
		assert.True(t, handled)
		assert.False(t, met)
	})

	t.Run("field_cleared predicate", func(t *testing.T) {
		obj := map[string]any{
			"id":          "BLI-123",
			"empty_str":   "",
			"empty_slice": []any{},
			"populated":   "data present",
		}

		handled, met := evalOverlayDSLStage(gv, "field_cleared:empty_str", obj, opts)
		assert.True(t, handled)
		assert.True(t, met)

		handled, met = evalOverlayDSLStage(gv, "field_cleared:missing_field", obj, opts)
		assert.True(t, handled)
		assert.True(t, met)

		handled, met = evalOverlayDSLStage(gv, "field_cleared:populated", obj, opts)
		assert.True(t, handled)
		assert.False(t, met)
	})

	t.Run("field_matches regex predicate", func(t *testing.T) {
		obj := map[string]any{
			"id":      "BLI-456",
			"version": "v1.2.3",
		}

		handled, met := evalOverlayDSLStage(gv, "field_matches:version:^v[0-9]+\\.[0-9]+\\.[0-9]+$", obj, opts)
		assert.True(t, handled)
		assert.True(t, met)

		handled, met = evalOverlayDSLStage(gv, "field_matches:version:^v0\\.", obj, opts)
		assert.True(t, handled)
		assert.False(t, met)
	})
}

// TestOverlayDSL_PreconditionCompoundAndProse verifies compound AND logic, disjunctions, and prose compilation.
func TestOverlayDSL_PreconditionCompoundAndProse(t *testing.T) {
	gv := NewGoValidator()
	opts := &ValidationOptions{}

	t.Run("compound predicates via AND combinator", func(t *testing.T) {
		obj := map[string]any{
			"id":    "BLI-789",
			"title": "Good title",
			"desc":  "Good desc",
		}

		handled, met := evalOverlayDSLStage(gv, "field_nonempty:title AND field_nonempty:desc", obj, opts)
		assert.True(t, handled)
		assert.True(t, met)

		handled, met = evalOverlayDSLStage(gv, "field_nonempty:title AND field_nonempty:missing", obj, opts)
		assert.True(t, handled)
		assert.False(t, met)
	})

	t.Run("any_nonempty disjunction predicate", func(t *testing.T) {
		objWithWorkstream := map[string]any{
			"id":              "BLI-101",
			"workstream_refs": []string{"WS-100"},
		}
		objWithMilestone := map[string]any{
			"id":            "BLI-102",
			"milestone_ref": "MIL-200",
		}
		objWithNeither := map[string]any{
			"id":              "BLI-103",
			"workstream_refs": []string{},
			"milestone_ref":   "",
		}

		// Comma-delimited
		handled, met := evalOverlayDSLStage(gv, "any_nonempty:workstream_ref,milestone_ref", objWithWorkstream, opts)
		assert.True(t, handled)
		assert.True(t, met)

		handled, met = evalOverlayDSLStage(gv, "any_nonempty:workstream_ref,milestone_ref", objWithMilestone, opts)
		assert.True(t, handled)
		assert.True(t, met)

		handled, met = evalOverlayDSLStage(gv, "any_nonempty:workstream_ref,milestone_ref", objWithNeither, opts)
		assert.True(t, handled)
		assert.False(t, met)

		// Colon-delimited
		handled, met = evalOverlayDSLStage(gv, "any_nonempty:workstream_ref:milestone_ref", objWithWorkstream, opts)
		assert.True(t, handled)
		assert.True(t, met)

		// Prose disjunction compilation
		handled, met = evalOverlayDSLStage(gv, "at least one workstream_ref or milestone_ref linked", objWithMilestone, opts)
		assert.True(t, handled)
		assert.True(t, met)

		handled, met = evalOverlayDSLStage(gv, "at least one workstream_ref or milestone_ref linked", objWithNeither, opts)
		assert.True(t, handled)
		assert.False(t, met)
	})

	t.Run("prose compilation to predicate DSL", func(t *testing.T) {
		obj := map[string]any{
			"id":           "BLI-999",
			"title":        "My Title",
			"description":  "My Description",
			"content_size": 1024,
		}

		// "title and description are populated" compiles to field_nonempty:title AND field_nonempty:description
		handled, met := evalOverlayDSLStage(gv, "title and description are populated", obj, opts)
		assert.True(t, handled)
		assert.True(t, met)

		// "content_size measured" compiles to content_size_positive
		handled, met = evalOverlayDSLStage(gv, "content_size measured", obj, opts)
		assert.True(t, handled)
		assert.True(t, met)
	})
}

// TestOverlayDSL_ComposePredicateOp verifies CRI-CEF-DSL-COMPOSE-PREDICATE-OP:
// Predicate DSL expressions correctly validate in kernelcas compose overlay pipelines.
func TestOverlayDSL_ComposePredicateOp(t *testing.T) {
	// Verify predicate syntax checker directly
	err := predicate.ValidatePredicateSyntax("field_nonempty:title AND field_cleared:error")
	require.NoError(t, err)

	canon, ok := predicate.CompilePrecondition("title, summary, and path are populated")
	require.True(t, ok)
	assert.Contains(t, canon, "field_nonempty:title")

	preds, err := predicate.SplitPredicates(canon)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(preds), 3)
}

// TestOverlayDSL_LifecycleCorrespondenceCoverage verifies CRI-CEF-DSL-LIFECYCLE-CORRESPONDENCE:
// All registered pack lifecycles maintain valid and consistent precondition mappings.
func TestOverlayDSL_LifecycleCorrespondenceCoverage(t *testing.T) {
	loader := objects.GetGlobalLifecycleLoader()
	require.NotNil(t, loader)

	kinds := objects.GetGlobalKindMapper().GetAllKinds()
	require.NotEmpty(t, kinds)

	testedKinds := 0
	for _, kind := range kinds {
		lc, err := loader.LoadLifecycle(kind)
		if err != nil || lc == nil {
			continue
		}
		testedKinds++
		for _, trans := range lc.Transitions {
			for _, pre := range trans.Preconditions {
				if pre == "" {
					continue
				}
				// Verify precondition syntax is either parseable prose or valid predicate syntax
				_, isProse := predicate.CompilePrecondition(pre)
				syntaxErr := predicate.ValidatePredicateSyntax(pre)
				assert.True(t, isProse || syntaxErr == nil, "kind %s transition %s->%s has invalid precondition: %s", kind, trans.From, trans.To, pre)
			}
		}
	}
	assert.GreaterOrEqual(t, testedKinds, 20, "Should have verified lifecycles across at least 20 registered kinds")
}

// TestOverlayDSL_UnifiedPredicateLifecycles verifies:
// - CRI-CEF-DSL-FIELD-PRESENCE: field_nonempty, field_cleared, is set, is not empty
// - CRI-CEF-DSL-OR-CONDITIONS: any_nonempty disjunction and prose compilation
// - CRI-CEF-DSL-SNOWFLAKE-ELIMINATION: go_validator delegation through evalOverlayDSLStage
// - CRI-CEF-DSL-UNIFICATION-DOCS: grammar compliance and canonical expression validation
func TestOverlayDSL_UnifiedPredicateLifecycles(t *testing.T) {
	gv := NewGoValidator()

	// 1. CRI-CEF-DSL-FIELD-PRESENCE: Proper string trimming, slice and map verification
	t.Run("CRI-CEF-DSL-FIELD-PRESENCE", func(t *testing.T) {
		obj := map[string]any{
			"valid_str":   "  hello world  ",
			"blank_str":   "   \t\n   ",
			"empty_slice": []string{},
			"blank_slice": []string{"", "   "},
			"valid_slice": []string{"   ", "item"},
			"empty_map":   map[string]any{},
			"valid_map":   map[string]any{"k": "v"},
			"nil_field":   nil,
		}

		// Field nonempty verification
		assert.True(t, gv.checkIsNotEmptyPrecondition("valid_str is not empty", obj))
		assert.False(t, gv.checkIsNotEmptyPrecondition("blank_str is not empty", obj))
		assert.False(t, gv.checkIsNotEmptyPrecondition("empty_slice is not empty", obj))
		assert.False(t, gv.checkIsNotEmptyPrecondition("blank_slice is not empty", obj))
		assert.True(t, gv.checkIsNotEmptyPrecondition("valid_slice is not empty", obj))
		assert.False(t, gv.checkIsNotEmptyPrecondition("empty_map is not empty", obj))
		assert.True(t, gv.checkIsNotEmptyPrecondition("valid_map is not empty", obj))
		assert.False(t, gv.checkIsNotEmptyPrecondition("nil_field is not empty", obj))
		assert.False(t, gv.checkIsNotEmptyPrecondition("nonexistent is not empty", obj))

		// Field cleared verification
		_, met := evalOverlayDSLStage(gv, "field_cleared:blank_str", obj, nil)
		assert.True(t, met)
		_, met = evalOverlayDSLStage(gv, "field_cleared:empty_slice", obj, nil)
		assert.True(t, met)
		_, met = evalOverlayDSLStage(gv, "field_cleared:valid_str", obj, nil)
		assert.False(t, met)
		_, met = evalOverlayDSLStage(gv, "field_cleared:valid_map", obj, nil)
		assert.False(t, met)
	})

	// 2. CRI-CEF-DSL-OR-CONDITIONS: any_nonempty disjunction and compilePrecondition
	t.Run("CRI-CEF-DSL-OR-CONDITIONS", func(t *testing.T) {
		obj := map[string]any{
			"milestone_refs": []string{"MIL-001"},
		}

		// Precondition with "or" compiles to any_nonempty
		precond := "at least one workstream_ref or milestone_ref linked"
		canon, ok := predicate.CompilePrecondition(precond)
		require.True(t, ok)
		assert.Equal(t, "any_nonempty:workstream_ref,milestone_ref", canon)

		handled, met := evalOverlayDSLStage(gv, canon, obj, nil)
		assert.True(t, handled)
		assert.True(t, met)

		// Empty object fails disjunction
		handled, met = evalOverlayDSLStage(gv, canon, map[string]any{}, nil)
		assert.True(t, handled)
		assert.False(t, met)
	})

	// 3. CRI-CEF-DSL-SNOWFLAKE-ELIMINATION: methods route cleanly through evalOverlayDSLStage
	t.Run("CRI-CEF-DSL-SNOWFLAKE-ELIMINATION", func(t *testing.T) {
		obj := map[string]any{
			"owner_ref": "ACC-001",
			"status":    "in_progress",
		}

		assert.True(t, gv.checkIsSetPrecondition("owner_ref is set", obj))
		assert.False(t, gv.checkIsSetPrecondition("missing is set", obj))
		assert.True(t, gv.checkAtLeastPrecondition("at least one owner_ref", obj))
		assert.False(t, gv.checkAtLeastPrecondition("at least one workstream_ref", obj))
		assert.True(t, gv.checkAtLeastOrPrecondition("at least one owner_ref or author_ref", obj))
		assert.False(t, gv.checkAtLeastOrPrecondition("at least one persona_ref or team_ref", obj))
	})

	// 4. CRI-CEF-DSL-UNIFICATION-DOCS: predicate grammar syntax checks
	t.Run("CRI-CEF-DSL-UNIFICATION-DOCS", func(t *testing.T) {
		assert.NoError(t, predicate.ValidatePredicateSyntax("any_nonempty:workstream_ref,milestone_ref"))
		assert.NoError(t, predicate.ValidatePredicateSyntax("any_nonempty:a:b:c"))
		assert.NoError(t, predicate.ValidatePredicateSyntax("field_nonempty:title; any_nonempty:workstream_ref,milestone_ref"))
	})
}
