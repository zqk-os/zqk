package object

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestObjectList_IncludesGlossaryTerm(t *testing.T) {
	t.Parallel()
	if ShouldSkipInternalKind("glossary_term", false) {
		t.Fatal("glossary_term should be a public kind and not skipped in non-elevated object list")
	}
}

func TestNewListCmd_Structure(t *testing.T) {
	cmd := NewListCmd()
	if cmd == nil {
		t.Fatal("expected NewListCmd to return non-nil")
	}
	if cmd.Use == "" {
		t.Error("expected Use to be set")
	}
	hasLs := false
	hasFind := false
	for _, a := range cmd.Aliases {
		if a == "ls" {
			hasLs = true
		}
		if a == "find" {
			hasFind = true
		}
	}
	if !hasLs || !hasFind {
		t.Errorf("expected aliases ls and find, got: %v", cmd.Aliases)
	}
	if cmd.Annotations[AnnotationKindValidate] != KindValidatePositional0Opt {
		t.Errorf("expected KindValidatePositional0Opt annotation")
	}
}

func TestInProcess_List_SingleKind_Matrix(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedBulkUpdateSupportObjects(t, provider)

	secCtx := pkgctx.NewSystemSecurityContext()

	// Seed 3 backlog items with distinct priorities
	b1 := plannedBLI("BLI-LST-101", "Alpha Item", map[string]any{
		objects.FieldKeyPriority: "low",
	})
	b2 := plannedBLI("BLI-LST-102", "Beta Item", map[string]any{
		objects.FieldKeyPriority: "high",
	})
	b3 := plannedBLI("BLI-LST-103", "Gamma Item", map[string]any{
		objects.FieldKeyPriority: "critical",
	})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, b1, objectStatusPlanned)
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, b2, objectStatusPlanned)
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, b3, objectStatusPlanned)

	// 1. Basic list
	out, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item")
	if err != nil {
		t.Fatalf("list backlog_item failed: %v", err)
	}
	if !strings.Contains(out, "BLI-LST-101") || !strings.Contains(out, "BLI-LST-102") || !strings.Contains(out, "BLI-LST-103") {
		t.Errorf("expected all 3 items in list output, got:\n%s", out)
	}

	// 2. Filter by priority
	outFilter, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--filter", "priority=high")
	if err != nil {
		t.Fatalf("list with filter failed: %v", err)
	}
	if !strings.Contains(outFilter, "BLI-LST-102") {
		t.Errorf("expected BLI-LST-102 in filter output, got:\n%s", outFilter)
	}
	if strings.Contains(outFilter, "BLI-LST-101") || strings.Contains(outFilter, "BLI-LST-103") {
		t.Errorf("expected only high priority item in filtered output, got:\n%s", outFilter)
	}

	// 3. Count only
	outCount, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--count")
	if err != nil {
		t.Fatalf("list --count failed: %v", err)
	}
	if !strings.Contains(outCount, "3") {
		t.Errorf("expected count 3 in output, got: %s", outCount)
	}

	// 4. Sort-by title ascending
	outSort, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--sort-by", "title", "--sort-asc")
	if err != nil {
		t.Fatalf("list with sort failed: %v", err)
	}
	if !strings.Contains(outSort, "Alpha Item") {
		t.Errorf("expected Alpha Item in sorted output, got:\n%s", outSort)
	}

	// 5. Group by status
	outGroup, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--group-by", "status")
	if err != nil {
		t.Fatalf("list with group-by failed: %v", err)
	}
	if !strings.Contains(outGroup, "planned") {
		t.Errorf("expected group planned in output, got:\n%s", outGroup)
	}

	// 6. Custom fields projection
	outFields, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--fields", "id,title,priority")
	if err != nil {
		t.Fatalf("list with --fields failed: %v", err)
	}
	if !strings.Contains(outFields, "BLI-LST-101") {
		t.Errorf("expected BLI-LST-101 in projection output, got:\n%s", outFields)
	}

	// 7. Output format YAML
	outYAML, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--format", "yaml")
	if err != nil {
		t.Fatalf("list with --format yaml failed: %v", err)
	}
	if !strings.Contains(outYAML, "BLI-LST-101") {
		t.Errorf("expected YAML output with item ID, got:\n%s", outYAML)
	}

	// 8. Limit and Offset
	outLim, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--limit", "1", "--offset", "1")
	if err != nil {
		t.Fatalf("list with limit/offset failed: %v", err)
	}
	if outLim == "" {
		t.Error("expected non-empty output with limit and offset")
	}

	// 9. Error path: invalid filter field
	_, errInvalidFilter := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--filter", "nonexistent_field_xyz=123")
	if errInvalidFilter == nil {
		t.Error("expected error for invalid filter field")
	}
}

func TestInProcess_List_AllKinds_Matrix(t *testing.T) {
	tempDir, cleanup := setupTestProject(t)
	defer cleanup()

	provider, err := storage.NewFileObjectStorageForTest(tempDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	testkit.RegisterStorageTestCleanup(t, tempDir, provider)
	seedBulkUpdateSupportObjects(t, provider)

	secCtx := pkgctx.NewSystemSecurityContext()

	// Seed objects of different kinds
	b1 := plannedBLI("BLI-ALL-101", "All Kinds BLI")
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, b1, objectStatusPlanned)

	// 1. List all kinds default (grouped by kind)
	outAll, err := executeObjectCommand(t, tempDir, provider, "list")
	if err != nil {
		t.Fatalf("list all kinds failed: %v", err)
	}
	if !strings.Contains(outAll, "BLI-ALL-101") && !strings.Contains(outAll, "backlog_item") {
		t.Errorf("expected backlog_item in all kinds output, got:\n%s", outAll)
	}

	// 2. List all kinds with --all-kinds flag
	outFlag, err := executeObjectCommand(t, tempDir, provider, "list", "--all-kinds")
	if err != nil {
		t.Fatalf("list --all-kinds failed: %v", err)
	}
	if !strings.Contains(outFlag, "backlog_item") {
		t.Errorf("expected backlog_item in --all-kinds output, got:\n%s", outFlag)
	}

	// 3. List all kinds grouped by field (status)
	outStatusGroup, err := executeObjectCommand(t, tempDir, provider, "list", "--group-by", "status")
	if err != nil {
		t.Fatalf("list all kinds with --group-by status failed: %v", err)
	}
	if !strings.Contains(outStatusGroup, "planned") && !strings.Contains(outStatusGroup, "validated") {
		t.Errorf("expected group status in output, got:\n%s", outStatusGroup)
	}

	// 4. List all kinds count
	outCount, err := executeObjectCommand(t, tempDir, provider, "list", "--count")
	if err != nil {
		t.Fatalf("list all kinds --count failed: %v", err)
	}
	if outCount == "" {
		t.Error("expected non-empty count output")
	}

	// 5. List with --internal (elevated mode requires enterprise license)
	_, errInternal := executeObjectCommand(t, tempDir, provider, "list", "--internal")
	if errInternal == nil || !strings.Contains(errInternal.Error(), "elevated object access (--internal) denied") {
		t.Errorf("expected license check error for --internal, got: %v", errInternal)
	}
}

func TestList_UnitHelpers(t *testing.T) {
	// 1. applyQuietProfileFilters without quiet profile (no-op)
	cmdNonQuiet := &cobra.Command{}
	filtersNonQuiet := map[string]any{"title": "Test"}
	applyQuietProfileFilters(cmdNonQuiet, filtersNonQuiet)
	if _, ok := filtersNonQuiet[objects.FieldKeyStatus]; ok {
		t.Error("expected status filter not to be added for non-quiet profile")
	}

	// 2. groupObjectsByField
	objs := []map[string]any{
		{"id": "1", "status": "planned"},
		{"id": "2", "status": "planned"},
		{"id": "3", "status": "complete"},
		{"id": "4"}, // missing status -> (none)
	}
	grouped := groupObjectsByField(objs, "status")
	if len(grouped["planned"]) != 2 {
		t.Errorf("expected 2 planned objects, got: %d", len(grouped["planned"]))
	}
	if len(grouped["complete"]) != 1 {
		t.Errorf("expected 1 complete object, got: %d", len(grouped["complete"]))
	}
	if len(grouped["(none)"]) != 1 {
		t.Errorf("expected 1 (none) object, got: %d", len(grouped["(none)"]))
	}

	// 3. applyGroupLimits
	limited := applyGroupLimits(grouped, 1, 0)
	for groupKey, groupObjs := range limited {
		if len(groupObjs) > 1 {
			t.Errorf("group %s has %d objects, want at most 1", groupKey, len(groupObjs))
		}
	}

	// 4. buildGroupedResult
	res := buildGroupedResult(grouped, objs)
	if res == nil || len(res.Groups) != len(grouped) {
		t.Fatalf("unexpected buildGroupedResult: %v", res)
	}
	if res.Meta["total_count"] != 4 {
		t.Errorf("expected total_count 4, got: %v", res.Meta["total_count"])
	}

	// 5. calculateEffectiveLimitFromContext
	storageCtx := &storage.StorageContext{MaxPageSize: 20}
	if lim := calculateEffectiveLimitFromContext(5, storageCtx); lim != 5 {
		t.Errorf("expected 5 when flag limit is set, got: %d", lim)
	}
	if lim := calculateEffectiveLimitFromContext(0, storageCtx); lim != 0 {
		t.Errorf("expected 0 when flag limit is 0, got: %d", lim)
	}

	// 6. ShouldSkipInternalKind
	if !ShouldSkipInternalKind("audit_event", false) {
		t.Error("expected audit_event to be skipped in non-elevated mode")
	}
	if ShouldSkipInternalKind("audit_event", true) {
		t.Error("expected audit_event not to be skipped in elevated mode")
	}
	if ShouldSkipInternalKind("backlog_item", false) {
		t.Error("expected backlog_item never to be skipped")
	}
}
