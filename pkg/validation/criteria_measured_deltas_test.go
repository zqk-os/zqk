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

	// Explicit category 'refactor' is always a refactor criterion
	assert.True(t, IsRefactorCriterion(map[string]any{
		objects.FieldKeyCategory: "refactor",
		objects.FieldKeyTitle:    "Any title whatsoever",
	}))

	// Explicit category 'functional' is never a refactor criterion, even if title or description mentions refactor/extraction
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
