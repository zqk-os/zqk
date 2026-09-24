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
	objects   map[string]map[string]any
	filePaths map[string]string
}

func (m *mockInspectStorage) GetFilePathForObject(id, kind string) (string, error) {
	if m.filePaths != nil {
		if p, ok := m.filePaths[id]; ok {
			return p, nil
		}
	}
	return "", nil
}

func (m *mockInspectStorage) Read(ctx context.Context, secCtx *pkgctx.SecurityContext, id string) (map[string]any, error) {
	if obj, ok := m.objects[id]; ok {
		return obj, nil
	}
	return nil, nil
}

func (m *mockInspectStorage) Update(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, updates map[string]any) error {
	if obj, ok := m.objects[id]; ok {
		for k, v := range updates {
			obj[k] = v
		}
	}
	return nil
}

func (m *mockInspectStorage) Delete(ctx context.Context, secCtx *pkgctx.SecurityContext, id string, force bool) error {
	delete(m.objects, id)
	return nil
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

func TestInspect_ModularCards_StorageAndOntology(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"BLI-CARDS-001": {
				objects.FieldKeyID:       "BLI-CARDS-001",
				objects.FieldKeyKind:     objects.KindBacklogItem,
				objects.FieldKeyTitle:    "Display Cards Test",
				objects.FieldKeyStatus:   "originated",
				objects.FieldKeyPriority: "P1",
				"claimed_by":             "agent-test",
				"namespace_id":           "zqk:kernel",
				"version_context":        "v2.0.0",
				"requirement_ref":        "REQ-001",
				"milestone_ref":          "MIL-001",
				"criteria_refs":          []string{"CRIT-001"},
				"updated_at":             "2026-09-23T14:00:00Z",
			},
			"CRIT-001": {
				objects.FieldKeyID:     "CRIT-001",
				objects.FieldKeyKind:   objects.KindCriteria,
				objects.FieldKeyStatus: "satisfied",
			},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()

	// 1. Test buildSemanticProjection
	rawObj, err := mockStorage.Read(ctx, secCtx, "BLI-CARDS-001")
	require.NoError(t, err)
	proj := buildSemanticProjection(ctx, mockStorage, secCtx, rawObj, objects.KindBacklogItem, nil)

	// Verify StorageProfile
	require.NotNil(t, proj.StorageProfile)
	assert.NotEmpty(t, proj.StorageProfile.CASHash)
	assert.NotEmpty(t, proj.StorageProfile.StoragePlane)
	assert.True(t, proj.StorageProfile.ByteSize > 0)
	assert.NotEmpty(t, proj.StorageProfile.Permissions)
	assert.Equal(t, "2026-09-23T14:00:00Z", proj.StorageProfile.LastModified)

	// Verify draft-plane path detection (.zqk/object_drafts/...)
	mockStorage.filePaths = map[string]string{
		"BLI-CARDS-001": "/Users/test/workspace/.zqk/object_drafts/backlog_item/ab/BLI-CARDS-001.yaml",
	}
	projDraft := buildSemanticProjection(ctx, mockStorage, secCtx, rawObj, objects.KindBacklogItem, nil)
	require.NotNil(t, projDraft.StorageProfile)
	assert.Equal(t, "draft_plane", projDraft.StorageProfile.StoragePlane)
	assert.Equal(t, "/Users/test/workspace/.zqk/object_drafts/backlog_item/ab/BLI-CARDS-001.yaml", projDraft.StorageProfile.FilePath)

	// Verify Ontology
	require.NotNil(t, proj.Ontology)
	assert.Equal(t, "zqk:kernel", proj.Ontology.Namespace)
	assert.Equal(t, "v2.0.0", proj.Ontology.VersionContext)
	assert.NotEmpty(t, proj.Ontology.StorageProfile)
	assert.NotEmpty(t, proj.Ontology.Traits)

	// Verify Lineage Radar & Criteria
	require.NotNil(t, proj.Lineage)
	assert.Equal(t, "REQ-001", proj.Lineage.Requirement)
	assert.Equal(t, "MIL-001", proj.Lineage.Milestone)
	assert.True(t, proj.Lineage.IsIntact)
	require.NotNil(t, proj.CriteriaSummary)
	assert.Equal(t, 1, proj.CriteriaSummary.Total)
	assert.Equal(t, 1, proj.CriteriaSummary.Satisfied)

	// 2. Test Human/TDS output rendering in inspectSingleObject
	var buf bytes.Buffer
	testCmd := &cobra.Command{Use: "inspect"}
	testCmd.SetOut(&buf)
	err = inspectSingleObject(testCmd, objects.KindBacklogItem, "BLI-CARDS-001", nil, mockStorage, ctx, secCtx, false)
	require.NoError(t, err)
	outStr := buf.String()
	assert.Contains(t, outStr, "CAS Storage & Data-Cell Profile")
	assert.Contains(t, outStr, "Ontology & Schema Profile")
	assert.Contains(t, outStr, "Lineage & Traceability Radar")
	assert.Contains(t, outStr, "Acceptance Criteria (1 total, 1 satisfied, 0 pending)")
}

func TestInspectTUIModel_ActionPalette_RoleGated(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"BLI-ACT-001": {
				objects.FieldKeyID:       "BLI-ACT-001",
				objects.FieldKeyKind:     objects.KindBacklogItem,
				objects.FieldKeyTitle:    "Action Palette Integration",
				objects.FieldKeyStatus:   "originated",
				objects.FieldKeyPriority: "P1",
				"claimed_by":             "",
				"updated_at":             "2026-09-23T12:00:00Z",
			},
		},
	}

	ctx := context.Background()
	// Unprivileged operator (cannot delete, but can claim and transition)
	unprivilegedSecCtx := pkgctx.NewSecurityContext("operator-user", []string{"developer"}, []string{"read:*", "write:backlog_item"})
	storageCtx := pkgctx.NewStorageContext()

	model := NewInspectTUIModel(ctx, objects.KindBacklogItem, nil, nil, "updated_at", false, mockStorage, unprivilegedSecCtx, storageCtx)
	require.NotNil(t, model)
	model.Width = 140
	assert.Equal(t, 1, len(model.VisibleProjections))

	// 1. Open Action Palette with 'a'
	model.HandleInput([]byte{'a'})
	assert.True(t, model.ActionPaletteOpen)
	assert.Equal(t, 0, model.ActionIndex)

	renderedPalette := model.Render()
	assert.Contains(t, renderedPalette, "ROLE-GATED ACTION PALETTE: BLI-ACT-001")
	assert.Contains(t, renderedPalette, "Claim Work")
	assert.Contains(t, renderedPalette, "Transition Status")
	assert.Contains(t, renderedPalette, "Edit in $EDITOR")
	assert.Contains(t, renderedPalette, "Open Policy Studio")
	assert.Contains(t, renderedPalette, "Delete Object")
	// For unprivileged user, delete should be locked
	assert.Contains(t, renderedPalette, "LOCKED: requires role:admin or permission:delete:object")

	// 2. Test Navigation in Action Palette ('j' and 'k')
	model.HandleInput([]byte{'j'})
	assert.Equal(t, 1, model.ActionIndex) // Selected: Transition Status
	model.HandleInput([]byte{'k'})
	assert.Equal(t, 0, model.ActionIndex) // Selected: Claim Work

	// 3. Test Claim execution via hotkey 'c' in Action Palette
	model.HandleInput([]byte{'c'})
	assert.False(t, model.ActionPaletteOpen) // Palette closes on action execution
	// Object should now be claimed by operator-user
	obj, _ := mockStorage.Read(ctx, unprivilegedSecCtx, "BLI-ACT-001")
	assert.Equal(t, "operator-user", obj["claimed_by"])

	// 4. Test Transition Status via hotkey 't' in main table
	model.HandleInput([]byte{'t'})
	obj, _ = mockStorage.Read(ctx, unprivilegedSecCtx, "BLI-ACT-001")
	assert.Equal(t, "in_progress", obj["status"])

	// 5. Test Admin Role Unlocks Delete
	adminSecCtx := pkgctx.NewSystemSecurityContext()
	adminModel := NewInspectTUIModel(ctx, objects.KindBacklogItem, nil, nil, "updated_at", false, mockStorage, adminSecCtx, storageCtx)
	adminModel.Width = 140
	adminModel.HandleInput([]byte{'a'})
	adminRendered := adminModel.Render()
	assert.NotContains(t, adminRendered, "LOCKED:")

	// 6. Test Close Action Palette with 'Esc'
	adminModel.HandleInput([]byte{27})
	assert.False(t, adminModel.ActionPaletteOpen)
}

func TestPolicyStudio_SuggestDSLTokens(t *testing.T) {
	// 1. Empty input suggests available schema fields
	suggs := SuggestDSLTokens(objects.KindBacklogItem, "")
	require.NotEmpty(t, suggs)
	hasID := false
	hasStatus := false
	for _, s := range suggs {
		if s.Token == objects.FieldKeyID {
			hasID = true
		}
		if s.Token == objects.FieldKeyStatus {
			hasStatus = true
		}
	}
	assert.True(t, hasID, "expected id token suggested")
	assert.True(t, hasStatus, "expected status token suggested")

	// 2. Exact field token suggests comparison operators
	opSuggs := SuggestDSLTokens(objects.KindBacklogItem, "status")
	require.NotEmpty(t, opSuggs)
	hasEquals := false
	hasIsPopulated := false
	for _, s := range opSuggs {
		if s.Token == "==" {
			hasEquals = true
		}
		if s.Token == "is_populated" {
			hasIsPopulated = true
		}
	}
	assert.True(t, hasEquals, "expected == operator suggested")
	assert.True(t, hasIsPopulated, "expected is_populated operator suggested")

	// 3. Status operator suggests lifecycle statuses
	statusValSuggs := SuggestDSLTokens(objects.KindBacklogItem, "status ==")
	require.NotEmpty(t, statusValSuggs)
	for _, s := range statusValSuggs {
		assert.Equal(t, TokenTypeValue, s.Type)
	}

	// 4. Logical AND suggests schema fields again
	logicalSuggs := SuggestDSLTokens(objects.KindBacklogItem, "status == \"planned\" &&")
	require.NotEmpty(t, logicalSuggs)
	assert.Equal(t, TokenTypeField, logicalSuggs[0].Type)
}

func TestPolicyStudio_EvaluateDSLExpression(t *testing.T) {
	objPass := map[string]any{
		objects.FieldKeyID:       "BLI-TEST-001",
		objects.FieldKeyTitle:    "Policy Studio Implementation",
		objects.FieldKeyStatus:   "in_progress",
		objects.FieldKeyPriority: "P0",
		"requirement_refs":       []any{"REQ-001"},
		"milestone_refs":         []any{"MIL-001"},
		"tags":                   []string{"qa", "studio"},
		"effort_hours":           8,
	}

	// Unary predicates
	pass, err := EvaluateDSLExpression("id is_populated", objPass)
	require.NoError(t, err)
	assert.True(t, pass)

	pass, err = EvaluateDSLExpression("missing_key is_empty", objPass)
	require.NoError(t, err)
	assert.True(t, pass)

	pass, err = EvaluateDSLExpression("requirement_refs is_not_empty", objPass)
	require.NoError(t, err)
	assert.True(t, pass)

	// Binary equality & comparison
	pass, err = EvaluateDSLExpression("status == \"in_progress\"", objPass)
	require.NoError(t, err)
	assert.True(t, pass)

	pass, err = EvaluateDSLExpression("priority != \"P2\"", objPass)
	require.NoError(t, err)
	assert.True(t, pass)

	pass, err = EvaluateDSLExpression("title contains \"Studio\"", objPass)
	require.NoError(t, err)
	assert.True(t, pass)

	pass, err = EvaluateDSLExpression("effort_hours > 5", objPass)
	require.NoError(t, err)
	assert.True(t, pass)

	// Compound expressions with &&
	compoundExpr := "status == \"in_progress\" && priority == \"P0\" && requirement_refs is_not_empty"
	pass, err = EvaluateDSLExpression(compoundExpr, objPass)
	require.NoError(t, err)
	assert.True(t, pass)

	// Failing compound condition
	failExpr := "status == \"in_progress\" && priority == \"P3\""
	pass, err = EvaluateDSLExpression(failExpr, objPass)
	require.NoError(t, err)
	assert.False(t, pass)
}

func TestPolicyStudio_RunPolicyStudioDryRun(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"BLI-POL-001": {
				objects.FieldKeyID:       "BLI-POL-001",
				objects.FieldKeyKind:     objects.KindBacklogItem,
				objects.FieldKeyTitle:    "Valid Item",
				objects.FieldKeyStatus:   "planned",
				"requirement_refs":       []any{"REQ-1"},
				"milestone_refs":         []any{"MIL-1"},
				"estimated_effort":       "2d",
			},
			"BLI-POL-002": {
				objects.FieldKeyID:       "BLI-POL-002",
				objects.FieldKeyKind:     objects.KindBacklogItem,
				objects.FieldKeyTitle:    "Invalid Item (Missing lineage & effort)",
				objects.FieldKeyStatus:   "planned",
			},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	rules := []PolicyRule{
		{
			ID:          "POL-LINEAGE-CHECK",
			Name:        "Lineage Intact",
			TargetKind:  objects.KindBacklogItem,
			Expression:  "requirement_refs is_not_empty && milestone_refs is_not_empty",
			Severity:    "blocker",
		},
		{
			ID:          "POL-EFFORT-CHECK",
			Name:        "Effort Populated",
			TargetKind:  objects.KindBacklogItem,
			Expression:  "estimated_effort is_populated",
			Severity:    "warning",
		},
	}

	results, err := RunPolicyStudioDryRun(ctx, mockStorage, secCtx, storageCtx, objects.KindBacklogItem, rules)
	require.NoError(t, err)
	require.Len(t, results, 2)

	// Both rules should have 1 violation (BLI-POL-002)
	assert.False(t, results[0].Passed)
	assert.Equal(t, 1, results[0].ViolationsCount)
	assert.Contains(t, results[0].OffendingIDs, "BLI-POL-002")

	assert.False(t, results[1].Passed)
	assert.Equal(t, 1, results[1].ViolationsCount)
	assert.Contains(t, results[1].OffendingIDs, "BLI-POL-002")
}

func TestInspectTUIModel_PolicyStudio_Interactive(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"BLI-TUI-POL-001": {
				objects.FieldKeyID:       "BLI-TUI-POL-001",
				objects.FieldKeyKind:     objects.KindBacklogItem,
				objects.FieldKeyTitle:    "TUI Studio Item",
				objects.FieldKeyStatus:   "planned",
				objects.FieldKeyPriority: "P1",
				"requirement_refs":       []any{"REQ-1"},
				"milestone_refs":         []any{"MIL-1"},
			},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	model := NewInspectTUIModel(ctx, objects.KindBacklogItem, nil, nil, "updated_at", false, mockStorage, secCtx, storageCtx)
	require.NotNil(t, model)
	model.Width = 140

	// 1. Toggle Policy Studio Open with 'p'
	model.HandleInput([]byte{'p'})
	assert.True(t, model.PolicyStudioOpen)
	assert.False(t, model.DSLEditMode)
	assert.NotEmpty(t, model.PolicyRules)

	rendered := model.Render()
	assert.Contains(t, rendered, "LIVE POLICY RULE STUDIO: BACKLOG_ITEM")
	assert.Contains(t, rendered, "POL-INTEGRITY-LINEAGE-001")

	// 2. Navigate Rules with 'j' and 'k'
	model.HandleInput([]byte{'j'})
	assert.Equal(t, 1, model.ActiveRuleIndex)
	model.HandleInput([]byte{'k'})
	assert.Equal(t, 0, model.ActiveRuleIndex)

	// 3. Enter DSL Edit Mode with 'c'
	model.HandleInput([]byte{'c'})
	assert.True(t, model.DSLEditMode)
	assert.NotEmpty(t, model.DSLSuggestions)

	// 4. Type a condition into DSL buffer
	for _, ch := range "status == " {
		model.HandleInput([]byte{byte(ch)})
	}
	assert.Contains(t, model.DSLInputBuffer, "status ==")

	// 5. Test Autocomplete Tab Key
	model.HandleInput([]byte{9}) // Tab
	// Commit condition with Enter
	model.HandleInput([]byte{13})
	assert.False(t, model.DSLEditMode)

	// 6. Test Dry-Run Re-evaluation with 't'
	model.HandleInput([]byte{'t'})
	assert.NotEmpty(t, model.DryRunResults)

	// 7. Close Policy Studio with 'Esc'
	model.HandleInput([]byte{27})
	assert.False(t, model.PolicyStudioOpen)
}

func TestInspectTUIModel_StatusTransitionInDrillDownModal(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"BLI-MODAL-001": {
				objects.FieldKeyID:       "BLI-MODAL-001",
				objects.FieldKeyKind:     objects.KindBacklogItem,
				objects.FieldKeyTitle:    "Modal Transition Item",
				objects.FieldKeyStatus:   "originated",
				objects.FieldKeyPriority: "P1",
			},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	model := NewInspectTUIModel(ctx, objects.KindBacklogItem, nil, nil, "updated_at", false, mockStorage, secCtx, storageCtx)
	require.NotNil(t, model)
	model.Width = 120
	model.Height = 30

	// 1. Enter drill-down modal
	model.HandleInput([]byte{13})
	assert.True(t, model.DetailModalOpen)

	modalRender := model.Render()
	assert.Contains(t, modalRender, "DEEP OBJECT INSPECTION: BLI-MODAL-001")
	assert.Contains(t, modalRender, "[t] Status Transition")

	// 2. Press 't' in drill-down modal
	model.HandleInput([]byte{'t'})
	assert.True(t, model.DetailModalOpen, "modal must remain open after transition")
	assert.Contains(t, model.StatusMessage, "Status transitioned")
	assert.Equal(t, "in_progress", mockStorage.objects["BLI-MODAL-001"][objects.FieldKeyStatus])

	// 3. Dismiss modal with Esc
	model.HandleInput([]byte{27})
	assert.False(t, model.DetailModalOpen)
}

func TestInspectTUIModel_TestCaseInspection(t *testing.T) {
	mockStorage := &mockInspectStorage{
		objects: map[string]map[string]any{
			"TST-INSPECT-001": {
				objects.FieldKeyID:       "TST-INSPECT-001",
				objects.FieldKeyKind:     objects.KindTestCase,
				objects.FieldKeyTitle:    "End-to-End Test Suite",
				objects.FieldKeyStatus:   "complete",
				objects.FieldKeyPriority: "P0",
				"category":               "test",
				"scope":                  "integration",
				"path_or_id":             "cmd/zqk/object/inspect_test.go",
				"criteria_refs":          []string{"CRIT-1", "CRIT-2"},
				"backlog_item_refs":      []string{"BLI-1"},
			},
			"CRIT-1": {
				objects.FieldKeyID:     "CRIT-1",
				objects.FieldKeyStatus: "satisfied",
			},
			"CRIT-2": {
				objects.FieldKeyID:     "CRIT-2",
				objects.FieldKeyStatus: "satisfied",
			},
		},
	}

	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	model := NewInspectTUIModel(ctx, objects.KindTestCase, nil, nil, "updated_at", false, mockStorage, secCtx, storageCtx)
	require.NotNil(t, model)
	model.Width = 140
	model.Height = 30

	// 1. Verify table view renders test case
	rendered := model.Render()
	assert.Contains(t, rendered, "TST-INSPECT-001")
	assert.Contains(t, rendered, "End-to-End Test Suite")

	// 2. Open drill-down modal
	model.HandleInput([]byte{13})
	assert.True(t, model.DetailModalOpen)

	modalRender := model.Render()
	assert.Contains(t, modalRender, "DEEP OBJECT INSPECTION: TST-INSPECT-001")
	assert.Contains(t, modalRender, "test_case")
	assert.Contains(t, modalRender, "criteria_refs")

	// 3. Dismiss modal
	model.HandleInput([]byte{'q'})
	assert.False(t, model.DetailModalOpen)
}




