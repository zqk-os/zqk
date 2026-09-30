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
