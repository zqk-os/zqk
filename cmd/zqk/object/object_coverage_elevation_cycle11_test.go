package object

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestCycle11_SPlan_Commands(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// 1. splan list and show when empty
	out, err := executeObjectCommand(t, tempDir, provider, "splan", "list")
	assert.NoError(t, err, "splan list empty: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "splan", "show")
	assert.Error(t, err, "splan show empty should return not found error")

	// 2. Create a strategic plan object with canonical prefix STRAT-PLAN-
	splanObj := map[string]any{
		objects.FieldKeyID:          "STRAT-PLAN-001",
		objects.FieldKeyKind:        objects.KindStrategicPlan,
		objects.FieldKeyTitle:       "Strategic Plan Q4",
		objects.FieldKeyDescription: "Strategic roadmap plan for Q4 execution.",
		objects.FieldKeyStatus:      "active",
		"vision":                    "Universal Autonomous Delivery",
		"planning_horizon":          "1y",
		"themes":                    []any{"Durability", "Ecosystem", "Autonomy"},
	}
	require.NoError(t, provider.Create(ctx, secCtx, splanObj))

	// 3. splan list and show with object present
	out, err = executeObjectCommand(t, tempDir, provider, "splan", "list", "--format", "json")
	assert.NoError(t, err, "splan list json: %s", out)
	assert.Contains(t, out, "STRAT-PLAN-001")

	out, err = executeObjectCommand(t, tempDir, provider, "splan", "show", "STRAT-PLAN-001", "--format", "json")
	assert.NoError(t, err, "splan show specific: %s", out)
	assert.Contains(t, out, "STRAT-PLAN-001")
}

func TestCycle11_Wstrans_Commands(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// 1. wstrans list and show when empty
	out, err := executeObjectCommand(t, tempDir, provider, "wstrans", "list")
	assert.NoError(t, err, "wstrans list empty: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "wstrans", "show")
	assert.Error(t, err, "wstrans show empty should return error")

	// 2. Create a workstream transition object with canonical prefix WST-
	wstransObj := map[string]any{
		objects.FieldKeyID:          "WST-001",
		objects.FieldKeyKind:        objects.KindWorkstreamTransition,
		objects.FieldKeyTitle:       "Workstream Transition 001",
		objects.FieldKeyDescription: "Transitioning workstream into active phase.",
		objects.FieldKeyStatus:      "active",
	}
	require.NoError(t, provider.Create(ctx, secCtx, wstransObj))

	// 3. wstrans list and show with object present
	out, err = executeObjectCommand(t, tempDir, provider, "wstrans", "list", "--format", "json")
	assert.NoError(t, err, "wstrans list json: %s", out)
	assert.Contains(t, out, "WST-001")

	out, err = executeObjectCommand(t, tempDir, provider, "wstrans", "show", "WST-001", "--format", "json")
	assert.NoError(t, err, "wstrans show specific: %s", out)
	assert.Contains(t, out, "WST-001")
}

func TestCycle11_Evoman_Commands(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// 1. evoman list and show when empty
	out, err := executeObjectCommand(t, tempDir, provider, "evoman", "list")
	assert.NoError(t, err, "evoman list empty: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "evoman", "show")
	assert.Error(t, err, "evoman show empty should return error")

	// 2. Create an evolution management object with canonical prefix EVO-
	evomanObj := map[string]any{
		objects.FieldKeyID:          "EVO-001",
		objects.FieldKeyKind:        objects.KindEvolutionManagement,
		objects.FieldKeyTitle:       "Evolution Management 001",
		objects.FieldKeyDescription: "Management of system evolution and migrations.",
		objects.FieldKeyStatus:      "active",
	}
	require.NoError(t, provider.Create(ctx, secCtx, evomanObj))

	// 3. evoman list and show with object present
	out, err = executeObjectCommand(t, tempDir, provider, "evoman", "list", "--format", "json")
	assert.NoError(t, err, "evoman list json: %s", out)
	assert.Contains(t, out, "EVO-001")

	out, err = executeObjectCommand(t, tempDir, provider, "evoman", "show", "EVO-001", "--format", "json")
	assert.NoError(t, err, "evoman show specific: %s", out)
	assert.Contains(t, out, "EVO-001")
}

func TestCycle11_PPlan_Commands(t *testing.T) {
	cmd := NewPPlanCmd()
	assert.Equal(t, "pplan", cmd.Use)
	assert.Contains(t, cmd.Aliases, "plan")

	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// 1. pplan current when no priority plan exists -> error expected
	_, err = executeObjectCommand(t, tempDir, provider, "pplan", "current")
	assert.Error(t, err)

	// 2. Seed an in_progress priority plan and associated backlog items
	planObj := map[string]any{
		objects.FieldKeyID:          "PRI-CYCLE-11-TEST",
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyTitle:       "Cycle 11 Priority Plan",
		objects.FieldKeyDescription: "Priority plan for cycle 11 testing.",
		objects.FieldKeyStatus:      "in_progress",
		"active_order":              1,
	}
	require.NoError(t, provider.Create(ctx, secCtx, planObj))

	bliObj := map[string]any{
		objects.FieldKeyID:                       "BLI-PPLAN-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "BLI under active plan",
		objects.FieldKeyStatus:                   "in_progress",
		objects.FieldKeyProblemStatement:         "Problem statement with sufficient length for validation.",
		objects.FieldKeyDescription:              "Description for testing pplan current command.",
		objects.FieldKeyAcceptanceConsiderations: "Acceptance considerations.",
		objects.FieldKeyPriority:                 "high",
		objects.FieldKeyPriorityTier:             "P1",
		objects.FieldKeyPriorityPlanRef:          "PRI-CYCLE-11-TEST",
	}
	require.NoError(t, provider.Create(ctx, secCtx, bliObj))

	// 3. pplan current with active plan and items
	out, err := executeObjectCommand(t, tempDir, provider, "pplan", "current", "--format", "json")
	assert.NoError(t, err, "pplan current: %s", out)
	assert.Contains(t, out, "BLI-PPLAN-001")
}

func TestCycle11_NamespaceScope_Logic(t *testing.T) {
	// 1. applyNamespaceBoundaries with nil filters
	scope := applyNamespaceBoundaries(nil, nil)
	assert.Equal(t, namespaceScopeModeDefault, scope.Mode)
	assert.True(t, scope.IsolationActive)
	assert.Equal(t, validation.DefaultNamespaceKernel, scope.NamespaceScope)

	// 2. applyNamespaceBoundaries with federated flag
	cmdFed := &cobra.Command{Use: "test"}
	cmdFed.Flags().Bool("federated", true, "")
	filters := make(map[string]any)
	scope = applyNamespaceBoundaries(cmdFed, filters)
	assert.Equal(t, namespaceScopeModeFederated, scope.Mode)
	assert.False(t, scope.IsolationActive)
	assert.Empty(t, scope.NamespaceScope)

	// 3. applyNamespaceBoundaries with all-namespaces flag
	cmdAll := &cobra.Command{Use: "test"}
	cmdAll.Flags().Bool("all-namespaces", true, "")
	filters = make(map[string]any)
	scope = applyNamespaceBoundaries(cmdAll, filters)
	assert.Equal(t, namespaceScopeModeFederated, scope.Mode)

	// 4. applyNamespaceBoundaries with explicit namespace flag
	cmdNS := &cobra.Command{Use: "test"}
	cmdNS.Flags().String("namespace", "custom-ns", "")
	filters = make(map[string]any)
	scope = applyNamespaceBoundaries(cmdNS, filters)
	assert.Equal(t, namespaceScopeModeExplicit, scope.Mode)
	assert.Equal(t, "custom-ns", scope.NamespaceScope)
	assert.Equal(t, "custom-ns", filters[objects.FieldKeyNamespaceID])

	// 5. applyNamespaceBoundaries with existing filter
	cmdNone := &cobra.Command{Use: "test"}
	filters = map[string]any{objects.FieldKeyNamespaceID: "filter-ns"}
	scope = applyNamespaceBoundaries(cmdNone, filters)
	assert.Equal(t, namespaceScopeModeFilter, scope.Mode)
	assert.Equal(t, "filter-ns", scope.NamespaceScope)

	// 6. attachNamespaceScopeMeta
	meta := attachNamespaceScopeMeta(nil, scope)
	require.NotNil(t, meta)
	assert.Equal(t, namespaceScopeModeFilter, meta[metaKeyNamespaceScopeMode])
	assert.Equal(t, "filter-ns", meta[metaKeyNamespaceScope])

	hiddenCount := 5
	scope.HiddenOutsideScope = &hiddenCount
	meta = attachNamespaceScopeMeta(meta, scope)
	assert.Equal(t, 5, meta[metaKeyHiddenOutsideScope])

	// 7. formatNamespaceScopeTableLine
	lineFed := formatNamespaceScopeTableLine(NamespaceQueryScope{Mode: namespaceScopeModeFederated})
	assert.Contains(t, lineFed, "federated")

	lineNormal := formatNamespaceScopeTableLine(NamespaceQueryScope{NamespaceScope: "tenant-a"})
	assert.Equal(t, "Namespace: tenant-a", lineNormal)

	lineHidden := formatNamespaceScopeTableLine(NamespaceQueryScope{NamespaceScope: "tenant-a", HiddenOutsideScope: &hiddenCount})
	assert.Contains(t, lineHidden, "hiding 5 outside scope")

	// 8. scopedCountFromMeta and namespaceScopeFromMeta
	assert.Equal(t, 0, scopedCountFromMeta(nil))
	assert.Equal(t, 10, scopedCountFromMeta(map[string]any{"total_count": 10}))

	_, ok := namespaceScopeFromMeta(nil)
	assert.False(t, ok)
	_, ok = namespaceScopeFromMeta(map[string]any{})
	assert.False(t, ok)

	parsedScope, ok := namespaceScopeFromMeta(meta)
	assert.True(t, ok)
	assert.Equal(t, namespaceScopeModeFilter, parsedScope.Mode)
	assert.Equal(t, "filter-ns", parsedScope.NamespaceScope)
	assert.Equal(t, 5, *parsedScope.HiddenOutsideScope)

	// 9. enrichHiddenOutsideScope edge cases
	enrichHiddenOutsideScope(nil, "", nil, 0, nil)
	sDummy := &NamespaceQueryScope{IsolationActive: false}
	enrichHiddenOutsideScope(nil, "backlog_item", nil, 0, sDummy)
	assert.Equal(t, 0, *sDummy.HiddenOutsideScope)
}

func TestCycle11_ChildRollupOverlay(t *testing.T) {
	// Nil guards
	applyChildRollupOverlay(nil, nil)
	applyChildRollupOverlay(nil, map[string]any{"id": ""})

	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// Seed priority plan and completed backlog items
	planID := "PRI-ROLLUP-TEST"
	planObj := map[string]any{
		objects.FieldKeyID:          planID,
		objects.FieldKeyKind:        objects.KindPriorityPlan,
		objects.FieldKeyTitle:       "Rollup Test Plan",
		objects.FieldKeyDescription: "Plan to test child rollup overlay.",
		objects.FieldKeyStatus:      "in_progress",
	}
	require.NoError(t, provider.Create(ctx, secCtx, planObj))

	bli1 := map[string]any{
		objects.FieldKeyID:                       "BLI-ROLLUP-1",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Rollup item 1",
		objects.FieldKeyStatus:                   "complete",
		objects.FieldKeyProblemStatement:         "Problem statement for rollup testing 1.",
		objects.FieldKeyDescription:              "Description for rollup testing 1.",
		objects.FieldKeyAcceptanceConsiderations: "Acceptance considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyPriorityPlanRef:          planID,
	}
	require.NoError(t, provider.Create(ctx, secCtx, bli1))

	// Execute get on priority plan with JSON format to trigger overlay
	out, err := executeObjectCommand(t, tempDir, provider, "get", planID, "--format", "json")
	assert.NoError(t, err, "get plan with overlay: %s", out)
	assert.Contains(t, out, "child_rollup")
	assert.Contains(t, out, "priority_plan_percent_complete")
}

func TestCycle11_MilestoneCriteriaOverlay(t *testing.T) {
	// Non-milestone kind is no-op
	nonMilestone := map[string]any{objects.FieldKeyKind: objects.KindGoal}
	applyMilestoneCriteriaOverlay(nil, nonMilestone)
	assert.NotContains(t, nonMilestone, "milestone_complete_via_criteria_refs")

	// Milestone with resolved criteria refs
	mObj := map[string]any{
		objects.FieldKeyKind: objects.KindMilestone,
		"resolved_criteria_refs": []map[string]any{
			{"id": "CRIT-1", "status": "complete"},
			{"id": "CRIT-2", "status": "in_progress"},
		},
	}
	applyMilestoneCriteriaOverlay(nil, mObj)
	assert.False(t, mObj["milestone_complete_via_criteria_refs"].(bool))
	assert.InDelta(t, 50.0, mObj["milestone_percent_complete_via_criteria_refs"].(float64), 0.1)

	// All complete
	mObjAll := map[string]any{
		objects.FieldKeyKind: objects.KindMilestone,
		"resolved_criteria_refs": []any{
			map[string]any{"id": "CRIT-1", "status": "completed"},
			map[string]any{"id": "CRIT-2", "status": "validated"},
		},
	}
	applyMilestoneCriteriaOverlay(nil, mObjAll)
	assert.True(t, mObjAll["milestone_complete_via_criteria_refs"].(bool))
	assert.InDelta(t, 100.0, mObjAll["milestone_percent_complete_via_criteria_refs"].(float64), 0.1)

	// Status complete check helper
	assert.True(t, milestoneCriteriaStatusIsComplete("complete"))
	assert.True(t, milestoneCriteriaStatusIsComplete("completed"))
	assert.True(t, milestoneCriteriaStatusIsComplete("validated"))
	assert.False(t, milestoneCriteriaStatusIsComplete("passed"))
	assert.False(t, milestoneCriteriaStatusIsComplete("verified"))
	assert.False(t, milestoneCriteriaStatusIsComplete("in_progress"))
	assert.False(t, milestoneCriteriaStatusIsComplete("draft"))
}

func TestCycle11_MutationDurabilityLogging(t *testing.T) {
	// 1. Nil logger guard
	logSlowCLIObjectMutationFlush(nil, "create", "BLI-1", nil, 3*time.Second, 0)

	// 2. Below threshold (no log emitted)
	logger := logging.NewEventLogger(context.Background())
	logSlowCLIObjectMutationFlush(logger, "create", "BLI-1", []string{"backlog_item"}, 100*time.Millisecond, 50*time.Millisecond)

	// 3. At or above threshold (2s)
	logSlowCLIObjectMutationFlush(logger, "update", "BLI-2", []string{"backlog_item"}, 1500*time.Millisecond, 600*time.Millisecond)

	// 4. Above threshold with empty ID and empty kinds
	logSlowCLIObjectMutationFlush(logger, "delete", "", nil, 2500*time.Millisecond, 0)
}

func TestCycle11_ObjectIDArgs(t *testing.T) {
	// 1. Nil command
	ids := expandObjectIDArgs(nil, []string{"BLI-1,BLI-2", "BLI-3"})
	assert.Equal(t, []string{"BLI-1", "BLI-2", "BLI-3"}, ids)

	// 2. Command with --ids flag changed
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("ids", "", "")
	require.NoError(t, cmd.Flags().Set("ids", "BLI-4, BLI-5"))

	ids = expandObjectIDArgs(cmd, []string{"BLI-1"})
	assert.Contains(t, ids, "BLI-1")
	assert.Contains(t, ids, "BLI-4")
	assert.Contains(t, ids, "BLI-5")
}

func TestCycle11_RootFieldsCommand(t *testing.T) {
	cmd := NewObjectRootFieldsCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "fields [flags]", cmd.Use)

	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// 1. Calling fields without --list-kinds fails with helpful message
	_, err = executeObjectCommand(t, tempDir, provider, "fields")
	assert.Error(t, err)

	// 2. Calling fields with --list-kinds succeeds
	out, err := executeObjectCommand(t, tempDir, provider, "fields", "--list-kinds")
	assert.NoError(t, err, "fields --list-kinds: %s", out)
	assert.Contains(t, out, "backlog_item")

	// 3. Calling fields with --list-kinds --all-kinds succeeds
	out, err = executeObjectCommand(t, tempDir, provider, "fields", "--list-kinds", "--all-kinds", "--format", "json")
	assert.NoError(t, err, "fields --list-kinds --all-kinds json: %s", out)
	assert.Contains(t, out, "goal")
}

func TestCycle11_LifecycleProbe_Helpers(t *testing.T) {
	// 1. lifecycleStatusByValue
	assert.Equal(t, objects.Status{}, lifecycleStatusByValue(nil, "active"))

	lc := &objects.Lifecycle{
		Statuses: []objects.Status{
			{Value: "draft", Description: "Draft status"},
			{Value: "in_progress", Description: "Active work"},
			{Value: "complete", Description: "Completed"},
		},
	}
	s := lifecycleStatusByValue(lc, "in_progress")
	assert.Equal(t, "in_progress", s.Value)
	assert.Equal(t, "Active work", s.Description)

	assert.Equal(t, objects.Status{}, lifecycleStatusByValue(lc, "nonexistent"))

	// 2. isNonProgressLifecycleProbeCandidate
	assert.False(t, isNonProgressLifecycleProbeCandidate("in_progress", objects.Status{}))
	assert.True(t, isNonProgressLifecycleProbeCandidate("archived", objects.Status{Archive: true}))
	assert.True(t, isNonProgressLifecycleProbeCandidate("blocked", objects.Status{}))
	assert.True(t, isNonProgressLifecycleProbeCandidate("rejected", objects.Status{}))

	// 3. isSuccessLifecycleTerminal
	assert.True(t, isSuccessLifecycleTerminal("complete"))
	assert.True(t, isSuccessLifecycleTerminal("completed"))
	assert.True(t, isSuccessLifecycleTerminal("implemented"))
	assert.True(t, isSuccessLifecycleTerminal("success"))
	assert.False(t, isSuccessLifecycleTerminal("in_progress"))
	assert.False(t, isSuccessLifecycleTerminal("draft"))

	// 4. promoteAllowsArchiveHop
	assert.True(t, promoteAllowsArchiveHop("complete", "archived"))
	assert.True(t, promoteAllowsArchiveHop("completed", "archived"))
	assert.False(t, promoteAllowsArchiveHop("in_progress", "archived"))
	assert.False(t, promoteAllowsArchiveHop("complete", "in_progress"))

	// 5. getPercentComplete
	config := objects.PercentCompleteConfig{
		DefaultByStatus: map[string]any{
			"draft":       10,
			"in_progress": 50,
			"complete":    100,
		},
	}
	assert.InDelta(t, 50.0, getPercentComplete("in_progress", config), 0.1)
}

func TestCycle11_InspectTUI_Model(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// Seed backlog item for TUI model
	bli := map[string]any{
		objects.FieldKeyID:                       "BLI-TUI-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyTitle:                    "Inspect TUI Backlog Item",
		objects.FieldKeyStatus:                   "in_progress",
		objects.FieldKeyProblemStatement:         "Problem statement for TUI test with sufficient length.",
		objects.FieldKeyDescription:              "Description for TUI test with sufficient length.",
		objects.FieldKeyAcceptanceConsiderations: "Acceptance considerations.",
		objects.FieldKeyPriority:                 "high",
		objects.FieldKeyPriorityTier:             "P1",
	}
	require.NoError(t, provider.Create(ctx, secCtx, bli))

	m := NewInspectTUIModel(ctx, "backlog_item", nil, nil, "id", true, provider, secCtx, nil)
	require.NotNil(t, m)

	// Cycle editor profiles
	assert.Equal(t, "newb", m.EditorProfile)
	m.CycleEditorProfile()
	assert.Equal(t, "pro", m.EditorProfile)
	m.CycleEditorProfile()
	assert.Equal(t, "jedi", m.EditorProfile)
	m.CycleEditorProfile()
	assert.Equal(t, "newb", m.EditorProfile)

	// SetEditorProfile
	m.SetEditorProfile("pro")
	assert.Equal(t, "pro", m.EditorProfile)
	m.SetEditorProfile("jedi")
	assert.Equal(t, "jedi", m.EditorProfile)
	m.SetEditorProfile("unknown_profile")
	assert.Equal(t, "newb", m.EditorProfile)

	// Cycle filters
	assert.Equal(t, "all", m.FilterPill)
	m.CycleFilter()
	assert.NotEqual(t, "all", m.FilterPill)

	// Cycle sorts
	m.CycleSort()

	// Cycle kinds
	m.CycleKind(true)
	m.CycleKind(false)

	// SetStatus
	m.SetStatus("Test status notification", 5*time.Second)
	assert.Equal(t, "Test status notification", m.StatusMessage)

	// Filter matching
	testObj := map[string]any{
		"id":     "BLI-TEST",
		"status": "in_progress",
		"title":  "A searchable title",
	}
	m.FilterPill = "all"
	assert.True(t, m.matchesCurrentFilter(testObj))
	m.FilterPill = "active"
	assert.True(t, m.matchesCurrentFilter(testObj))
	m.FilterPill = "draft"
	assert.False(t, m.matchesCurrentFilter(testObj))

	// Search matching
	m.SearchQuery = "searchable"
	assert.True(t, m.matchesSearch(testObj))
	m.SearchQuery = "unmatched_query_xyz"
	assert.False(t, m.matchesSearch(testObj))
	m.SearchQuery = ""

	// Render
	rendered := m.Render()
	assert.NotEmpty(t, rendered)

	// Action items
	actions := m.prepareActionItems()
	assert.NotEmpty(t, actions)

	// HandleInput key operations
	assert.False(t, m.HandleInput([]byte("?"))) // cycles editor profile
	assert.Equal(t, "pro", m.EditorProfile)

	assert.False(t, m.HandleInput([]byte("/"))) // open search
	assert.True(t, m.IsSearching)
	assert.False(t, m.HandleInput([]byte{27})) // escape closes search
	assert.False(t, m.IsSearching)

	assert.False(t, m.HandleInput([]byte("a"))) // open action palette
	assert.True(t, m.ActionPaletteOpen)
	assert.False(t, m.HandleInput([]byte{27})) // escape closes action palette
	assert.False(t, m.ActionPaletteOpen)

	assert.False(t, m.HandleInput([]byte("p"))) // toggle policy studio
	assert.True(t, m.PolicyStudioOpen)
	assert.False(t, m.HandleInput([]byte{27})) // escape closes policy studio
	assert.False(t, m.PolicyStudioOpen)

	assert.True(t, m.HandleInput([]byte("q"))) // q quits
}

func TestCycle11_LifecycleUpdater_EdgeCases(t *testing.T) {
	// 1. RunPriorityPlanCompleteUpdater with empty projectRoot (no-op)
	RunPriorityPlanCompleteUpdater(context.Background(), "")

	// 2. RunPriorityPlanCompleteUpdater with non-cached projectRoot (no-op)
	RunPriorityPlanCompleteUpdater(context.Background(), "/nonexistent/test/dir")
}
