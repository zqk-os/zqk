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

func TestInspectTUIModel_NavigationAndDrillDown(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"BLI-001": {
				objects.FieldKeyID:       "BLI-001",
				objects.FieldKeyKind:     objects.KindBacklogItem,
				objects.FieldKeyTitle:    "TUI View Integration",
				objects.FieldKeyStatus:   "in_progress",
				objects.FieldKeyPriority: "P0",
				"claimed_by":             "agent-alpha",
				"requirement_ref":        "REQ-001",
				"milestone_ref":          "MIL-001",
				"updated_at":             "2026-09-23T12:00:00Z",
			},
			"BLI-002": {
				objects.FieldKeyID:       "BLI-002",
				objects.FieldKeyKind:     objects.KindBacklogItem,
				objects.FieldKeyTitle:    "CAS Optimization",
				objects.FieldKeyStatus:   "planned",
				objects.FieldKeyPriority: "P1",
				"claimed_by":             "",
				"updated_at":             "2026-09-23T11:00:00Z",
			},
			"BLI-003": {
				objects.FieldKeyID:       "BLI-003",
				objects.FieldKeyKind:     objects.KindBacklogItem,
				objects.FieldKeyTitle:    "Deadlock Fix",
				objects.FieldKeyStatus:   "complete",
				objects.FieldKeyPriority: "P2",
				"claimed_by":             "agent-beta",
				"updated_at":             "2026-09-23T10:00:00Z",
			},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	model := NewInspectTUIModel(ctx, objects.KindBacklogItem, nil, nil, "updated_at", false, mockStorage, secCtx, storageCtx)
	require.NotNil(t, model)
	assert.Equal(t, objects.KindBacklogItem, model.ActiveKind)
	assert.Equal(t, 3, len(model.VisibleProjections))
	assert.Equal(t, 0, model.SelectedIndex)

	// 1. Test Key Down ('j') and Key Up ('k')
	model.HandleInput([]byte{'j'})
	assert.Equal(t, 1, model.SelectedIndex)
	model.HandleInput([]byte{'k'})
	assert.Equal(t, 0, model.SelectedIndex)

	// 2. Test Quick Filter Cycling ('f')
	model.HandleInput([]byte{'f'}) // "active"
	assert.Equal(t, "active", model.FilterPill)
	// BLI-001 (in_progress) and BLI-002 (planned) are active; BLI-003 (complete) is excluded
	assert.Equal(t, 2, len(model.VisibleProjections))

	model.HandleInput([]byte{'f'}) // "draft"
	assert.Equal(t, "draft", model.FilterPill)

	model.HandleInput([]byte{'f'}) // "blocked"
	assert.Equal(t, "blocked", model.FilterPill)

	model.HandleInput([]byte{'f'}) // "complete"
	assert.Equal(t, "complete", model.FilterPill)
	assert.Equal(t, 1, len(model.VisibleProjections))
	assert.Equal(t, "BLI-003", model.VisibleProjections[0].ID)

	// Reset filter back to "all"
	for model.FilterPill != "all" {
		model.HandleInput([]byte{'f'})
	}
	assert.Equal(t, 3, len(model.VisibleProjections))

	// 3. Test Inline Search ('/')
	model.HandleInput([]byte{'/'})
	assert.True(t, model.IsSearching)
	model.HandleInput([]byte{'C'})
	model.HandleInput([]byte{'A'})
	model.HandleInput([]byte{'S'})
	assert.Equal(t, 1, len(model.VisibleProjections))
	assert.Equal(t, "BLI-002", model.VisibleProjections[0].ID)
	// Confirm search with Enter
	model.HandleInput([]byte{13})
	assert.False(t, model.IsSearching)
	assert.Equal(t, "CAS", model.SearchQuery)

	// Clear search
	model.HandleInput([]byte{'/'})
	model.HandleInput([]byte{27}) // Cancel search with Esc
	assert.False(t, model.IsSearching)

	// Reset search query manually for further tests
	model.SearchQuery = ""
	model.RefreshObjects()
	assert.Equal(t, 3, len(model.VisibleProjections))

	// 4. Test Drill-Down Detail Modal (Enter)
	model.SelectedIndex = 0
	model.HandleInput([]byte{13})
	assert.True(t, model.DetailModalOpen)

	// Verify rendered output contains deep inspection panel
	rendered := model.Render()
	assert.Contains(t, rendered, "DEEP OBJECT INSPECTION: BLI-001")
	assert.Contains(t, rendered, "End-to-End Lineage Hierarchy")
	assert.Contains(t, rendered, "REQ-001")

	// Dismiss detail modal with Esc
	model.HandleInput([]byte{27})
	assert.False(t, model.DetailModalOpen)

	// 5. Test Policy Studio Toggle ('p')
	model.HandleInput([]byte{'p'})
	assert.True(t, model.PolicyStudioOpen)
	renderedStudio := model.Render()
	assert.Contains(t, renderedStudio, "LIVE POLICY RULE STUDIO: BACKLOG_ITEM")
	model.HandleInput([]byte{27})
	assert.False(t, model.PolicyStudioOpen)

	// 6. Test Kind Cycling ('Tab')
	initialKind := model.ActiveKind
	model.HandleInput([]byte{9}) // Tab
	assert.NotEqual(t, initialKind, model.ActiveKind)
}

func TestInspectTUIModel_VimNav_gG_TopBottom(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"BLI-001": {objects.FieldKeyID: "BLI-001", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeyTitle: "Alpha"},
			"BLI-002": {objects.FieldKeyID: "BLI-002", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeyTitle: "Beta"},
			"BLI-003": {objects.FieldKeyID: "BLI-003", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeyTitle: "Gamma"},
			"BLI-004": {objects.FieldKeyID: "BLI-004", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeyTitle: "Delta"},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	model := NewInspectTUIModel(ctx, objects.KindBacklogItem, nil, nil, "id", true, mockStorage, secCtx, storageCtx)
	require.NotNil(t, model)
	assert.Equal(t, 4, len(model.VisibleProjections))

	// Start at 0, jump to bottom with 'G' (big gee)
	model.SelectedIndex = 0
	model.HandleInput([]byte{'G'})
	assert.Equal(t, 3, model.SelectedIndex, "G should jump to bottom")

	// Jump to top with 'g' (little gee)
	model.HandleInput([]byte{'g'})
	assert.Equal(t, 0, model.SelectedIndex, "g should jump to top")

	// Toggle g -> G when already at top
	model.HandleInput([]byte{'g'})
	assert.Equal(t, 3, model.SelectedIndex, "g when at top should toggle to bottom (g->G)")

	// Toggle G -> g when already at bottom
	model.HandleInput([]byte{'G'})
	assert.Equal(t, 0, model.SelectedIndex, "G when at bottom should toggle to top (G->g)")

	// Test n/N cycling
	model.HandleInput([]byte{'n'})
	assert.Equal(t, 1, model.SelectedIndex, "n should cycle forward")
	model.HandleInput([]byte{'N'})
	assert.Equal(t, 0, model.SelectedIndex, "N should cycle backward")
}

func TestInspectTUIModel_EditorProfiles_NewbProJedi(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"BLI-001": {objects.FieldKeyID: "BLI-001", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeyTitle: "Alpha"},
			"BLI-002": {objects.FieldKeyID: "BLI-002", objects.FieldKeyKind: objects.KindBacklogItem, objects.FieldKeyTitle: "Beta"},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	model := NewInspectTUIModel(ctx, objects.KindBacklogItem, nil, nil, "id", true, mockStorage, secCtx, storageCtx)
	require.NotNil(t, model)
	assert.Equal(t, "newb", model.EditorProfile)

	// 1. Cycle to Pro
	model.CycleEditorProfile()
	assert.Equal(t, "pro", model.EditorProfile)
	assert.Contains(t, model.StatusMessage, "PRO")

	// 2. Cycle to Jedi
	model.CycleEditorProfile()
	assert.Equal(t, "jedi", model.EditorProfile)
	assert.Contains(t, model.StatusMessage, "JEDI")

	// 3. Cycle to Newb
	model.CycleEditorProfile()
	assert.Equal(t, "newb", model.EditorProfile)
	assert.Contains(t, model.StatusMessage, "NEWB")

	// 4. Test keypresses 'z', 'Z', '?'
	model.HandleInput([]byte{'z'})
	assert.Equal(t, "pro", model.EditorProfile)

	model.HandleInput([]byte{'Z'})
	assert.Equal(t, "jedi", model.EditorProfile)

	model.HandleInput([]byte{'?'})
	assert.Equal(t, "newb", model.EditorProfile)

	// 5. Test SetEditorProfile
	model.SetEditorProfile("pro")
	assert.Equal(t, "pro", model.EditorProfile)
	model.SetEditorProfile("jedi")
	assert.Equal(t, "jedi", model.EditorProfile)
	model.SetEditorProfile("invalid")
	assert.Equal(t, "newb", model.EditorProfile)

	// 6. Test Render in Newb Mode
	model.SetEditorProfile("newb")
	model.Width = 100
	model.Height = 30
	renderedNewb := model.Render()
	assert.Contains(t, renderedNewb, "ZQK OBJECT INSPECTOR")
	assert.Contains(t, renderedNewb, "INSPECTED OBJECT:")
	assert.Contains(t, renderedNewb, "Profile (newb)")

	// 7. Test Render in Pro Mode
	model.SetEditorProfile("pro")
	renderedPro := model.Render()
	assert.Contains(t, renderedPro, "[PRO]")
	assert.Contains(t, renderedPro, "jedi")
	assert.Contains(t, renderedPro, "INSPECTED OBJECT:")

	// 8. Test Render in Jedi Mode (Zen)
	model.SetEditorProfile("jedi")
	renderedJedi := model.Render()
	assert.NotContains(t, renderedJedi, "ZQK OBJECT INSPECTOR")
	assert.NotContains(t, renderedJedi, "INSPECTED OBJECT:")
	assert.NotContains(t, renderedJedi, "Nav:")

	// 9. When searching in Jedi mode, search prompt is visible
	model.IsSearching = true
	model.SearchBuffer = "alpha"
	renderedJediSearch := model.Render()
	assert.Contains(t, renderedJediSearch, "Search regex/substring:")
	assert.Contains(t, renderedJediSearch, "alpha")
}

