package mutation_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/mutation"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestPreflight_ValueRestrictedSpecFields(t *testing.T) {
	ctx := context.Background()
	validator := mutation.NewDefaultPreflightValidator()

	t.Run("rejects_invalid_criteria_category_mutation", func(t *testing.T) {
		mut := mutation.Mutation{
			Action:     mutation.ActionUpdateNode,
			TargetKind: "criteria",
			TargetID:   "CRIT-TEST-001",
			Fields: map[string]any{
				"category": "invalid_cat",
			},
		}

		receipt, err := validator.Validate(ctx, &mut)
		require.NoError(t, err)
		require.False(t, receipt.Valid, "expected mutation with invalid criteria category to be rejected")
		assert.Equal(t, "rejected_failclosed", receipt.Disposition)

		var matched *mutation.SchemaViolation
		for i := range receipt.Violations {
			if receipt.Violations[i].FieldPath == "fields.category" && receipt.Violations[i].FailingConstraint == "enum_membership" {
				matched = &receipt.Violations[i]
				break
			}
		}
		require.NotNil(t, matched, "expected enum_membership violation for fields.category")
		assert.Contains(t, matched.Expected, "functional")
		assert.Contains(t, matched.Expected, "acceptance")
		assert.Equal(t, "invalid_cat", matched.Actual)
	})

	t.Run("accepts_valid_criteria_category_mutation", func(t *testing.T) {
		for _, validCat := range objects.ValidCriteriaCategories {
			mut := mutation.Mutation{
				Action:     mutation.ActionUpdateNode,
				TargetKind: "criteria",
				TargetID:   "CRIT-TEST-001",
				Fields: map[string]any{
					"category": validCat,
				},
			}

			receipt, err := validator.Validate(ctx, &mut)
			require.NoError(t, err)
			assert.True(t, receipt.Valid, "expected valid category %q to be accepted", validCat)
		}
	})

	t.Run("rejects_invalid_priority_tier_mutation", func(t *testing.T) {
		mut := mutation.Mutation{
			Action:     mutation.ActionUpdateNode,
			TargetKind: "priority_plan",
			TargetID:   "PRI-TEST-001",
			Fields: map[string]any{
				"priority_tier": "P99",
			},
		}

		receipt, err := validator.Validate(ctx, &mut)
		require.NoError(t, err)
		require.False(t, receipt.Valid)
		assert.Equal(t, "rejected_failclosed", receipt.Disposition)

		var matched *mutation.SchemaViolation
		for i := range receipt.Violations {
			if receipt.Violations[i].FieldPath == "fields.priority_tier" && receipt.Violations[i].FailingConstraint == "enum_membership" {
				matched = &receipt.Violations[i]
				break
			}
		}
		require.NotNil(t, matched)
		assert.Contains(t, matched.Expected, "P0")
		assert.Equal(t, "P99", matched.Actual)
	})

	t.Run("accepts_valid_priority_tier_mutation", func(t *testing.T) {
		for _, tier := range []string{"P0", "P1", "P2", "P3", "P4", "P5"} {
			mut := mutation.Mutation{
				Action:     mutation.ActionUpdateNode,
				TargetKind: "priority_plan",
				TargetID:   "PRI-TEST-001",
				Fields: map[string]any{
					"priority_tier": tier,
				},
			}

			receipt, err := validator.Validate(ctx, &mut)
			require.NoError(t, err)
			assert.True(t, receipt.Valid, "expected valid priority tier %q to be accepted", tier)
		}
	})
}
