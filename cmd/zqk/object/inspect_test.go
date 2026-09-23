package object

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestNewInspectCmd_Structure(t *testing.T) {
	cmd := NewInspectCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "inspect", cmd.Use)
	assert.Contains(t, cmd.Aliases, "ins")

	// Verify required flags exist
	assert.NotNil(t, cmd.Flags().Lookup("kind"))
	assert.NotNil(t, cmd.Flags().Lookup("id"))
	assert.NotNil(t, cmd.Flags().Lookup("fields"))
	assert.NotNil(t, cmd.Flags().Lookup("filter"))
	assert.NotNil(t, cmd.Flags().Lookup("sort-by"))
	assert.NotNil(t, cmd.Flags().Lookup("sort-asc"))
	assert.NotNil(t, cmd.Flags().Lookup("group-by"))
	assert.NotNil(t, cmd.Flags().Lookup("policy-studio"))
	assert.NotNil(t, cmd.Flags().Lookup("format"))
}

func TestBuildSemanticProjection_BacklogItem(t *testing.T) {
	rawObj := map[string]any{
		objects.FieldKeyID:       "BLI-TEST-001",
		objects.FieldKeyKind:     objects.KindBacklogItem,
		objects.FieldKeyTitle:    "Test Backlog Item Title",
		objects.FieldKeyStatus:   "in_progress",
		objects.FieldKeyPriority: "high",
		"claimed_by":             "agent-test",
		"requirement_ref":        "REQ-TEST-001",
		"milestone_ref":          "MIL-TEST-001",
		"priority_plan_ref":      "PRI-TEST-001",
		"test_case_refs":         []string{"TST-TEST-001"},
		"criteria_refs":          []string{"CRIT-TEST-001", "CRIT-TEST-002"},
		"custom_field":           "custom_value",
	}

	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"CRIT-TEST-001": {
				objects.FieldKeyID:     "CRIT-TEST-001",
				objects.FieldKeyStatus: "satisfied",
			},
			"CRIT-TEST-002": {
				objects.FieldKeyID:     "CRIT-TEST-002",
				objects.FieldKeyStatus: "originated",
			},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	proj := buildSemanticProjection(ctx, mockStorage, secCtx, rawObj, objects.KindBacklogItem, []string{"custom_field"})

	assert.Equal(t, objects.KindBacklogItem, proj.Kind)
	assert.Equal(t, "BLI-TEST-001", proj.ID)
	assert.Equal(t, "in_progress", proj.Status)
	assert.Equal(t, "high", proj.Priority)
	assert.Equal(t, "Test Backlog Item Title", proj.Title)
	assert.Equal(t, "agent-test", proj.ClaimedBy)

	require.NotNil(t, proj.Lineage)
	assert.Equal(t, "REQ-TEST-001", proj.Lineage.Requirement)
	assert.Equal(t, "MIL-TEST-001", proj.Lineage.Milestone)
	assert.Equal(t, "PRI-TEST-001", proj.Lineage.PriorityPlan)
	assert.Equal(t, []string{"TST-TEST-001"}, proj.Lineage.TestCases)
	assert.True(t, proj.Lineage.IsIntact)

	require.NotNil(t, proj.CriteriaSummary)
	assert.Equal(t, 2, proj.CriteriaSummary.Total)
	assert.Equal(t, 1, proj.CriteriaSummary.Satisfied)
	assert.Equal(t, 1, proj.CriteriaSummary.Pending)

	assert.Contains(t, proj.ActionsAvailable, "transition_status")
	assert.Contains(t, proj.ActionsAvailable, "unclaim")
	assert.Contains(t, proj.ActionsAvailable, "edit_properties")

	require.NotNil(t, proj.RawFields)
	assert.Equal(t, "custom_value", proj.RawFields["custom_field"])
}

func TestMatchesFilterExpr(t *testing.T) {
	obj := map[string]any{
		"status":   "in_progress",
		"priority": "high",
		"tier":     "P1",
	}

	assert.True(t, matchesFilterExpr(obj, []string{"status=in_progress"}))
	assert.False(t, matchesFilterExpr(obj, []string{"status=complete"}))
	assert.True(t, matchesFilterExpr(obj, []string{"status!=complete"}))
	assert.False(t, matchesFilterExpr(obj, []string{"status!=in_progress"}))
	assert.True(t, matchesFilterExpr(obj, []string{"status=in_progress", "priority=high"}))
	assert.False(t, matchesFilterExpr(obj, []string{"status=in_progress", "priority=low"}))
}

func TestInspectSingleObject_JSONFormatting(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"BLI-456": {
				objects.FieldKeyID:     "BLI-456",
				objects.FieldKeyKind:   objects.KindBacklogItem,
				objects.FieldKeyTitle:  "Sample Task",
				objects.FieldKeyStatus: "planned",
			},
		},
	}

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.Flags().String("format", "table", "")
	_ = cmd.Flags().Set("format", "json")

	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)
	cmd.SetContext(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()

	err := inspectSingleObject(cmd, objects.KindBacklogItem, "BLI-456", nil, mockStorage, ctx, secCtx, true)
	require.NoError(t, err)

	var proj SemanticAgentProjection
	err = json.Unmarshal(outBuf.Bytes(), &proj)
	require.NoError(t, err)
	assert.Equal(t, "BLI-456", proj.ID)
	assert.Equal(t, objects.KindBacklogItem, proj.Kind)
	assert.Equal(t, "Sample Task", proj.Title)
	assert.Equal(t, "planned", proj.Status)
}

func TestInspectSingleObject_HumanTDS(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"BLI-789": {
				objects.FieldKeyID:     "BLI-789",
				objects.FieldKeyKind:   objects.KindBacklogItem,
				objects.FieldKeyTitle:  "Human View Task",
				objects.FieldKeyStatus: "testing",
			},
		},
	}

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	err := inspectSingleObject(cmd, objects.KindBacklogItem, "BLI-789", nil, mockStorage, ctx, secCtx, false)
	require.NoError(t, err)

	rendered := outBuf.String()
	assert.Contains(t, rendered, "OBJECT INSPECTOR: BLI-789")
	assert.Contains(t, rendered, "Human View Task")
	assert.Contains(t, rendered, "Kind: backlog_item")
}

func TestRunPolicyStudio_JSON(t *testing.T) {
	mockStorage := &mockInspectStorage{}

	cmd := &cobra.Command{}
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.Flags().String("format", "table", "")
	_ = cmd.Flags().Set("format", "json")

	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &outBuf)
	cmd.SetContext(ctx)
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	err := runPolicyStudio(cmd, objects.KindBacklogItem, mockStorage, ctx, secCtx, storageCtx, true)
	require.NoError(t, err)

	var studio PolicyStudioProjection
	err = json.Unmarshal(outBuf.Bytes(), &studio)
	require.NoError(t, err)
	assert.Equal(t, objects.KindBacklogItem, studio.Kind)
	assert.GreaterOrEqual(t, studio.TotalRules, 3)
	assert.NotEmpty(t, studio.Evaluations)
}

// mockInspectStorage implements minimal storage methods for test isolation
type mockInspectStorage struct {
	storage.ObjectStorageProvider
	objects map[string]map[string]any
}

func (m *mockInspectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objects[id]; ok {
		return obj, nil
	}
	return nil, nil
}

func (m *mockInspectStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *storage.StorageContext, filter storage.ListFilter) (*storage.QueryResult, error) {
	var list []map[string]any
	for _, o := range m.objects {
		if filter.Kind == "" || o[objects.FieldKeyKind] == filter.Kind {
			list = append(list, o)
		}
	}
	return &storage.QueryResult{Objects: list}, nil
}
