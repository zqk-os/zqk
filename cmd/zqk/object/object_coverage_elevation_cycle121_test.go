package object

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCycle121_ElevatedAndSchedulerGuard(t *testing.T) {
	assert.False(t, ElevatedInternalRequested(nil))

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Bool(FlagElevatedInternal, false, "elevated")
	assert.False(t, ElevatedInternalRequested(cmd))

	err := cmd.Flags().Set(FlagElevatedInternal, "true")
	require.NoError(t, err)
	assert.True(t, ElevatedInternalRequested(cmd))

	// RequireElevatedInternal when flag unset
	cmdUnset := &cobra.Command{Use: "test2"}
	cmdUnset.Flags().Bool(FlagElevatedInternal, false, "elevated")
	assert.NoError(t, RequireElevatedInternal(cmdUnset))

	// isZqkAdminExecutable check
	assert.False(t, isZqkAdminExecutable())

	// ShouldSkipInternalKind
	assert.False(t, ShouldSkipInternalKind("object_spec", true))
	assert.True(t, ShouldSkipInternalKind("object_spec", false))
	assert.False(t, ShouldSkipInternalKind("backlog_item", false))

	// requirementForObjectCommand
	assert.Equal(t, schedulerRequirementOptional, requirementForObjectCommand("bulk"))
	assert.Equal(t, schedulerRequirementOptional, requirementForObjectCommand("bulk-delete"))
	assert.Equal(t, schedulerRequirementOptional, requirementForObjectCommand("bulk-update"))
	assert.Equal(t, schedulerRequirementOptional, requirementForObjectCommand("import"))
	assert.Equal(t, schedulerRequirementOptional, requirementForObjectCommand("list"))
	assert.Equal(t, schedulerRequirementOptional, requirementForObjectCommand("count"))
	assert.Equal(t, schedulerRequirementNone, requirementForObjectCommand("get"))

	// runObjectSchedulerGuard
	assert.NoError(t, runObjectSchedulerGuard(nil, nil))
	objCmd := &cobra.Command{Use: "object"}
	assert.NoError(t, runObjectSchedulerGuard(objCmd, nil))
}

func TestCycle121_TypeConversionAndNormalization(t *testing.T) {
	// normalizeValueForField with nil fieldInfo
	val, err := normalizeValueForField("test", nil)
	require.NoError(t, err)
	assert.Equal(t, "test", val)

	// number / float conversions
	numField := &objects.FieldInfo{Type: "number"}
	val, err = normalizeValueForField(42, numField)
	require.NoError(t, err)
	assert.Equal(t, float64(42), val)

	val, err = normalizeValueForField(int32(10), numField)
	require.NoError(t, err)
	assert.Equal(t, float64(10), val)

	val, err = normalizeValueForField(int64(20), numField)
	require.NoError(t, err)
	assert.Equal(t, float64(20), val)

	val, err = normalizeValueForField(float32(3.14), numField)
	require.NoError(t, err)
	assert.InDelta(t, float64(3.14), val.(float64), 0.001)

	val, err = normalizeValueForField(float64(5.5), numField)
	require.NoError(t, err)
	assert.Equal(t, 5.5, val)

	val, err = normalizeValueForField("123.45", numField)
	require.NoError(t, err)
	assert.Equal(t, 123.45, val)

	val, err = normalizeValueForField("not-a-number", numField)
	require.NoError(t, err)
	assert.Equal(t, "not-a-number", val)

	// integer conversions
	intField := &objects.FieldInfo{Type: "integer"}
	val, err = normalizeValueForField(float64(100), intField)
	require.NoError(t, err)
	assert.Equal(t, 100, val)

	val, err = normalizeValueForField(float64(100.5), intField)
	require.NoError(t, err)
	assert.Equal(t, 100.5, val)

	val, err = normalizeValueForField(77, intField)
	require.NoError(t, err)
	assert.Equal(t, 77, val)

	// list / array conversions
	listField := &objects.FieldInfo{Type: "list"}
	val, err = normalizeValueForField([]any{"a", "b"}, listField)
	require.NoError(t, err)
	assert.Equal(t, []any{"a", "b"}, val)

	// string passthrough
	strField := &objects.FieldInfo{Type: "string"}
	val, err = normalizeValueForField("hello", strField)
	require.NoError(t, err)
	assert.Equal(t, "hello", val)

	// normalizeObjectValues with nil kindFields
	assert.NoError(t, normalizeObjectValues(map[string]any{"k": "v"}, nil))

	// NormalizeObjectValues with unknown kind
	testObj := map[string]any{"field": "val"}
	res, err := NormalizeObjectValues(testObj, "unknown_kind_xyz")
	require.NoError(t, err)
	assert.Equal(t, "val", res["field"])
}

func TestCycle121_FieldsHelpers(t *testing.T) {
	cmd := &cobra.Command{Use: "fields"}
	clipkg.AddFieldsFlags(cmd, true)
	flags, err := parseFieldsFlags(cmd)
	require.NoError(t, err)
	assert.False(t, flags.ListKinds)

	// buildFieldMap
	kindFields := &objects.KindFields{
		AllFields: []objects.FieldInfo{
			{Name: "id", Type: "string"},
			{Name: "title", Type: "string"},
		},
		SpecializedFields: []objects.FieldInfo{
			{Name: "title", Type: "string"},
		},
	}
	fMap := buildFieldMap(kindFields)
	assert.Contains(t, fMap, "id")
	assert.Contains(t, fMap, "title")

	// getFieldsToShow
	toShow, err := getFieldsToShow(FieldsFlags{SpecializedOnly: true}, "backlog_item", nil, kindFields)
	require.NoError(t, err)
	assert.Len(t, toShow, 1)

	toShowAll, err := getFieldsToShow(FieldsFlags{}, "backlog_item", nil, kindFields)
	require.NoError(t, err)
	assert.Nil(t, toShowAll)

	// outputFieldsByFormat table and formats
	outCmd := &cobra.Command{Use: "test"}
	assert.NoError(t, outputFieldsTable(outCmd, kindFields))
	assert.NoError(t, outputFieldsJSON(outCmd, kindFields))
	assert.NoError(t, outputFieldsYAML(outCmd, kindFields))
	assert.NoError(t, outputFieldsListTable(outCmd, kindFields.AllFields))
	assert.NoError(t, outputFieldsByFormat(outCmd, objectFormatJSON, kindFields, nil))
	assert.NoError(t, outputFieldsByFormat(outCmd, objectFormatYAML, kindFields, nil))
	assert.NoError(t, outputFieldsByFormat(outCmd, "table", kindFields, nil))
	assert.NoError(t, outputFieldsByFormat(outCmd, objectFormatJSON, nil, kindFields.AllFields))
}

func TestCycle121_BulkHelpers(t *testing.T) {
	// applyHybridProjectionToObjectMaps
	applyHybridProjectionToObjectMaps(nil, []string{"id"})
	applyHybridProjectionToObjectMaps([]map[string]any{}, []string{"id"})

	objs := []map[string]any{
		nil,
		{objects.FieldKeyKind: "backlog_item", "id": "BLI-1", "title": "Title 1", "notes": "Secret"},
	}
	applyHybridProjectionToObjectMaps(objs, []string{"id", "title"})
	assert.Nil(t, objs[0])
	assert.Equal(t, "BLI-1", objs[1]["id"])

	// applyHybridProjectionToBulkResult
	applyHybridProjectionToBulkResult(nil, []string{"id"})
	bulkRes := &storagepkg.BulkResult{
		Results: []map[string]any{
			{objects.FieldKeyKind: "backlog_item", "id": "BLI-2", "title": "Title 2"},
		},
	}
	applyHybridProjectionToBulkResult(bulkRes, []string{})
	applyHybridProjectionToBulkResult(bulkRes, []string{"id"})
	assert.Equal(t, "BLI-2", bulkRes.Results[0]["id"])

	// setCacheCheckerForBatchCreation
	setCacheCheckerForBatchCreation(nil)

	// outputBulkResult
	cmd := &cobra.Command{Use: "test"}
	outputBulkResult(cmd, bulkRes, "json", "create")
}

func TestCycle121_KindCommandHelpers(t *testing.T) {
	// firstHelpSentence
	assert.Equal(t, "", firstHelpSentence("", 100))
	assert.Equal(t, "", firstHelpSentence("   \n\n  ", 100))
	assert.Equal(t, "First line.", firstHelpSentence("Lifecycle: active\nFirst line. Second sentence.", 100))
	assert.Equal(t, "Hello.", firstHelpSentence("Hello. How are you?", 100))
	assert.Equal(t, "What?", firstHelpSentence("What? Really!", 100))
	assert.Equal(t, "Wow!", firstHelpSentence("Wow! Nice.", 100))

	longText := strings.Repeat("A", 120)
	truncated := firstHelpSentence(longText, 50)
	assert.True(t, len([]rune(truncated)) <= 50)
	assert.True(t, strings.HasSuffix(truncated, "…"))

	// kindCommandShort
	assert.Contains(t, kindCommandShort(""), "Operations for a specific object kind")
	assert.Contains(t, kindCommandShort("<kind>"), "Operations for a specific object kind")
	assert.NotEmpty(t, kindCommandShort("backlog_item"))

	// NewKindCmd and newKindCmdWithUse
	kCmd := NewKindCmd()
	assert.Equal(t, "<kind>", kCmd.Use)
	assert.NoError(t, kCmd.PersistentPreRunE(kCmd, []string{"backlog_item"}))
	assert.Equal(t, "backlog_item", kCmd.Annotations[objects.FieldKeyKind])
}

func TestCycle121_CoreDeleteReasonAndFlags(t *testing.T) {
	// parseDeleteFlags
	cmd := &cobra.Command{Use: "delete"}
	cmd.Flags().Bool("cascade", false, "")
	cmd.Flags().Bool("unlink-references", false, "")
	cmd.Flags().Bool("dry-run", false, "")

	// neither flag
	_, err := parseDeleteFlags(cmd, "custom refuse msg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "custom refuse msg")

	// both flags
	require.NoError(t, cmd.Flags().Set("cascade", "true"))
	require.NoError(t, cmd.Flags().Set("unlink-references", "true"))
	_, err = parseDeleteFlags(cmd, "custom refuse msg")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--unlink-references cannot be combined with --cascade")

	// cascade only
	require.NoError(t, cmd.Flags().Set("unlink-references", "false"))
	require.NoError(t, cmd.Flags().Set("dry-run", "true"))
	flags, err := parseDeleteFlags(cmd, "custom refuse msg")
	require.NoError(t, err)
	assert.True(t, flags.Cascade)
	assert.False(t, flags.UnlinkRefs)
	assert.True(t, flags.DryRun)

	// unlink-references only
	require.NoError(t, cmd.Flags().Set("cascade", "false"))
	require.NoError(t, cmd.Flags().Set("unlink-references", "true"))
	flags, err = parseDeleteFlags(cmd, "custom refuse msg")
	require.NoError(t, err)
	assert.False(t, flags.Cascade)
	assert.True(t, flags.UnlinkRefs)

	// withCoreDeleteReasonFromFlags
	reasonCmd := &cobra.Command{Use: "reason_test"}
	reasonCmd.Flags().String("reason-code", "", "")

	// empty reason
	ctx, err := withCoreDeleteReasonFromFlags(reasonCmd, context.Background())
	require.NoError(t, err)
	assert.False(t, pkgctx.GetAllowCoreObjectDelete(ctx))

	// short reason (< 30 runes)
	require.NoError(t, reasonCmd.Flags().Set("reason-code", "too short"))
	_, err = withCoreDeleteReasonFromFlags(reasonCmd, context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be at least 30 characters")

	// valid reason (>= 30 runes)
	validReason := "This is a legitimate reason exceeding 30 characters for test."
	require.NoError(t, reasonCmd.Flags().Set("reason-code", validReason))
	ctx, err = withCoreDeleteReasonFromFlags(reasonCmd, context.Background())
	require.NoError(t, err)
	assert.True(t, pkgctx.GetAllowCoreObjectDelete(ctx))
}

func TestCycle121_PplanHelpers(t *testing.T) {
	// backlogItemTerminalStatuses
	terms := backlogItemTerminalStatuses()
	assert.NotEmpty(t, terms)

	// statusFilterExcludingTerminal
	filter := statusFilterExcludingTerminal()
	assert.Contains(t, filter, "$nin")

	// findCurrentPriorityPlan with empty storage
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	_, err = findCurrentPriorityPlan(context.Background(), provider)
	assert.Error(t, err)
}

func TestCycle121_DaemonAdapter(t *testing.T) {
	cmd := NewDaemonCmd()
	assert.Equal(t, "daemon", cmd.Use)
	assert.True(t, cmd.Hidden)

	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	adapter := &daemonAdapter{fileStorage: provider}
	ctx := context.Background()

	rawYAML := []byte("id: BLI-DAEMON-DRAFT\nkind: backlog_item\ntitle: Draft\n")
	err = adapter.WriteObject(ctx, "BLI-DAEMON-DRAFT", "backlog_item", rawYAML, true)
	assert.NoError(t, err)

	err = adapter.RenameObject(ctx, "NONEXISTENT-OLD", "NONEXISTENT-NEW", "backlog_item")
	assert.Error(t, err)

	err = adapter.DeleteObject(ctx, "NONEXISTENT-OLD", "backlog_item")
	assert.Error(t, err)
}

func TestCycle121_InProcessCommands_Part1(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// Seed goal and requirement
	goalObj := map[string]any{
		objects.FieldKeyID:          "GOAL-COV-101",
		objects.FieldKeyKind:        objects.KindGoal,
		objects.FieldKeyTitle:       "Coverage Elevation Goal",
		objects.FieldKeyDescription: "Goal for testing graph relationships.",
		objects.FieldKeyStatus:      "active",
	}
	require.NoError(t, provider.Create(ctx, secCtx, goalObj))

	reqObj := map[string]any{
		objects.FieldKeyID:          "REQ-COV-101",
		objects.FieldKeyKind:        objects.KindRequirement,
		objects.FieldKeyTitle:       "Coverage Elevation Requirement",
		objects.FieldKeyDescription: "Requirement for testing relationships.",
		objects.FieldKeyStatus:      "originated",
		objects.FieldKeyGoalRefs:    []any{"GOAL-COV-101"},
	}
	require.NoError(t, provider.Create(ctx, secCtx, reqObj))

	// Path tests
	out, err := executeObjectCommand(t, tempDir, provider, "path", "REQ-COV-101", "GOAL-COV-101")
	assert.NoError(t, err, "path output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "path", "GOAL-COV-101", "NON-EXISTENT")
	assert.NoError(t, err, "path output: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "path")
	assert.Error(t, err)

	// Neighbors tests
	out, err = executeObjectCommand(t, tempDir, provider, "neighbors", "REQ-COV-101")
	assert.NoError(t, err, "neighbors output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "neighbors", "REQ-COV-101", "--direction", "outgoing")
	assert.NoError(t, err, "neighbors outgoing: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "neighbors", "REQ-COV-101", "--direction", "incoming")
	assert.NoError(t, err, "neighbors incoming: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "neighbors", "REQ-COV-101", "--direction", "invalid_dir")
	assert.Error(t, err)

	_, err = executeObjectCommand(t, tempDir, provider, "neighbors")
	assert.Error(t, err)

	// Related tests
	out, err = executeObjectCommand(t, tempDir, provider, "related", "REQ-COV-101", "--depth", "1")
	assert.NoError(t, err, "related output: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "related")
	assert.Error(t, err)

	// Rename tests
	out, err = executeObjectCommand(t, tempDir, provider, "rename", "REQ-COV-101", "REQ-COV-RENAMED", "--dry-run")
	assert.NoError(t, err, "rename dry-run output: %s", out)
	assert.Contains(t, out, "Would rename object")

	_, err = executeObjectCommand(t, tempDir, provider, "rename")
	assert.Error(t, err)

	_, err = executeObjectCommand(t, tempDir, provider, "rename", "NONEXISTENT", "TARGET", "--dry-run")
	assert.Error(t, err)
}

func TestCycle121_InProcessCommands_Part2(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	// Seed backlog item for park and delete
	bliObj := map[string]any{
		objects.FieldKeyID:                       "BLI-PARK-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyStatus:                   "exploring",
		objects.FieldKeyProblemStatement:         "Problem statement for parking test with sufficient length.",
		objects.FieldKeyDescription:              "Description for parking test with sufficient length.",
		objects.FieldKeyAcceptanceConsiderations: "Acceptance considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyTitle:                    "Park Test Backlog Item",
	}
	require.NoError(t, provider.Create(ctx, secCtx, bliObj))

	// Park tests
	out, err := executeObjectCommand(t, tempDir, provider, "park", "BLI-PARK-001", "--to", "deferred")
	assert.NoError(t, err, "park output: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "park", "BLI-PARK-001", "--to", "invalid_stage")
	assert.Error(t, err)

	_, err = executeObjectCommand(t, tempDir, provider, "park", "BLI-PARK-001")
	assert.Error(t, err)

	_, err = executeObjectCommand(t, tempDir, provider, "park")
	assert.Error(t, err)

	// Fields root command
	out, err = executeObjectCommand(t, tempDir, provider, "fields", "--list-kinds")
	assert.NoError(t, err, "fields --list-kinds output: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "fields", "--list-kinds", "--format", "json")
	assert.NoError(t, err, "fields json output: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "fields")
	assert.Error(t, err)

	// Template tests
	out, err = executeObjectCommand(t, tempDir, provider, "template", "backlog_item", "--format", "json")
	assert.NoError(t, err, "template json: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "template", "backlog_item", "--format", "yaml")
	assert.NoError(t, err, "template yaml: %s", out)

	// Draft sweep & promote
	out, err = executeObjectCommand(t, tempDir, provider, "draft", "sweep", "--dry-run", "--force")
	assert.NoError(t, err, "draft sweep dry-run: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "draft", "promote", "--dry-run")
	assert.NoError(t, err, "draft promote dry-run: %s", out)

	// Delete tests
	out, err = executeObjectCommand(t, tempDir, provider, "delete", "BLI-PARK-001", "--dry-run", "--unlink-references")
	assert.NoError(t, err, "delete dry-run: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "delete", "BLI-PARK-001")
	assert.Error(t, err, "delete without flags should fail")

	_, err = executeObjectCommand(t, tempDir, provider, "delete", "BLI-PARK-001", "--cascade", "--unlink-references")
	assert.Error(t, err, "conflicting delete flags should fail")

	_, err = executeObjectCommand(t, tempDir, provider, "delete")
	assert.Error(t, err)

	// Bulk operations
	testCycle121BulkOperations(t, tempDir, provider)
}

func testCycle121BulkOperations(t *testing.T, tempDir string, provider storagepkg.ObjectStorageProvider) {
	// Bulk get
	out, err := executeObjectCommand(t, tempDir, provider, "bulk", "get", "--ids", "BLI-PARK-001")
	assert.NoError(t, err, "bulk get: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "bulk", "get")
	assert.Error(t, err)

	// Bulk delete
	out, err = executeObjectCommand(t, tempDir, provider, "bulk", "delete", "--ids", "BLI-PARK-001", "--dry-run", "--unlink-references")
	assert.NoError(t, err, "bulk delete dry-run: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "bulk", "delete", "--ids", "BLI-PARK-001")
	assert.Error(t, err)

	_, err = executeObjectCommand(t, tempDir, provider, "bulk", "delete", "--ids", "BLI-PARK-001", "--cascade", "--unlink-references")
	assert.Error(t, err)

	// Bulk create
	bulkFile := filepath.Join(tempDir, "bulk_create_items.yaml")
	itemsContent := `- title: Bulk Create Item 1
  description: Valid description for bulk created item 1 with sufficient length.
  problem_statement: Valid problem statement for item 1 with sufficient length.
  priority: medium
  priority_tier: P2
`
	err = fileutil.WriteFile(bulkFile, []byte(itemsContent), 0644)
	require.NoError(t, err)

	out, err = executeObjectCommand(t, tempDir, provider, "bulk", "create", "backlog_item", "--file", bulkFile)
	assert.NoError(t, err, "bulk create: %s", out)

	// Bulk create negative: missing kind
	_, err = executeObjectCommand(t, tempDir, provider, "bulk", "create", "--file", bulkFile)
	assert.Error(t, err)

	// Bulk create negative: missing file
	_, err = executeObjectCommand(t, tempDir, provider, "bulk", "create", "backlog_item")
	assert.Error(t, err)
}

func TestCycle121_InspectHelpers(t *testing.T) {
	assert.Equal(t, "fallback", defaultStr("", "fallback"))
	assert.Equal(t, "fallback", defaultStr("   ", "fallback"))
	assert.Equal(t, "hello", defaultStr("hello", "fallback"))

	assert.Equal(t, "short", truncateString("short", 10))
	assert.Equal(t, "ab", truncateString("abc", 2))
	assert.Equal(t, "test...", truncateString("testinglonger", 7))

	obj := map[string]any{"status": "active", "kind": "backlog_item"}
	assert.True(t, matchesFilterExpr(obj, []string{"status=active"}))
	assert.False(t, matchesFilterExpr(obj, []string{"status=closed"}))
	assert.True(t, matchesFilterExpr(obj, []string{"status!=closed"}))
	assert.False(t, matchesFilterExpr(obj, []string{"status!=active"}))
	assert.True(t, matchesFilterExpr(obj, []string{""}))

	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	bliObj := map[string]any{
		objects.FieldKeyID:                       "BLI-INSPECT-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyStatus:                   "exploring",
		objects.FieldKeyProblemStatement:         "Problem statement for inspect test.",
		objects.FieldKeyDescription:              "Description for inspect test.",
		objects.FieldKeyAcceptanceConsiderations: "Acceptance considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyTitle:                    "Inspect Test Backlog Item",
	}
	require.NoError(t, provider.Create(ctx, secCtx, bliObj))

	out, err := executeObjectCommand(t, tempDir, provider, "inspect", "BLI-INSPECT-001", "--format", "json")
	assert.NoError(t, err, "inspect single object: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "inspect", "backlog_item", "--format", "json")
	assert.NoError(t, err, "inspect kind: %s", out)

	out, err = executeObjectCommand(t, tempDir, provider, "inspect", "--format", "json")
	assert.NoError(t, err, "inspect all: %s", out)

	_, err = executeObjectCommand(t, tempDir, provider, "inspect", "NONEXISTENT", "--format", "json")
	assert.Error(t, err)
}

func TestCycle121_CreateHelpersAndCLI(t *testing.T) {
	// handleKindSynonym
	synData := map[string]any{
		objects.FieldKeyKind: "task",
	}
	handleKindSynonym(synData)
	assert.Equal(t, objects.KindSynonym, synData[objects.FieldKeyKind])
	assert.Equal(t, "task", synData[objects.FieldKeyTargetKind])

	// lifecycleAllowsTransition
	assert.False(t, lifecycleAllowsTransition(nil, "a", "b"))

	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	// Create CLI
	createPayload := `{"title":"Created Item","description":"Valid description with sufficient length for validation.","problem_statement":"Valid problem statement with sufficient length.","priority":"low","priority_tier":"P3"}`
	out, err := executeObjectCommand(t, tempDir, provider, "create", "backlog_item", "--data", createPayload)
	assert.NoError(t, err, "create output: %s", out)

	// Create dry-run
	out, err = executeObjectCommand(t, tempDir, provider, "create", "backlog_item", "--data", createPayload, "--dry-run")
	assert.NoError(t, err, "create dry-run: %s", out)

	// Create negative: missing kind
	_, err = executeObjectCommand(t, tempDir, provider, "create")
	assert.Error(t, err)

	// Create negative: mismatched kind in payload
	mismatchedPayload := `{"kind":"goal","title":"Mismatched"}`
	_, err = executeObjectCommand(t, tempDir, provider, "create", "backlog_item", "--data", mismatchedPayload)
	assert.Error(t, err)
}

func TestCycle121_ExportAndImport(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	bliObj := map[string]any{
		objects.FieldKeyID:                       "BLI-EXP-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyStatus:                   "planned",
		objects.FieldKeyProblemStatement:         "Problem statement for export test with sufficient length.",
		objects.FieldKeyDescription:              "Description for export test with sufficient length.",
		objects.FieldKeyAcceptanceConsiderations: "Acceptance considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyPriorityPlanRef:          "PRI-TEST-PLAN",
		objects.FieldKeyMilestoneRefs:            []any{"MIL-001"},
		objects.FieldKeyTitle:                    "Export Test Backlog Item",
	}
	require.NoError(t, provider.Create(ctx, secCtx, bliObj))

	// Export stdout
	out, err := executeObjectCommand(t, tempDir, provider, "export", "backlog_item", "-o", "-")
	assert.NoError(t, err, "export stdout: %s", out)
	assert.Contains(t, out, "BLI-EXP-001")

	// Export json
	out, err = executeObjectCommand(t, tempDir, provider, "export", "backlog_item", "--format", "json", "-o", "-")
	assert.NoError(t, err, "export json: %s", out)
	assert.Contains(t, out, "BLI-EXP-001")

	// Export negative: missing kind
	_, err = executeObjectCommand(t, tempDir, provider, "export")
	assert.Error(t, err)

	// Import
	importFile := filepath.Join(tempDir, "import_test_items.yaml")
	importContent := `- id: BLI-IMP-001
  kind: backlog_item
  title: Imported Backlog Item
  description: Valid description for imported item with sufficient length.
  problem_statement: Valid problem statement for imported item.
  priority: medium
  priority_tier: P2
`
	err = fileutil.WriteFile(importFile, []byte(importContent), 0644)
	require.NoError(t, err)

	// Import dry-run
	out, err = executeObjectCommand(t, tempDir, provider, "import", "--file", importFile, "--dry-run")
	assert.NoError(t, err, "import dry-run: %s", out)

	// Import create_only
	out, err = executeObjectCommand(t, tempDir, provider, "import", "--file", importFile, "--mode", "create_only")
	assert.NoError(t, err, "import create_only: %s", out)

	// Import negative: missing file
	_, err = executeObjectCommand(t, tempDir, provider, "import")
	assert.Error(t, err)
}

func TestCycle121_MoveAndPolicyStudio(t *testing.T) {
	// Policy Studio DSL evaluation
	obj := map[string]any{
		"status":   "open",
		"title":    "critical bug fix needed",
		"priority": "high",
		"count":    10,
	}

	match, err := EvaluateDSLExpression("status == 'open'", obj)
	require.NoError(t, err)
	assert.True(t, match)

	match, err = EvaluateDSLExpression("status != 'open'", obj)
	require.NoError(t, err)
	assert.False(t, match)

	match, err = EvaluateDSLExpression("title contains 'bug'", obj)
	require.NoError(t, err)
	assert.True(t, match)

	match, err = EvaluateDSLExpression("title not_contains 'feature'", obj)
	require.NoError(t, err)
	assert.True(t, match)

	match, err = EvaluateDSLExpression("status is_populated", obj)
	require.NoError(t, err)
	assert.True(t, match)

	match, err = EvaluateDSLExpression("status is_not_empty", obj)
	require.NoError(t, err)
	assert.True(t, match)

	match, err = EvaluateDSLExpression("missing_key is_empty", obj)
	require.NoError(t, err)
	assert.True(t, match)

	// SuggestDSLTokens
	suggs := SuggestDSLTokens("backlog_item", "")
	assert.NotEmpty(t, suggs)

	suggs = SuggestDSLTokens("backlog_item", "status &&")
	assert.NotEmpty(t, suggs)

	suggs = SuggestDSLTokens("backlog_item", "status ==")
	assert.NotEmpty(t, suggs)

	// DefaultPolicyRulesForKind
	rules := DefaultPolicyRulesForKind("backlog_item")
	assert.NotEmpty(t, rules)

	// Move command
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storagepkg.NewFileObjectStorageForTest(tempDir)
	require.NoError(t, err)
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	bliObj := map[string]any{
		objects.FieldKeyID:                       "BLI-MOVE-001",
		objects.FieldKeyKind:                     objects.KindBacklogItem,
		objects.FieldKeyStatus:                   "exploring",
		objects.FieldKeyProblemStatement:         "Problem statement for move test with sufficient length.",
		objects.FieldKeyDescription:              "Description for move test with sufficient length.",
		objects.FieldKeyAcceptanceConsiderations: "Acceptance considerations.",
		objects.FieldKeyPriority:                 "medium",
		objects.FieldKeyPriorityTier:             "P2",
		objects.FieldKeyTitle:                    "Move Test Backlog Item",
	}
	require.NoError(t, provider.Create(ctx, secCtx, bliObj))

	// Move across schemas triggers validation guard
	_, err = executeObjectCommand(t, tempDir, provider, "move", "BLI-MOVE-001", "--kind", "goal", "--update-references")
	assert.Error(t, err)

	// Move negative: missing args
	_, err = executeObjectCommand(t, tempDir, provider, "move")
	assert.Error(t, err)

	// Move negative: nonexistent kind
	_, err = executeObjectCommand(t, tempDir, provider, "move", "BLI-MOVE-001", "--kind", "nonexistent_xyz")
	assert.Error(t, err)
}
