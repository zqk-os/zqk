package objects

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestSpecValidator(t *testing.T) *SpecValidator {
	t.Helper()
	t.Parallel()
	return NewSpecValidator(NewSpecLoader(findSpecsDir()), nil)
}

func TestValidateLoadedSpec_NilSpec(t *testing.T) {
	t.Parallel()
	errs := ValidateLoadedSpec(context.Background(), nil, nil, nil)
	assert.Nil(t, errs)
}

func TestSpecValidator_FieldDefVariants(t *testing.T) {
	validator := setupTestSpecValidator(t)

	// 1. Non-map field definition should be skipped without panic
	specNonMap := &Spec{
		Ontology: "test_kind",
		ResolvedFields: map[string]any{
			"bad_field": "not_a_map",
		},
	}
	errsNonMap := validator.ValidateSpec(context.Background(), specNonMap)
	// No panic, skipped
	assert.Empty(t, errsNonMap)

	// 2. Field without checklist map emits missing checklist errors
	specNoChecklist := &Spec{
		Ontology: "test_kind",
		ResolvedFields: map[string]any{
			"field_without_checklist": map[string]any{
				"type": "string",
			},
		},
	}
	errsNoChecklist := validator.ValidateSpec(context.Background(), specNoChecklist)
	require.NotEmpty(t, errsNoChecklist)
	var foundChecklistErr bool
	for _, err := range errsNoChecklist {
		if err.Field == "field_without_checklist" && err.MissingItem == "purpose" {
			foundChecklistErr = true
			break
		}
	}
	assert.True(t, foundChecklistErr)
}

func TestSpecValidator_FieldTraitValidationErrors(t *testing.T) {
	validator := setupTestSpecValidator(t)

	// Field trait not matching known valid traits or invalid context
	specTraitErr := &Spec{
		Ontology:       "test_kind",
		ResolvedTraits: []string{TraitOccupiable},
		ResolvedFields: map[string]any{
			"assigned_agent": map[string]any{
				"type":   "string",
				"traits": []any{"invalid_unknown_trait_xyz"},
				"checklist": map[string]any{
					"purpose": "holds agent id",
				},
			},
		},
	}
	errs := validator.ValidateSpec(context.Background(), specTraitErr)
	require.NotEmpty(t, errs)
	var foundTraitErr bool
	for _, err := range errs {
		if err.Field == "assigned_agent" {
			foundTraitErr = true
			break
		}
	}
	assert.True(t, foundTraitErr)
}

