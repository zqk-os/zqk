package object

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestPolicyStudio_Extended_SuggestDSLTokens(t *testing.T) {
	// Empty input suggests schema fields
	emptySug := SuggestDSLTokens("backlog_item", "")
	assert.NotEmpty(t, emptySug)

	// Logical operator suggests fields
	andSug := SuggestDSLTokens("backlog_item", "status == \"planned\" &&")
	assert.NotEmpty(t, andSug)

	// Field suggests operators
	fieldSug := SuggestDSLTokens("backlog_item", "status")
	assert.NotEmpty(t, fieldSug)

	// Comparison operator suggests values
	opSug := SuggestDSLTokens("backlog_item", "status ==")
	assert.NotEmpty(t, opSug)
}

func TestPolicyStudio_EvaluateDSLExpression_Basic(t *testing.T) {
	obj := map[string]any{
		"id":          "BLI-POL-001",
		"status":      "planned",
		"priority":    2,
		"title":       "Policy Studio Item",
		"empty_field": "",
	}

	// Equality
	passed, err := EvaluateDSLExpression(`status == "planned"`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = EvaluateDSLExpression(`status == "active"`, obj)
	require.NoError(t, err)
	assert.False(t, passed)

	// Inequality
	passed, err = EvaluateDSLExpression(`status != "active"`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	// Numeric comparison
	passed, err = EvaluateDSLExpression(`priority > 1`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = EvaluateDSLExpression(`priority <= 2`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = EvaluateDSLExpression(`priority < 2`, obj)
	require.NoError(t, err)
	assert.False(t, passed)
}

func TestPolicyStudio_EvaluateDSLExpression_Clauses(t *testing.T) {
	obj := map[string]any{
		"id":          "BLI-POL-002",
		"status":      "planned",
		"title":       "Policy Studio Item",
		"empty_field": "",
	}

	// Contains
	passed, err := EvaluateDSLExpression(`title contains "Policy"`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	// Unary predicates: is_populated and is_empty
	passed, err = EvaluateDSLExpression(`title is_populated`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = EvaluateDSLExpression(`empty_field is_empty`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = EvaluateDSLExpression(`title is_not_empty`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	// Compound expressions (&& and ||)
	passed, err = EvaluateDSLExpression(`status == "planned" && title is_populated`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	// Compound expressions with AND
	passed, err = EvaluateDSLExpression(`status == "planned" AND title is_populated`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	// Unsupported single-token clause triggers error
	_, err = EvaluateDSLExpression(`singletoken`, obj)
	assert.Error(t, err)
}

func TestPolicyStudio_EvaluateDSLExpression_Extended(t *testing.T) {
	obj := map[string]any{
		"tags":       []any{"alpha", "beta"},
		"str_tags":   []string{"gamma", "delta"},
		"empty_list": []string{},
		"count":      10,
	}

	// not_contains with string slice and any slice
	passed, err := EvaluateDSLExpression(`tags contains "alpha"`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = EvaluateDSLExpression(`tags not_contains "omega"`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = EvaluateDSLExpression(`str_tags contains "gamma"`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = EvaluateDSLExpression(`str_tags not_contains "omega"`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	// Unary predicates on slices
	passed, err = EvaluateDSLExpression(`tags is_populated`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = EvaluateDSLExpression(`empty_list is_empty`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	passed, err = EvaluateDSLExpression(`empty_list is_not_empty`, obj)
	require.NoError(t, err)
	assert.False(t, passed)

	// Numeric >= comparison
	passed, err = EvaluateDSLExpression(`count >= 10`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	// Inequality on missing field
	passed, err = EvaluateDSLExpression(`missing_field != "something"`, obj)
	require.NoError(t, err)
	assert.True(t, passed)

	// Criteria default rules
	critRules := DefaultPolicyRulesForKind(objects.KindCriteria)
	assert.NotEmpty(t, critRules)
}

func TestPolicyStudio_DefaultRulesAndDryRun(t *testing.T) {
	rules := DefaultPolicyRulesForKind("backlog_item")
	assert.NotEmpty(t, rules)

	unknownRules := DefaultPolicyRulesForKind("non_existent_kind")
	assert.NotNil(t, unknownRules)

	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	ctx := pkgctx.NewSystemContext()
	secCtx := pkgctx.NewSystemSecurityContext()

	obj := map[string]any{
		objects.FieldKeyID:                       "BLI-POL-003",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Dry Run Item",
		objects.FieldKeyDescription:              "Valid description of sufficient length.",
		objects.FieldKeyProblemStatement:         "Valid problem statement.",
		objects.FieldKeyAcceptanceConsiderations: "Valid considerations.",
		objects.FieldKeyPriority:                 "high",
		objects.FieldKeyPriorityTier:             "P1",
		objects.FieldKeyPriorityPlanRef:          "PRI-POL-001",
		objects.FieldKeyStatus:                   "planned",
	}
	err = provider.Create(ctx, secCtx, obj)
	require.NoError(t, err)

	testRules := []PolicyRule{
		{
			ID:         "RULE-01",
			Name:       "Status Must Be Planned",
			TargetKind: objects.KindBacklogItem,
			Expression: `status == "planned"`,
			Severity:   "blocker",
		},
		{
			ID:         "RULE-02",
			Name:       "Status Must Be Active",
			TargetKind: objects.KindBacklogItem,
			Expression: `status == "active"`,
			Severity:   "warning",
		},
	}

	storageCtx := pkgctx.NewStorageContext()
	results, err := RunPolicyStudioDryRun(context.Background(), provider, secCtx, storageCtx, objects.KindBacklogItem, testRules)
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.True(t, results[0].Passed)
	assert.False(t, results[1].Passed)
}
