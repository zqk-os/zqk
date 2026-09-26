package objects

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractFieldRestrictions_FallbackCriteriaCategory(t *testing.T) {
	restr := ExtractFieldRestrictions(nil, KindCriteria, FieldKeyCategory)
	require.NotNil(t, restr)
	assert.Equal(t, FieldKeyCategory, restr.FieldName)
	assert.ElementsMatch(t, ValidCriteriaCategories, restr.EnumValues)

	// Valid category
	err := restr.ValidateFieldValue("acceptance")
	assert.NoError(t, err)

	// Invalid category
	err = restr.ValidateFieldValue("invalid_cat")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `field "category" has invalid value "invalid_cat"`)
	assert.Contains(t, err.Error(), "acceptance")
}

func TestExtractFieldRestrictions_FallbackPriorityTier(t *testing.T) {
	restr := ExtractFieldRestrictions(nil, KindBacklogItem, FieldKeyPriorityTier)
	require.NotNil(t, restr)
	assert.Equal(t, FieldKeyPriorityTier, restr.FieldName)
	assert.ElementsMatch(t, ValidPriorityTiers, restr.EnumValues)

	// Valid priority tier
	err := restr.ValidateFieldValue("P1")
	assert.NoError(t, err)

	// Invalid priority tier
	err = restr.ValidateFieldValue("P99")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `field "priority_tier" has invalid value "P99"`)
}

func TestExtractFieldRestrictions_FromSpecDefinition(t *testing.T) {
	spec := &Spec{
		Ontology: "custom_kind",
		ResolvedFields: map[string]any{
			"status_mode": map[string]any{
				"validation": map[string]any{
					"enum": []any{"manual", "automated", "hybrid"},
				},
			},
			"pattern_field": map[string]any{
				"validation": map[string]any{
					"pattern": `^[a-z]+-[0-9]+$`,
				},
			},
			"bounded_num": map[string]any{
				"validation": map[string]any{
					"min": 10,
					"max": 100,
				},
			},
		},
	}

	// 1. Enum validation
	restr := ExtractFieldRestrictions(spec, "custom_kind", "status_mode")
	require.NotNil(t, restr)
	assert.Equal(t, []string{"manual", "automated", "hybrid"}, restr.EnumValues)
	assert.NoError(t, restr.ValidateFieldValue("automated"))
	err := restr.ValidateFieldValue("unknown")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status_mode")
	assert.Contains(t, err.Error(), "unknown")

	// 2. Pattern validation
	restrPat := ExtractFieldRestrictions(spec, "custom_kind", "pattern_field")
	require.NotNil(t, restrPat)
	assert.NoError(t, restrPat.ValidateFieldValue("abc-123"))
	errPat := restrPat.ValidateFieldValue("ABC_123")
	assert.Error(t, errPat)
	assert.Contains(t, errPat.Error(), "pattern_field")

	// 3. Numeric bounds validation
	restrNum := ExtractFieldRestrictions(spec, "custom_kind", "bounded_num")
	require.NotNil(t, restrNum)
	assert.NoError(t, restrNum.ValidateFieldValue(50))
	errLow := restrNum.ValidateFieldValue(5)
	assert.Error(t, errLow)
	assert.Contains(t, errLow.Error(), "less than minimum allowed")
	errHigh := restrNum.ValidateFieldValue(150)
	assert.Error(t, errHigh)
	assert.Contains(t, errHigh.Error(), "greater than maximum allowed")
}

func TestValidateObjectFieldRestrictions(t *testing.T) {
	spec := &Spec{
		Ontology: "criteria",
		ResolvedFields: map[string]any{
			"category": map[string]any{
				"validation": map[string]any{
					"enum": []any{"functional", "non-functional", "acceptance", "test", "performance", "security", "compliance"},
				},
			},
		},
	}

	// Valid updates
	updates := map[string]any{
		"category": "functional",
		"title":    "Some title",
	}
	require.NoError(t, ValidateObjectFieldRestrictions(spec, "criteria", updates))

	// Invalid update
	badUpdates := map[string]any{
		"category": "invalid_cat",
	}
	err := ValidateObjectFieldRestrictions(spec, "criteria", badUpdates)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `field "category" has invalid value "invalid_cat"`)
	assert.Contains(t, err.Error(), "functional, non-functional, acceptance, test, performance, security, compliance")
}
