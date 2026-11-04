package workflow

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestCycle147_ClassifyFormulaCategory_NilAndEmpty(t *testing.T) {
	assert.Equal(t, FormulaUnknown, ClassifyFormulaCategory(nil))
	assert.Equal(t, FormulaUnknown, ClassifyFormulaCategory(map[string]any{}))
	assert.Equal(t, FormulaUnknown, ClassifyFormulaCategory(map[string]any{
		"category": "arbitrary_custom_category",
	}))
}

func TestCycle147_ClassifyFormulaCategory_ExplicitFields(t *testing.T) {
	fields := []string{"formula_type", "category", "proof_type", "type"}
	testMatrix := []struct {
		val      string
		expected FormulaCategory
	}{
		{"invariant", FormulaInvariant},
		{"state_invariant", FormulaInvariant},
		{"static", FormulaInvariant},
		{"dynamic", FormulaDynamic},
		{"dynamic_behavior", FormulaDynamic},
		{"operational", FormulaDynamic},
		{"functional", FormulaDynamic},
		{"adversarial", FormulaAdversarial},
		{"negative_invariant", FormulaAdversarial},
		{"fail_closed", FormulaAdversarial},
		{"boundary", FormulaAdversarial},
	}

	for _, field := range fields {
		for _, tc := range testMatrix {
			m := map[string]any{field: tc.val}
			res := ClassifyFormulaCategory(m)
			assert.Equal(t, tc.expected, res, "field %s with val %s", field, tc.val)
		}
	}
}

func TestCycle147_ClassifyFormulaCategory_Heuristics_Adversarial(t *testing.T) {
	testKeywords := []string{
		"adversarial condition checks",
		"fail-closed on corrupted inputs",
		"negative validation proof",
		"reject invalid payloads",
		"disallow unauthorized updates",
		"malformed data detection",
	}

	for _, kw := range testKeywords {
		m := map[string]any{
			objects.FieldKeyDescription: kw,
		}
		assert.Equal(t, FormulaAdversarial, ClassifyFormulaCategory(m), "keyword: %s", kw)
	}
}

func TestCycle147_ClassifyFormulaCategory_Heuristics_Invariant(t *testing.T) {
	testKeywords := []string{
		"invariant state proof",
		"schema conforms to spec",
		"predicate assertion passes",
		"conforms to architectural standard",
		"static floor coverage",
	}

	for _, kw := range testKeywords {
		m := map[string]any{
			"statement": kw,
		}
		assert.Equal(t, FormulaInvariant, ClassifyFormulaCategory(m), "keyword: %s", kw)
	}
}

func TestCycle147_ClassifyFormulaCategory_Heuristics_Dynamic(t *testing.T) {
	testKeywords := []string{
		"dynamic response verified",
		"behavior observed under load",
		"executes workflow seamlessly",
		"runs scenario deterministically",
		"assert state transition completes",
	}

	for _, kw := range testKeywords {
		m := map[string]any{
			objects.FieldKeyTitle: kw,
		}
		assert.Equal(t, FormulaDynamic, ClassifyFormulaCategory(m), "keyword: %s", kw)
	}
}

func TestCycle147_LintObject_NilAndClean(t *testing.T) {
	nilRes := LintObject(nil)
	require.NotNil(t, nilRes)
	assert.True(t, nilRes.Passed)
	assert.Empty(t, nilRes.Violations)

	cleanObj := map[string]any{
		objects.FieldKeyID:          "REQ-CLEAN-001",
		objects.FieldKeyKind:        "requirement",
		objects.FieldKeyTitle:       "High-Throughput Ingestion Engine",
		objects.FieldKeyDescription: "Process 50k events per second with sub-5ms latency and zero dropped payloads.",
		"statement":                 "All records are persisted to disk within transaction bounds.",
		"acceptance_criteria":       "P99 latency remains below 10ms under peak load.",
		"rationale":                 "Eliminates message loss during queue bursts.",
	}
	cleanRes := LintObject(cleanObj)
	require.NotNil(t, cleanRes)
	assert.True(t, cleanRes.Passed)
	assert.Empty(t, cleanRes.Violations)
}

func TestCycle147_LintObject_AllBannedVaguePhrases(t *testing.T) {
	bannedSamples := []string{
		"We must make the code is better before shipping",
		"Please make code better here",
		"Need to make it cleaner across the module",
		"Module cleaner now",
		"Ensure code has changed after run",
		"Verify code changed properly",
		"We should improve the code in this file",
		"We should improve code in this file",
		"Go ahead and fix things promptly",
		"Apply various improvements to the service",
		"This result is good enough for now",
		"The service works fine in staging",
	}

	for _, sample := range bannedSamples {
		obj := map[string]any{
			objects.FieldKeyID:   "REQ-VAGUE-001",
			objects.FieldKeyKind: "requirement",
			"rationale":          sample,
		}
		res := LintObject(obj)
		require.NotNil(t, res)
		assert.False(t, res.Passed, "sample: %s", sample)
		require.NotEmpty(t, res.Violations)
		assert.Equal(t, "banned_vague_phrase", res.Violations[0].Rule)
		assert.Equal(t, "blocker", res.Violations[0].Severity)
	}
}

func TestCycle147_LintRequirementWithCriteria_CriteriaSuperficial(t *testing.T) {
	req := map[string]any{
		objects.FieldKeyID:   "REQ-TEST-001",
		objects.FieldKeyKind: "requirement",
		"title":              "Strict Requirement Specification",
	}

	critWithVague := []map[string]any{
		{
			"id":           "CRIT-1",
			"formula_type": "invariant",
			"title":        "Schema invariant confirmed",
		},
		{
			"id":           "CRIT-2",
			"formula_type": "dynamic",
			"title":        "We will fix things in the controller",
		},
		{
			"id":           "CRIT-3",
			"formula_type": "adversarial",
			"title":        "Reject malformed data fail-closed",
		},
	}

	res := LintRequirementWithCriteria(req, critWithVague)
	require.NotNil(t, res)
	assert.False(t, res.Passed)
	require.NotEmpty(t, res.Violations)
	assert.Equal(t, "banned_vague_phrase", res.Violations[0].Rule)
}

func TestCycle147_LintRequirementWithCriteria_MissingMultipleDimensions(t *testing.T) {
	req := map[string]any{
		objects.FieldKeyID: "REQ-TEST-002",
	}

	critAllDynamic := []map[string]any{
		{"id": "CRIT-1", "formula_type": "dynamic"},
		{"id": "CRIT-2", "formula_type": "dynamic"},
		{"id": "CRIT-3", "formula_type": "dynamic"},
	}

	res := LintRequirementWithCriteria(req, critAllDynamic)
	require.NotNil(t, res)
	assert.False(t, res.Passed)

	err := CheckShovelReadyQuality(req, critAllDynamic)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrThreeFoldFormulaUnsatisfied))
	assert.Contains(t, err.Error(), "missing 3-Fold dimensions")
}
