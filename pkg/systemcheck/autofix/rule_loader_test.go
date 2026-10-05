package autofix

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

type mockRuleStorage struct {
	storage.ObjectStorageProvider
	rules []map[string]any
}

func (m *mockRuleStorage) List(ctx context.Context, secCtx *storage.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	if filter.Kind == objects.KindAutoFixRule {
		return &storage.QueryResult{
			Objects: m.rules,
		}, nil
	}
	return &storage.QueryResult{}, nil
}

func TestSubstituteFixCommandPlaceholders(t *testing.T) {
	template := "zqk object update {object_id} --kind {kind} --field {field} --msg {message} --rule {rule} --tier {tier} --cat {category}"
	result := SubstituteFixCommandPlaceholders(template, "backlog_item", "validation", 1, "CRI-SHOVEL-READY", "missing persona", "BLI-100", "persona")
	expected := "zqk object update BLI-100 --kind backlog_item --field persona --msg missing persona --rule CRI-SHOVEL-READY --tier 1 --cat validation"
	assert.Equal(t, expected, result)
}

func TestAutoFixRuleLoader_GetFixCommand(t *testing.T) {
	mockStore := &mockRuleStorage{
		rules: []map[string]any{
			{
				"applies_to_kind":            "backlog_item",
				"condition_category":         "validation",
				"condition_tier":             1,
				"condition_rule":             "CRI-SHOVEL-READY",
				"condition_message_contains": "missing persona",
				"fix_command_template":       "zqk object update {object_id} --field persona=PER-COMMUNITY",
				"priority":                   10,
				"enabled":                    true,
			},
			{
				"applies_to_kind":      "backlog_item",
				"condition_category":   "validation",
				"fix_command_template": "zqk object repair {object_id}",
				"priority":             50,
				"enabled":              true,
			},
		},
	}

	loader := NewAutoFixRuleLoader()
	cmd, ok := loader.GetFixCommand(
		"/test/project",
		mockStore,
		"backlog_item",
		"validation",
		1,
		"CRI-SHOVEL-READY",
		"missing persona in graph",
		"BLI-001",
		"persona",
		nil,
	)

	require.True(t, ok)
	assert.Equal(t, "zqk object update BLI-001 --field persona=PER-COMMUNITY", cmd)

	// Test cache hit
	rules, err := loader.GetRules("/test/project", mockStore)
	require.NoError(t, err)
	assert.Len(t, rules, 2)

	// Invalidate cache
	loader.InvalidateCache("/test/project")
	loader.mu.RLock()
	_, exists := loader.cache["/test/project"]
	loader.mu.RUnlock()
	assert.False(t, exists)
}

func TestGetAutoFixRuleLoader_Singleton(t *testing.T) {
	l1 := GetAutoFixRuleLoader()
	l2 := GetAutoFixRuleLoader()
	assert.Same(t, l1, l2)
}
