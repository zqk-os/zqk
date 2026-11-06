package scaffold

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// TestScaffold_ReferenceFieldOmission satisfies:
// 1. CRIT-1793968586969889000-ed911957 (Static/Doc Invariant): docs/guides/OBJECT_SCAFFOLDING_HYGIENE.md
// 2. CRIT-1793968589438914000-1a1e8231 (Operational Dynamic Proof): Optional reference fields omitted or [] in scaffolding
// 3. CRIT-1793968592146590000-f6148ecf (Negative Adversarial Boundary): Strict rejection of empty string references
func TestScaffold_ReferenceFieldOmission(t *testing.T) {
	t.Run("CRIT_DocumentationInvariant", func(t *testing.T) {
		root := paths.FindNearestProjectRoot(".")
		require.NotEmpty(t, root, "project root must be found")

		guidePath := filepath.Join(root, "docs", "guides", "OBJECT_SCAFFOLDING_HYGIENE.md")
		data, err := fileutil.ReadFile(guidePath)
		require.NoError(t, err, "docs/guides/OBJECT_SCAFFOLDING_HYGIENE.md must exist")

		content := string(data)
		require.Contains(t, content, "Object Scaffolding Hygiene", "must contain title")
		require.Contains(t, content, "POL-DOC-001", "must cite documentation invariant")
		require.Contains(t, content, "team_configuration_ref", "must mention team_configuration_ref invariant")
		require.Contains(t, content, "release_ref", "must mention release_ref invariant")
		require.Contains(t, content, "reference_not_empty", "must document validation rule")

		readmePath := filepath.Join(root, "docs", "guides", "README.md")
		readmeData, err := fileutil.ReadFile(readmePath)
		require.NoError(t, err, "docs/guides/README.md must exist")
		require.Contains(t, string(readmeData), "OBJECT_SCAFFOLDING_HYGIENE.md", "README must index the guide")
	})

	t.Run("CRIT_NegativeValidation_RejectsEmptyScalarReference", func(t *testing.T) {
		gv := validation.NewGoValidator()
		ctx := context.Background()
		opts := validation.DefaultValidationOptions()

		// Test A: priority_plan with empty release_ref
		planWithEmptyRelease := map[string]any{
			objects.FieldKeyID:            "PRI-1793968515974431000-test001",
			objects.FieldKeyKind:          objects.KindPriorityPlan,
			objects.FieldKeyTitle:         "Test Plan with empty release_ref",
			objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			"release_ref":                 "",
		}

		res, err := gv.Validate(ctx, planWithEmptyRelease, objects.KindPriorityPlan, opts)
		require.NoError(t, err)
		require.False(t, res.IsValid, "validation must fail for empty scalar release_ref")
		found := false
		for _, e := range res.Errors {
			if e.Field == "release_ref" && e.Rule == "reference_not_empty" {
				found = true
				break
			}
		}
		require.True(t, found, "must emit ValidationError with rule reference_not_empty for release_ref")

		// Test B: priority_plan with empty team_configuration_ref
		planWithEmptyTeamConfig := map[string]any{
			objects.FieldKeyID:            "PRI-1793968515974431000-test002",
			objects.FieldKeyKind:          objects.KindPriorityPlan,
			objects.FieldKeyTitle:         "Test Plan with empty team_configuration_ref",
			objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			"team_configuration_ref":      "",
		}

		res, err = gv.Validate(ctx, planWithEmptyTeamConfig, objects.KindPriorityPlan, opts)
		require.NoError(t, err)
		require.False(t, res.IsValid, "validation must fail for empty scalar team_configuration_ref")
		found = false
		for _, e := range res.Errors {
			if e.Field == "team_configuration_ref" && (e.Rule == "reference_not_empty" || e.Rule == "datatype") {
				found = true
				break
			}
		}
		require.True(t, found, "must emit ValidationError for team_configuration_ref")

		// Test C: backlog_item with whitespace-only milestone_ref
		bliWithWhitespaceRef := map[string]any{
			objects.FieldKeyID:            "BLI-1793968515974431000-test003",
			objects.FieldKeyKind:          objects.KindBacklogItem,
			objects.FieldKeyTitle:         "Test BLI with whitespace ref",
			objects.FieldKeyStatus:        objects.ObjectStatusConceptual,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			"milestone_ref":               "   ",
		}

		res, err = gv.Validate(ctx, bliWithWhitespaceRef, objects.KindBacklogItem, opts)
		require.NoError(t, err)
		require.False(t, res.IsValid, "validation must fail for whitespace scalar reference")
		found = false
		for _, e := range res.Errors {
			if e.Field == "milestone_ref" && e.Rule == "reference_not_empty" {
				found = true
				break
			}
		}
		require.True(t, found, "must emit ValidationError for whitespace milestone_ref")
	})

	t.Run("CRIT_NegativeValidation_RejectsEmptySliceReferenceEntries", func(t *testing.T) {
		gv := validation.NewGoValidator()
		ctx := context.Background()
		opts := validation.DefaultValidationOptions()

		// Test A: requirement with criteria_refs containing empty string
		reqWithEmptyCritRef := map[string]any{
			objects.FieldKeyID:            "REQ-1793968515974431000-test004",
			objects.FieldKeyKind:          objects.KindRequirement,
			objects.FieldKeyTitle:         "Test REQ with empty crit entry",
			objects.FieldKeyStatus:        "originated",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			"criteria_refs":               []string{""},
		}

		res, err := gv.Validate(ctx, reqWithEmptyCritRef, objects.KindRequirement, opts)
		require.NoError(t, err)
		require.False(t, res.IsValid, "validation must fail for slice containing empty string reference")
		found := false
		for _, e := range res.Errors {
			if e.Field == "criteria_refs" && e.Rule == "reference_not_empty" {
				found = true
				break
			}
		}
		require.True(t, found, "must emit ValidationError for criteria_refs empty entry")

		// Test B: slice containing both valid and empty string reference
		reqWithMixedRefs := map[string]any{
			objects.FieldKeyID:            "REQ-1793968515974431000-test005",
			objects.FieldKeyKind:          objects.KindRequirement,
			objects.FieldKeyTitle:         "Test REQ with mixed crit entries",
			objects.FieldKeyStatus:        "originated",
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			"criteria_refs":               []any{"CRIT-VALID-001", "   "},
		}

		res, err = gv.Validate(ctx, reqWithMixedRefs, objects.KindRequirement, opts)
		require.NoError(t, err)
		require.False(t, res.IsValid, "validation must fail for mixed slice containing empty entry")
		found = false
		for _, e := range res.Errors {
			if e.Field == "criteria_refs" && e.Rule == "reference_not_empty" {
				found = true
				break
			}
		}
		require.True(t, found, "must emit ValidationError for criteria_refs mixed empty entry")
	})

	t.Run("CRIT_PermitsOmittedOrEmptyArrayReferences", func(t *testing.T) {
		gv := validation.NewGoValidator()
		ctx := context.Background()
		opts := validation.DefaultValidationOptions()

		// Test A: priority_plan with omitted release_ref and team_configuration_ref
		cleanPlan := map[string]any{
			objects.FieldKeyID:             "PRI-1793968515974431000-test006",
			objects.FieldKeyKind:           objects.KindPriorityPlan,
			objects.FieldKeyTitle:          "Test Clean Plan",
			objects.FieldKeyStatus:         objects.ObjectStatusConceptual,
			objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
			objects.FieldKeyWorkstreamRefs: []string{"WS-001"},
		}

		res, err := gv.Validate(ctx, cleanPlan, objects.KindPriorityPlan, opts)
		require.NoError(t, err)
		for _, e := range res.Errors {
			require.NotEqual(t, "reference_not_empty", e.Rule, "no reference_not_empty error for clean plan: %v", e)
		}

		// Test B: priority_plan with explicit null (nil) reference
		planWithNil := map[string]any{
			objects.FieldKeyID:             "PRI-1793968515974431000-test007",
			objects.FieldKeyKind:           objects.KindPriorityPlan,
			objects.FieldKeyTitle:          "Test Plan with nil ref",
			objects.FieldKeyStatus:         objects.ObjectStatusConceptual,
			objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
			objects.FieldKeyWorkstreamRefs: []string{"WS-001"},
			"release_ref":                  nil,
			"team_configuration_ref":       nil,
		}

		res, err = gv.Validate(ctx, planWithNil, objects.KindPriorityPlan, opts)
		require.NoError(t, err)
		for _, e := range res.Errors {
			require.NotEqual(t, "reference_not_empty", e.Rule, "no reference_not_empty error for nil ref: %v", e)
		}

		// Test C: empty array slice reference is valid
		planWithEmptySlice := map[string]any{
			objects.FieldKeyID:             "PRI-1793968515974431000-test008",
			objects.FieldKeyKind:           objects.KindPriorityPlan,
			objects.FieldKeyTitle:          "Test Plan with empty slice",
			objects.FieldKeyStatus:         objects.ObjectStatusConceptual,
			objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
			objects.FieldKeyWorkstreamRefs: []string{"WS-001"},
			"persona_refs":                 []string{},
		}

		res, err = gv.Validate(ctx, planWithEmptySlice, objects.KindPriorityPlan, opts)
		require.NoError(t, err)
		for _, e := range res.Errors {
			require.NotEqual(t, "reference_not_empty", e.Rule, "no reference_not_empty error for empty array: %v", e)
		}
	})
}
