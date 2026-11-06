package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestIsRefactorOrExtractionCriterion(t *testing.T) {
	t.Parallel()

	// Should match refactoring titles
	assert.True(t, IsRefactorOrExtractionCriterion("Refactor storage layer"))
	assert.True(t, IsRefactorOrExtractionCriterion("refactor: optimize caching"))
	assert.True(t, IsRefactorOrExtractionCriterion("Refactoring of authentication pipeline"))
	assert.True(t, IsRefactorOrExtractionCriterion("Extract package storage into pkg/storage"))
	assert.True(t, IsRefactorOrExtractionCriterion("Code extraction for compiler AST"))

	// Should not match milestone-scoped child criteria
	assert.False(t, IsRefactorOrExtractionCriterion("Verify: Milestone: 1.0 - Documentation & Knowledge Base Entry"))
	assert.False(t, IsRefactorOrExtractionCriterion("Verify: Milestone: 1.0 - Boundary & Error Handling"))
	assert.False(t, IsRefactorOrExtractionCriterion("Verify: Milestone: 1.0 - Functional Acceptance"))

	// Should not match common noun extraction phrases
	assert.False(t, IsRefactorOrExtractionCriterion("AST symbol extraction from Python source"))
	assert.False(t, IsRefactorOrExtractionCriterion("Feature extraction pipeline"))
	assert.False(t, IsRefactorOrExtractionCriterion("Entity extraction from markdown headers"))
	assert.False(t, IsRefactorOrExtractionCriterion("Token extraction utility"))
	assert.False(t, IsRefactorOrExtractionCriterion("Data extraction from sqlite DB"))
	assert.False(t, IsRefactorOrExtractionCriterion("Archive extraction on startup"))
	assert.False(t, IsRefactorOrExtractionCriterion("Metadata extraction engine"))
	assert.False(t, IsRefactorOrExtractionCriterion("Context extraction for subagents"))
	assert.False(t, IsRefactorOrExtractionCriterion("Path extraction from URLs"))
	assert.False(t, IsRefactorOrExtractionCriterion("Key extraction from keystore"))
	assert.False(t, IsRefactorOrExtractionCriterion("Extract symbol from AST node"))
	assert.False(t, IsRefactorOrExtractionCriterion("Extract token from stream"))
	assert.False(t, IsRefactorOrExtractionCriterion("Extract feature flags from config"))
}

func TestIsRefactorCriterion(t *testing.T) {
	t.Parallel()

	// Nil object
	assert.False(t, IsRefactorCriterion(nil))

	// Explicit category 'refactor' is always a refactor criterion (case-insensitive)
	assert.True(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyCategory: "refactor",
		objects.FieldKeyTitle:    "Any title whatsoever",
	}))
	assert.True(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyCategory: "Refactor",
		objects.FieldKeyTitle:    "Title with mixed case category",
	}))
	assert.True(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyCategory: "  REFACTOR  ",
		objects.FieldKeyTitle:    "Title with uppercase padded category",
	}))

	// Explicit categories other than refactor (functional, performance, security, etc.)
	// are never refactor criteria, even if title or description mentions refactor/extraction
	assert.False(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyCategory:    "functional",
		objects.FieldKeyTitle:       "AST symbol extraction from Python source",
		objects.FieldKeyDescription: "Implement symbol extraction and refactor old parser utilities.",
	}))
	assert.False(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyCategory:    "functional",
		objects.FieldKeyTitle:       "Refactor storage layer",
		objects.FieldKeyDescription: "Prose mentioning refactoring.",
	}))
	assert.False(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyCategory:    "functional",
		objects.FieldKeyTitle:       "Legacy helper wrapper",
		objects.FieldKeyDescription: "Refactor legacy helper to integrate with new pipeline.",
	}))
	assert.False(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyCategory:    "performance",
		objects.FieldKeyTitle:       "Optimize legacy helper path",
		objects.FieldKeyDescription: "Refactor legacy helper to eliminate allocs.",
	}))
	assert.False(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyCategory:    "security",
		objects.FieldKeyTitle:       "Sanitize credentials",
		objects.FieldKeyDescription: "Refactor legacy helper to avoid logging secrets.",
	}))
	assert.False(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyCategory: "acceptance",
		objects.FieldKeyTitle:    "Code extraction acceptance",
	}))

	// Empty / omitted category falls back to title heuristics
	assert.True(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyTitle: "Refactor validation cache",
	}))
	assert.False(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyTitle: "AST symbol extraction from Python source",
	}))
}

func TestValidateCriteriaRefactorMeasuredDeltas_Scoping(t *testing.T) {
	t.Parallel()

	// 1. Category functional mentioning extraction or refactor in description passes without measured deltas
	functionalCrit := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyCategory:    "functional",
		objects.FieldKeyTitle:       "AST symbol extraction from Python source",
		objects.FieldKeyDescription: "Symbol extraction from Python source and refactoring of visitor passes.",
		objects.FieldKeyCompletenessValidation: []string{
			"command_exit_code:0",
		},
	}
	err := ValidateCriteriaRefactorMeasuredDeltas(functionalCrit)
	assert.Nil(t, err, "Functional criteria mentioning extraction/refactor should pass without measured deltas")

	// 2. Category refactor without measured deltas fails closed
	refactorCritNoDelta := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyCategory:    "refactor",
		objects.FieldKeyTitle:       "Decouple state cache from storage engine",
		objects.FieldKeyDescription: "Refactor cache architecture to isolate state mutations.",
		objects.FieldKeyCompletenessValidation: []string{
			"code exists in package",
		},
	}
	err = ValidateCriteriaRefactorMeasuredDeltas(refactorCritNoDelta)
	require.NotNil(t, err)
	assert.Equal(t, RuleCriteriaRefactorMeasuredDeltas, err.Rule)
	assert.Equal(t, MsgCriteriaRefactorMeasuredDeltas, err.Message)

	// 3. Category refactor with measured deltas passes
	refactorCritWithDelta := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyCategory:    "refactor",
		objects.FieldKeyTitle:       "Decouple state cache from storage engine",
		objects.FieldKeyDescription: "Refactor cache architecture to isolate state mutations.",
		objects.FieldKeyCompletenessValidation: []string{
			"measure storage package file count down by 4",
			"assert symbol StateCache absent from pkg/storage",
		},
	}
	err = ValidateCriteriaRefactorMeasuredDeltas(refactorCritWithDelta)
	assert.Nil(t, err, "Refactor criteria with measured delta assertions must pass")
}

func TestMeasuredDeltas_RefactorValidationPrecision(t *testing.T) {
	t.Parallel()

	// 1. Functional criterion with description containing "Refactor legacy helper" without measured deltas passes validation cleanly
	functionalCrit := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyCategory:    "functional",
		objects.FieldKeyTitle:       "Execute legacy helper adapter",
		objects.FieldKeyDescription: "Refactor legacy helper integration to support async context dispatch.",
		objects.FieldKeyCompletenessValidation: []string{
			"command_exit_code:0",
		},
	}
	assert.Nil(t, ValidateCriteriaRefactorMeasuredDeltas(functionalCrit),
		"Functional criterion with 'Refactor legacy helper' description must pass validation without measured deltas")

	// Verify additional categories with refactor/extract prose in description pass cleanly
	categories := []string{"performance", "security", "integration", "acceptance"}
	for _, cat := range categories {
		crit := map[string]any{
			objects.FieldKeyKind:        objects.KindCriteria,
			objects.FieldKeyCategory:    cat,
			objects.FieldKeyTitle:       "Optimize legacy helper routines",
			objects.FieldKeyDescription: "Refactor legacy helper methods to eliminate allocations under load.",
			objects.FieldKeyCompletenessValidation: []string{
				"command_exit_code:0",
			},
		}
		assert.Nil(t, ValidateCriteriaRefactorMeasuredDeltas(crit),
			"Criterion with category %q must pass validation without measured deltas", cat)
	}

	// 2. Criterion with category "refactor" without measured deltas is rejected as expected
	refactorNoDelta := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyCategory:    "refactor",
		objects.FieldKeyTitle:       "Extract legacy helper to standalone package",
		objects.FieldKeyDescription: "Refactor legacy helper to remove circular dependencies.",
		objects.FieldKeyCompletenessValidation: []string{
			"code exists in package",
		},
	}
	err := ValidateCriteriaRefactorMeasuredDeltas(refactorNoDelta)
	require.NotNil(t, err, "Refactor category criterion without measured deltas must fail")
	assert.Equal(t, RuleCriteriaRefactorMeasuredDeltas, err.Rule)
	assert.Equal(t, MsgCriteriaRefactorMeasuredDeltas, err.Message)

	// Also check mixed-case category "Refactor"
	refactorMixedCaseNoDelta := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyCategory:    "Refactor",
		objects.FieldKeyTitle:       "Extract legacy helper to standalone package",
		objects.FieldKeyDescription: "Refactor legacy helper to remove circular dependencies.",
		objects.FieldKeyCompletenessValidation: []string{
			"code exists in package",
		},
	}
	err = ValidateCriteriaRefactorMeasuredDeltas(refactorMixedCaseNoDelta)
	require.NotNil(t, err, "Mixed-case category 'Refactor' without measured deltas must fail")
	assert.Equal(t, RuleCriteriaRefactorMeasuredDeltas, err.Rule)
	assert.Equal(t, MsgCriteriaRefactorMeasuredDeltas, err.Message)

	// 3. Criterion with category "refactor" with valid measured deltas passes
	refactorWithDelta := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyCategory:    "refactor",
		objects.FieldKeyTitle:       "Extract legacy helper to standalone package",
		objects.FieldKeyDescription: "Refactor legacy helper to remove circular dependencies.",
		objects.FieldKeyCompletenessValidation: []string{
			"measure package file count down by 2",
			"assert symbol LegacyHelper absent from old package",
		},
	}
	assert.Nil(t, ValidateCriteriaRefactorMeasuredDeltas(refactorWithDelta),
		"Refactor category criterion with valid measured deltas must pass")

	// Mixed-case category "REFACTOR" with valid measured deltas passes
	refactorMixedCaseWithDelta := map[string]any{
		objects.FieldKeyKind:        objects.KindCriteria,
		objects.FieldKeyCategory:    "REFACTOR",
		objects.FieldKeyTitle:       "Extract legacy helper to standalone package",
		objects.FieldKeyDescription: "Refactor legacy helper to remove circular dependencies.",
		objects.FieldKeyCompletenessValidation: []string{
			"ast_semantic_match:type LegacyHelper interface{}",
		},
	}
	assert.Nil(t, ValidateCriteriaRefactorMeasuredDeltas(refactorMixedCaseWithDelta),
		"Uppercase category 'REFACTOR' with valid measured deltas must pass")
}
