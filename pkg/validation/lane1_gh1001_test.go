package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestGH1001_PriorityPlanAcceptsRequirementRefsAlias tests that priority_plan schema
// validation accepts requirement_refs as a valid alias for related_object_refs under strict mode.
func TestGH1001_PriorityPlanAcceptsRequirementRefsAlias(t *testing.T) {
	gv := NewGoValidator()
	ctx := context.Background()

	now := "2026-11-06T00:00:00Z"
	actor := "ACC-TEST-USER"

	// 1. Priority plan with requirement_refs only (legacy / alias)
	planWithReqRefs := map[string]any{
		objects.FieldKeyID:              "PRI-TEST-001",
		objects.FieldKeyKind:            objects.KindPriorityPlan,
		objects.FieldKeyTitle:           "Legacy Priority Plan",
		objects.FieldKeyDescription:     "Plan using legacy requirement_refs",
		objects.FieldKeyStatus:          objects.ObjectStatusGrooming,
		objects.FieldKeyCreatedAt:       now,
		objects.FieldKeyCreatedBy:       actor,
		objects.FieldKeyUpdatedAt:       now,
		objects.FieldKeyUpdatedBy:       actor,
		objects.FieldKeyRequirementRefs: []any{"REQ-1793942598159999000-1f95def0"},
	}

	opts := &ValidationOptions{
		StrictMode:        true,
		ValidateLifecycle: false,
	}

	res, err := gv.Validate(ctx, planWithReqRefs, objects.KindPriorityPlan, opts)
	require.NoError(t, err)
	require.True(t, res.IsValid, "plan with requirement_refs alias must be valid; got errors: %+v", res.Errors)

	// 2. Priority plan with both requirement_refs and related_object_refs pointing to the same REQ
	// should NOT trigger duplicate reference error
	planWithBoth := map[string]any{
		objects.FieldKeyID:                "PRI-TEST-002",
		objects.FieldKeyKind:              objects.KindPriorityPlan,
		objects.FieldKeyTitle:             "Dual Reference Priority Plan",
		objects.FieldKeyDescription:       "Plan using both fields for compatibility",
		objects.FieldKeyStatus:            objects.ObjectStatusGrooming,
		objects.FieldKeyCreatedAt:         now,
		objects.FieldKeyCreatedBy:         actor,
		objects.FieldKeyUpdatedAt:         now,
		objects.FieldKeyUpdatedBy:         actor,
		objects.FieldKeyRequirementRefs:   []any{"REQ-1793942598159999000-1f95def0"},
		objects.FieldKeyRelatedObjectRefs: []any{"REQ-1793942598159999000-1f95def0"},
	}

	resBoth, errBoth := gv.Validate(ctx, planWithBoth, objects.KindPriorityPlan, opts)
	require.NoError(t, errBoth)
	require.True(t, resBoth.IsValid, "plan with both requirement_refs and related_object_refs alias must not fail duplicates; errors: %+v", resBoth.Errors)

	// 3. Invalid requirement_refs type (e.g. integer) must fail validation
	planWithInvalidType := map[string]any{
		objects.FieldKeyID:              "PRI-TEST-003",
		objects.FieldKeyKind:            objects.KindPriorityPlan,
		objects.FieldKeyTitle:           "Invalid Field Type Priority Plan",
		objects.FieldKeyDescription:     "Plan with integer requirement_refs",
		objects.FieldKeyStatus:          objects.ObjectStatusGrooming,
		objects.FieldKeyCreatedAt:       now,
		objects.FieldKeyCreatedBy:       actor,
		objects.FieldKeyUpdatedAt:       now,
		objects.FieldKeyUpdatedBy:       actor,
		objects.FieldKeyRequirementRefs: 12345,
	}

	resInvalid, errInvalid := gv.Validate(ctx, planWithInvalidType, objects.KindPriorityPlan, opts)
	require.NoError(t, errInvalid)
	require.False(t, resInvalid.IsValid, "plan with non-list/non-string requirement_refs must fail validation")
}
