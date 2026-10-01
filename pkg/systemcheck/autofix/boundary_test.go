package autofix

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck"
)

type errorRuleStorage struct {
	storage.ObjectStorageProvider
}

func (e *errorRuleStorage) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	return nil, errors.New("simulated storage failure")
}

func TestAutoFixRuleLoader_NilOrEmptyInputs(t *testing.T) {
	loader := NewAutoFixRuleLoader()

	// Empty project root
	cmd, ok := loader.GetFixCommand("", nil, "backlog_item", "validation", 1, "R1", "msg", "BLI-1", "field", nil)
	assert.False(t, ok)
	assert.Empty(t, cmd)

	// Nil storage provider
	cmd, ok = loader.GetFixCommand("/project", nil, "backlog_item", "validation", 1, "R1", "msg", "BLI-1", "field", nil)
	assert.False(t, ok)
	assert.Empty(t, cmd)

	// InvalidateCache with empty string is a safe no-op
	assert.NotPanics(t, func() {
		loader.InvalidateCache("")
	})
}

func TestAutoFixRuleLoader_StorageError(t *testing.T) {
	loader := NewAutoFixRuleLoader()
	errStore := &errorRuleStorage{}

	rules, err := loader.GetRules("/project", errStore)
	assert.Error(t, err)
	assert.Nil(t, rules)

	cmd, ok := loader.GetFixCommand("/project", errStore, "backlog_item", "validation", 1, "R1", "msg", "BLI-1", "field", nil)
	assert.False(t, ok)
	assert.Empty(t, cmd)
}

func TestAutoFixRuleLoader_DisabledAndMismatchedRules(t *testing.T) {
	mockStore := &mockRuleStorage{
		rules: []map[string]any{
			{
				"applies_to_kind":      "backlog_item",
				"condition_category":  "validation",
				"fix_command_template": "zqk object repair {object_id}",
				"priority":            50,
				"enabled":             false, // disabled
			},
			{
				"applies_to_kind":            "backlog_item",
				"condition_category":        "validation",
				"condition_tier":            2,
				"condition_rule":            "R2",
				"condition_message_contains": "specific error",
				"fix_command_template":       "zqk object fix {object_id}",
				"priority":                  20,
				"enabled":                   true,
			},
		},
	}

	loader := NewAutoFixRuleLoader()

	// Tier mismatch (pass tier 1 instead of 2)
	cmd, ok := loader.GetFixCommand("/p", mockStore, "backlog_item", "validation", 1, "R2", "specific error", "BLI-1", "f", nil)
	assert.False(t, ok)
	assert.Empty(t, cmd)

	// Rule mismatch
	cmd, ok = loader.GetFixCommand("/p", mockStore, "backlog_item", "validation", 2, "OTHER_RULE", "specific error", "BLI-1", "f", nil)
	assert.False(t, ok)
	assert.Empty(t, cmd)

	// Message mismatch
	cmd, ok = loader.GetFixCommand("/p", mockStore, "backlog_item", "validation", 2, "R2", "different error", "BLI-1", "f", nil)
	assert.False(t, ok)
	assert.Empty(t, cmd)

	// Kind mismatch
	cmd, ok = loader.GetFixCommand("/p", mockStore, "milestone", "validation", 2, "R2", "specific error", "BLI-1", "f", nil)
	assert.False(t, ok)
	assert.Empty(t, cmd)
}

func TestSortIssuesByDependency_EdgeCases(t *testing.T) {
	// Empty slice
	assert.Empty(t, SortIssuesByDependency(nil))
	assert.Empty(t, SortIssuesByDependency([]systemcheck.Issue{}))

	// Single issue
	single := []systemcheck.Issue{{Message: "plain message without colon"}}
	sorted := SortIssuesByDependency(single)
	require.Len(t, sorted, 1)
	assert.Equal(t, "plain message without colon", sorted[0].Message)

	// Malformed fix command with --field but missing separator
	malformed := []systemcheck.Issue{
		{
			Message:    "syntax_error: invalid",
			FixCommand: "zqk object update ID --field goal_refs",
		},
	}
	sorted = SortIssuesByDependency(malformed)
	require.Len(t, sorted, 1)
}
