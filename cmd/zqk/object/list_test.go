package object

import (
	stdctx "context"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
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
	f := cmd.Flags().Lookup("kind")
	if f == nil {
		t.Fatal("expected --kind flag to be registered on list command")
	}
	if f.DefValue != "" {
		t.Errorf("expected default value empty, got %q", f.DefValue)
	}
}

func TestInProcess_List_SingleKind_Matrix(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	b1 := plannedBLI("BLI-LST-101", "Alpha Item", map[string]any{objects.FieldKeyPriority: "low"})
	b2 := plannedBLI("BLI-LST-102", "Beta Item", map[string]any{objects.FieldKeyPriority: "high"})
	b3 := plannedBLI("BLI-LST-103", "Gamma Item", map[string]any{objects.FieldKeyPriority: "critical"})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, b1, objectStatusPlanned)
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, b2, objectStatusPlanned)
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, b3, objectStatusPlanned)

	verifySingleKindBasicQueries(t, tempDir, provider)
	verifySingleKindProjectionAndLimits(t, tempDir, provider)
}

func verifySingleKindBasicQueries(t *testing.T, tempDir string, provider storage.ObjectStorageProvider) {
	t.Helper()
	out, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item")
	if err != nil || !strings.Contains(out, "BLI-LST-101") || !strings.Contains(out, "BLI-LST-102") || !strings.Contains(out, "BLI-LST-103") {
		t.Fatalf("basic list failed: out=%s, err=%v", out, err)
	}

	outFilter, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--filter", "priority=high")
	if err != nil || !strings.Contains(outFilter, "BLI-LST-102") || strings.Contains(outFilter, "BLI-LST-101") {
		t.Fatalf("filter list failed: out=%s, err=%v", outFilter, err)
	}

	outCount, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--count")
	if err != nil || !strings.Contains(outCount, "3") {
		t.Fatalf("count failed: out=%s, err=%v", outCount, err)
	}

	outSort, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--sort-by", "title", "--sort-asc")
	if err != nil || !strings.Contains(outSort, "Alpha Item") {
		t.Fatalf("sort failed: out=%s, err=%v", outSort, err)
	}
}

func verifySingleKindProjectionAndLimits(t *testing.T, tempDir string, provider storage.ObjectStorageProvider) {
	t.Helper()
	outGroup, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--group-by", "status")
	if err != nil || !strings.Contains(outGroup, "planned") {
		t.Fatalf("group-by failed: out=%s, err=%v", outGroup, err)
	}

	outFields, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--fields", "id,title,priority")
	if err != nil || !strings.Contains(outFields, "BLI-LST-101") {
		t.Fatalf("fields projection failed: out=%s, err=%v", outFields, err)
	}

	outYAML, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--format", "yaml")
	if err != nil || !strings.Contains(outYAML, "BLI-LST-101") {
		t.Fatalf("yaml format failed: out=%s, err=%v", outYAML, err)
	}

	outLim, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--limit", "1", "--offset", "1")
	if err != nil || outLim == "" {
		t.Fatalf("limit/offset failed: out=%s, err=%v", outLim, err)
	}

	if _, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--filter", "nonexistent_xyz=123"); err == nil {
		t.Fatal("expected error for invalid filter field")
	}
}

func TestInProcess_List_AllKinds_Matrix(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

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

func TestObjectList_KindFlagSymmetry(t *testing.T) {
	tempDir, provider, secCtx, cleanup := setupInProcessTestContext(t)
	defer cleanup()

	b1 := plannedBLI("BLI-KND-001", "Kind Symmetry Item", map[string]any{objects.FieldKeyPriority: "medium"})
	storage.CreateCASVisible(t, provider, stdctx.Background(), secCtx, b1, objectStatusPlanned)

	// 1. Positional only: zqk object list backlog_item
	outPos, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item")
	if err != nil || !strings.Contains(outPos, "BLI-KND-001") {
		t.Fatalf("positional list failed: err=%v, out=%s", err, outPos)
	}

	// 2. Flag only: zqk object list --kind backlog_item
	outFlag, err := executeObjectCommand(t, tempDir, provider, "list", "--kind", "backlog_item")
	if err != nil || !strings.Contains(outFlag, "BLI-KND-001") {
		t.Fatalf("flag list failed: err=%v, out=%s", err, outFlag)
	}

	// 3. Matching both (identical): zqk object list backlog_item --kind backlog_item
	outBoth, err := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--kind", "backlog_item")
	if err != nil || !strings.Contains(outBoth, "BLI-KND-001") {
		t.Fatalf("matching both list failed: err=%v, out=%s", err, outBoth)
	}

	// 4. Matching both (synonym alias + canonical): zqk object list bli --kind backlog_item
	outSynonym, err := executeObjectCommand(t, tempDir, provider, "list", "bli", "--kind", "backlog_item")
	if err != nil || !strings.Contains(outSynonym, "BLI-KND-001") {
		t.Fatalf("matching synonym list failed: err=%v, out=%s", err, outSynonym)
	}

	// 5. Mismatching contradiction: zqk object list backlog_item --kind requirement
	_, errMismatch := executeObjectCommand(t, tempDir, provider, "list", "backlog_item", "--kind", "requirement")
	if errMismatch == nil {
		t.Fatal("expected error for contradicting positional and --kind flag, got nil")
	}
	if !strings.Contains(errMismatch.Error(), "contradicts") {
		t.Fatalf("expected error mentioning contradicts, got: %v", errMismatch)
	}

	// 6. Invalid kind in flag: zqk object list --kind nonexistent_kind_xyz
	_, errInvalidFlag := executeObjectCommand(t, tempDir, provider, "list", "--kind", "nonexistent_kind_xyz")
	if errInvalidFlag == nil {
		t.Fatal("expected error for invalid kind flag, got nil")
	}
	if !strings.Contains(errInvalidFlag.Error(), "unknown kind") && !strings.Contains(errInvalidFlag.Error(), "invalid kind") {
		t.Fatalf("expected unknown/invalid kind error, got: %v", errInvalidFlag)
	}

	// 7. Invalid kind in positional: zqk object list nonexistent_kind_xyz
	_, errInvalidPos := executeObjectCommand(t, tempDir, provider, "list", "nonexistent_kind_xyz")
	if errInvalidPos == nil {
		t.Fatal("expected error for invalid positional kind, got nil")
	}
	if !strings.Contains(errInvalidPos.Error(), "unknown kind") && !strings.Contains(errInvalidPos.Error(), "invalid kind") {
		t.Fatalf("expected unknown/invalid kind error, got: %v", errInvalidPos)
	}

	// 8. Neither provided: zqk object list (default lists all kinds)
	outAll, errAll := executeObjectCommand(t, tempDir, provider, "list")
	if errAll != nil || !strings.Contains(outAll, "BLI-KND-001") {
		t.Fatalf("default list all kinds failed: err=%v, out=%s", errAll, outAll)
	}
}
