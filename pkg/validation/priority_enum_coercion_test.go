package validation_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

// TestPriority_EnumCoercionAndParity verifies bidirectional priority normalization,
// coercion, validation, and negative boundary rejection across requirements and backlog items.
// Satisfies:
// - CRIT-1793968598478512000-1638f7af (Static/Doc Invariant: docs/guides/PRIORITY_ENUM_PARITY.md)
// - CRIT-1793968601415990000-9fc4de65 (Operational Dynamic Proof: bidirectional coercion & validation)
// - CRIT-1793968604958619000-c1329de3 (Negative Adversarial Boundary: reject invalid literals with enum options)
func TestPriority_EnumCoercionAndParity(t *testing.T) {
	ctx := context.Background()
	gv := validation.NewGoValidator()

	t.Run("Requirements_WordAliasToPTier_Coercion", func(t *testing.T) {
		tests := []struct {
			input string
			want  string
		}{
			{"critical", "p0"},
			{"Critical", "p0"},
			{"CRITICAL", "p0"},
			{"high", "p1"},
			{"High", "p1"},
			{"HIGH", "p1"},
			{"medium", "p2"},
			{"Medium", "p2"},
			{"MEDIUM", "p2"},
			{"low", "p3"},
			{"Low", "p3"},
			{"LOW", "p3"},
			{"p0", "p0"},
			{"P0", "p0"},
			{"p1", "p1"},
			{"P1", "p1"},
			{"p2", "p2"},
			{"P2", "p2"},
			{"p3", "p3"},
			{"P3", "p3"},
		}

		for _, tc := range tests {
			t.Run(tc.input, func(t *testing.T) {
				req := map[string]any{
					objects.FieldKeyID:       "REQ-1793968565892064000-88993da3",
					objects.FieldKeyKind:     objects.KindRequirement,
					objects.FieldKeyTitle:    "Valid Requirement Title Here",
					objects.FieldKeyStatus:   "originated",
					objects.FieldKeyPriority: tc.input,
				}
				validation.NormalizePriority(objects.KindRequirement, req)
				assert.Equal(t, tc.want, req[objects.FieldKeyPriority], "expected coerced priority %s for input %s", tc.want, tc.input)

				// Also verify via objects.CoerceMutationFields
				req2 := map[string]any{
					objects.FieldKeyID:       "REQ-1793968565892064000-88993da3",
					objects.FieldKeyKind:     objects.KindRequirement,
					objects.FieldKeyTitle:    "Valid Requirement Title Here",
					objects.FieldKeyStatus:   "originated",
					objects.FieldKeyPriority: tc.input,
				}
				objects.CoerceMutationFields(objects.KindRequirement, req2)
				assert.Equal(t, tc.want, req2[objects.FieldKeyPriority], "CoerceMutationFields must coerce %s to %s", tc.input, tc.want)
			})
		}
	})

	t.Run("BacklogItems_PTierToWord_CoercionAndTierParity", func(t *testing.T) {
		tests := []struct {
			inputPri string
			wantPri  string
			wantTier string
		}{
			{"p0", "critical", "P0"},
			{"P0", "critical", "P0"},
			{"critical", "critical", "P0"},
			{"CRITICAL", "critical", "P0"},
			{"p1", "high", "P1"},
			{"P1", "high", "P1"},
			{"high", "high", "P1"},
			{"HIGH", "high", "P1"},
			{"p2", "medium", "P2"},
			{"P2", "medium", "P2"},
			{"medium", "medium", "P2"},
			{"MEDIUM", "medium", "P2"},
			{"p3", "low", "P3"},
			{"P3", "low", "P3"},
			{"low", "low", "P3"},
			{"LOW", "low", "P3"},
		}

		for _, tc := range tests {
			t.Run(tc.inputPri, func(t *testing.T) {
				bli := map[string]any{
					objects.FieldKeyID:       "BLI-1793968900963244000-4b662529",
					objects.FieldKeyKind:     objects.KindBacklogItem,
					objects.FieldKeyTitle:    "Valid Backlog Item Title",
					objects.FieldKeyStatus:   "planned",
					objects.FieldKeyPriority: tc.inputPri,
				}
				validation.NormalizePriority(objects.KindBacklogItem, bli)
				assert.Equal(t, tc.wantPri, bli[objects.FieldKeyPriority], "expected coerced priority %s for input %s", tc.wantPri, tc.inputPri)
				assert.Equal(t, tc.wantTier, bli[objects.FieldKeyPriorityTier], "expected synchronized priority_tier %s for input %s", tc.wantTier, tc.inputPri)

				// Also verify via objects.CoerceMutationFields
				bli2 := map[string]any{
					objects.FieldKeyID:       "BLI-1793968900963244000-4b662529",
					objects.FieldKeyKind:     objects.KindBacklogItem,
					objects.FieldKeyTitle:    "Valid Backlog Item Title",
					objects.FieldKeyStatus:   "planned",
					objects.FieldKeyPriority: tc.inputPri,
				}
				objects.CoerceMutationFields(objects.KindBacklogItem, bli2)
				assert.Equal(t, tc.wantPri, bli2[objects.FieldKeyPriority])
				assert.Equal(t, tc.wantTier, bli2[objects.FieldKeyPriorityTier])
			})
		}
	})

	t.Run("GoValidator_RequirementPriorityMedium_Succeeds", func(t *testing.T) {
		opts := &validation.ValidationOptions{
			ValidateLifecycle: false,
			StrictMode:        false,
		}

		req := map[string]any{
			objects.FieldKeyID:          "REQ-1793968565892064000-88993da3",
			objects.FieldKeyKind:        objects.KindRequirement,
			objects.FieldKeyTitle:       "Valid Requirement Functional Title",
			objects.FieldKeyDescription: "Requirement contract description meeting length floor",
			objects.FieldKeyStatus:      "originated",
			objects.FieldKeyPriority:    "medium",
			objects.FieldKeyCreatedAt:   "2026-11-06T12:00:00Z",
			objects.FieldKeyCreatedBy:   "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:   "2026-11-06T12:00:00Z",
			objects.FieldKeyUpdatedBy:   "ACC-SYSTEM",
		}

		res, err := gv.Validate(ctx, req, objects.KindRequirement, opts)
		require.NoError(t, err)
		assert.True(t, res.IsValid, "requirement with priority=medium must pass validation; errors: %+v", res.Errors)
		assert.Equal(t, "p2", req[objects.FieldKeyPriority], "requirement priority must be coerced to p2")
	})

	t.Run("GoValidator_BacklogItemPriorityP2_Succeeds", func(t *testing.T) {
		opts := &validation.ValidationOptions{
			ValidateLifecycle: false,
			StrictMode:        false,
		}

		bli := map[string]any{
			objects.FieldKeyID:              "BLI-1793968900963244000-4b662529",
			objects.FieldKeyKind:            objects.KindBacklogItem,
			objects.FieldKeyTitle:           "Valid Backlog Item Title For Test",
			objects.FieldKeyDescription:     "Backlog item description meeting length requirements",
			objects.FieldKeyStatus:          "planned",
			objects.FieldKeyPriority:        "p2",
			objects.FieldKeyRequirementRefs: []any{"REQ-1793968565892064000-88993da3"},
			objects.FieldKeyCreatedAt:       "2026-11-06T12:00:00Z",
			objects.FieldKeyCreatedBy:       "ACC-SYSTEM",
			objects.FieldKeyUpdatedAt:       "2026-11-06T12:00:00Z",
			objects.FieldKeyUpdatedBy:       "ACC-SYSTEM",
		}

		res, err := gv.Validate(ctx, bli, objects.KindBacklogItem, opts)
		require.NoError(t, err)
		assert.True(t, res.IsValid, "backlog item with priority=p2 must pass validation; errors: %+v", res.Errors)
		assert.Equal(t, "medium", bli[objects.FieldKeyPriority], "backlog item priority must be coerced to medium")
		assert.Equal(t, "P2", bli[objects.FieldKeyPriorityTier], "backlog item priority_tier must be synchronized to P2")
	})

	t.Run("NegativeBoundary_InvalidPriorityLiterals_RejectedWithEnumList", func(t *testing.T) {
		invalidValues := []string{"urgent", "p4", "p5", "unknown", "highest", "blocker", "none"}

		for _, bad := range invalidValues {
			t.Run(bad, func(t *testing.T) {
				// 1. Direct validation helper
				err := validation.ValidatePriority(objects.KindRequirement, bad)
				require.Error(t, err, "ValidatePriority must reject %s", bad)
				assert.Contains(t, err.Error(), "must be one of", "error message must contain enum options list")
				assert.Contains(t, err.Error(), "critical", "enum list must include valid options")
				assert.Contains(t, err.Error(), "p0", "enum list must include p-tier options")

				// 2. ValidatePriorityAndTier
				errObj := validation.ValidatePriorityAndTier(map[string]any{
					objects.FieldKeyPriority: bad,
				})
				require.Error(t, errObj, "ValidatePriorityAndTier must reject %s", bad)
				assert.Contains(t, errObj.Error(), "must be one of")

				// 3. GoValidator schema validation rejection
				opts := &validation.ValidationOptions{
					ValidateLifecycle: false,
					StrictMode:        false,
				}
				req := map[string]any{
					objects.FieldKeyID:          "REQ-1793968565892064000-88993da3",
					objects.FieldKeyKind:        objects.KindRequirement,
					objects.FieldKeyTitle:       "Valid Requirement Functional Title",
					objects.FieldKeyDescription: "Requirement contract description meeting length floor",
					objects.FieldKeyStatus:      "originated",
					objects.FieldKeyPriority:    bad,
					objects.FieldKeyCreatedAt:   "2026-11-06T12:00:00Z",
					objects.FieldKeyCreatedBy:   "ACC-SYSTEM",
					objects.FieldKeyUpdatedAt:   "2026-11-06T12:00:00Z",
					objects.FieldKeyUpdatedBy:   "ACC-SYSTEM",
				}
				res, valErr := gv.Validate(ctx, req, objects.KindRequirement, opts)
				require.NoError(t, valErr)
				assert.False(t, res.IsValid, "schema validation must fail for invalid priority %s", bad)

				foundEnumError := false
				for _, e := range res.Errors {
					if e.Field == objects.FieldKeyPriority {
						foundEnumError = true
						assert.True(t, strings.Contains(strings.ToLower(e.Message), "must be one of"), "message must list enum options: %s", e.Message)
						break
					}
				}
				assert.True(t, foundEnumError, "expected priority enum validation error for %s", bad)
			})
		}
	})

	t.Run("FieldRestrictions_CaseInsensitiveEnumMatching", func(t *testing.T) {
		reqUpdates := map[string]any{
			objects.FieldKeyPriority: "MEDIUM",
		}
		objects.CoerceMutationFields(objects.KindRequirement, reqUpdates)
		assert.Equal(t, "p2", reqUpdates[objects.FieldKeyPriority])

		bliUpdates := map[string]any{
			objects.FieldKeyPriority: "P2",
		}
		objects.CoerceMutationFields(objects.KindBacklogItem, bliUpdates)
		assert.Equal(t, "medium", bliUpdates[objects.FieldKeyPriority])
		assert.Equal(t, "P2", bliUpdates[objects.FieldKeyPriorityTier])
	})

	t.Run("StorageUpdate_PriorityCoercionAndNegativeBoundary", func(t *testing.T) {
		loader := objects.GetGlobalSpecLoader()
		require.NotNil(t, loader)
		reqSpec, err := loader.LoadSpecWithInheritance("requirement.yaml")
		require.NoError(t, err)
		bliSpec, err := loader.LoadSpecWithInheritance("backlog_item.yaml")
		require.NoError(t, err)

		// 1. Requirement update with priority=medium (coerced and valid)
		reqUpdates := map[string]any{
			objects.FieldKeyPriority: "medium",
		}
		objects.CoerceMutationFields(objects.KindRequirement, reqUpdates)
		assert.Equal(t, "p2", reqUpdates[objects.FieldKeyPriority])
		assert.NoError(t, objects.ValidateObjectFieldRestrictions(reqSpec, objects.KindRequirement, reqUpdates))

		// 2. Backlog item update with priority=p2 (coerced and valid)
		bliUpdates := map[string]any{
			objects.FieldKeyPriority: "p2",
		}
		objects.CoerceMutationFields(objects.KindBacklogItem, bliUpdates)
		assert.Equal(t, "medium", bliUpdates[objects.FieldKeyPriority])
		assert.Equal(t, "P2", bliUpdates[objects.FieldKeyPriorityTier])
		assert.NoError(t, objects.ValidateObjectFieldRestrictions(bliSpec, objects.KindBacklogItem, bliUpdates))

		// 3. Negative boundary on requirement update with priority=urgent
		badReqUpdates := map[string]any{
			objects.FieldKeyPriority: "urgent",
		}
		objects.CoerceMutationFields(objects.KindRequirement, badReqUpdates)
		errReq := objects.ValidateObjectFieldRestrictions(reqSpec, objects.KindRequirement, badReqUpdates)
		require.Error(t, errReq)
		assert.Contains(t, errReq.Error(), "must be one of")
		assert.Contains(t, errReq.Error(), "p0")

		// 4. Negative boundary on backlog item update with priority=unknown
		badBliUpdates := map[string]any{
			objects.FieldKeyPriority: "unknown",
		}
		objects.CoerceMutationFields(objects.KindBacklogItem, badBliUpdates)
		errBli := objects.ValidateObjectFieldRestrictions(bliSpec, objects.KindBacklogItem, badBliUpdates)
		require.Error(t, errBli)
		assert.Contains(t, errBli.Error(), "must be one of")
		assert.Contains(t, errBli.Error(), "critical")
	})
}
