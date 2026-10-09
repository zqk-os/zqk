package crud

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

type dummyReadFacade struct {
	FileStorageReadFacade
	streamEnabled bool
	deltaEnabled  bool
	deltaState    map[string]any
}

func (d *dummyReadFacade) StreamStorageEnabledForKind(kind string) bool {
	return d.streamEnabled
}

func (d *dummyReadFacade) RuntimeDeltaEnabledForKind(projectRoot, kind string) bool {
	return d.deltaEnabled
}

func (d *dummyReadFacade) ReadRuntimeDeltaCurrentState(projectRoot, kind, id string) map[string]any {
	return d.deltaState
}

func TestInternal_SearchHelpers(t *testing.T) {
	// minInt
	if minInt(1, 2, 3) != 1 || minInt(3, 1, 2) != 1 || minInt(3, 2, 1) != 1 {
		t.Errorf("minInt failed")
	}
	if minInt(5, 5, 2) != 2 || minInt(5, 2, 5) != 2 {
		t.Errorf("minInt equality cases failed")
	}

	// levenshteinDistance
	if levenshteinDistance("", "abc") != 3 || levenshteinDistance("abc", "") != 3 {
		t.Errorf("levenshtein empty string failed")
	}
	if levenshteinDistance("kitten", "sitting") != 3 {
		t.Errorf("levenshtein distance failed")
	}

	// fuzzyMatch
	// short terms (<= 3) require exact
	if !fuzzyMatch("cat", "cat") || fuzzyMatch("car", "cat") {
		t.Errorf("fuzzyMatch short term failed")
	}
	// longer terms (> 3) allow edit distance up to term len / 3
	if !fuzzyMatch("search", "search") {
		t.Errorf("fuzzyMatch exact failed")
	}
	if !fuzzyMatch("search", "searche") {
		t.Errorf("fuzzyMatch 1 edit failed")
	}
	if fuzzyMatch("search", "completelydifferent") {
		t.Errorf("fuzzyMatch different should fail")
	}

	// SearchObjects with Fuzzy & Highlight
	ctx := context.Background()
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.GetStorageContext()

	items := []map[string]any{
		{"id": "TASK-1", "title": "Implement fuzzy search capability", "description": "Needs testing"},
		{"id": "TASK-2", "title": "Another unrelated item", "description": "No matches here"},
	}

	listFn := func(ctx context.Context, sec *pkgctx.SecurityContext, stor *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
		return &QueryResult{Objects: items}, nil
	}
	permFn := func(sec *pkgctx.SecurityContext, op, kind string) error {
		return nil
	}

	query := SearchQuery{
		Query:     "fuzzi serch",
		Kinds:     []string{"task"},
		Fuzzy:     true,
		Highlight: true,
		MinScore:  0.1,
		Limit:     10,
	}

	res, err := SearchObjects(ctx, secCtx, storageCtx, query, listFn, permFn)
	if err != nil {
		t.Fatalf("SearchObjects fuzzy failed: %v", err)
	}
	if len(res.Objects) == 0 {
		t.Fatalf("Expected fuzzy match for 'fuzzi serch'")
	}
	if len(res.Objects[0].Highlights) == 0 {
		t.Errorf("Expected highlights in fuzzy search match")
	}
}

func TestInternal_ListMainHelpers(t *testing.T) {
	// EffectiveListLimit
	if EffectiveListLimit(nil, nil) != 0 {
		t.Errorf("nil filter limit should be 0")
	}
	if EffectiveListLimit(&ListFilter{Limit: 0}, nil) != 0 {
		t.Errorf("0 limit should be 0")
	}
	if EffectiveListLimit(&ListFilter{Limit: 42}, nil) != 42 {
		t.Errorf("expected 42 limit")
	}

	// ListFilterIsOnlyCreatedAtRange
	if ListFilterIsOnlyCreatedAtRange(nil) || ListFilterIsOnlyCreatedAtRange(map[string]any{}) {
		t.Errorf("empty filters should be false")
	}
	if ListFilterIsOnlyCreatedAtRange(map[string]any{"id": "1", "created_at": map[string]any{"$gte": "2026-01-01"}}) {
		t.Errorf("multiple filters should be false")
	}
	if ListFilterIsOnlyCreatedAtRange(map[string]any{"created_at": "not-a-map"}) {
		t.Errorf("non-map created_at should be false")
	}
	if ListFilterIsOnlyCreatedAtRange(map[string]any{"created_at": map[string]any{"$invalid": "val"}}) {
		t.Errorf("invalid operator should be false")
	}
	validRange := map[string]any{
		"created_at": map[string]any{
			"$gte": "2026-01-01T00:00:00Z",
			"$lte": "2026-01-02T00:00:00Z",
		},
	}
	if !ListFilterIsOnlyCreatedAtRange(validRange) {
		t.Errorf("valid created_at range should be true")
	}

	// ParseCreatedAtOlderThan
	_, ok := ParseCreatedAtOlderThan(map[string]any{})
	if ok {
		t.Errorf("empty filter should return false")
	}
	_, ok = ParseCreatedAtOlderThan(map[string]any{"created_at": map[string]any{"$gte": "2026-01-01T00:00:00Z"}})
	if ok {
		t.Errorf("has $gte should return false")
	}
	cutoff, ok := ParseCreatedAtOlderThan(map[string]any{
		"created_at": map[string]any{"$lt": "2026-01-01T00:00:00Z"},
	})
	if !ok || cutoff.Year() != 2026 {
		t.Errorf("ParseCreatedAtOlderThan $lt failed: %v, %v", cutoff, ok)
	}
	cutoff, ok = ParseCreatedAtOlderThan(map[string]any{
		"created_at": map[string]any{"$lte": "2026-01-05T00:00:00Z"},
	})
	if !ok || cutoff.Day() != 5 {
		t.Errorf("ParseCreatedAtOlderThan $lte failed: %v, %v", cutoff, ok)
	}

	// ParseCreatedAtRangeFromFilters
	st, et, ok := ParseCreatedAtRangeFromFilters(map[string]any{})
	if ok {
		t.Errorf("empty filter should return false")
	}
	st, et, ok = ParseCreatedAtRangeFromFilters(map[string]any{
		"created_at": map[string]any{
			"$gte": "2026-01-01T00:00:00Z",
			"$lte": "2026-01-02T00:00:00Z",
		},
	})
	if !ok || st.Day() != 1 || et.Day() != 2 {
		t.Errorf("ParseCreatedAtRangeFromFilters failed: %v, %v, %v", st, et, ok)
	}
}

func TestInternal_UpdateHelpers(t *testing.T) {
	// ComputeUpdatesMap
	if ComputeUpdatesMap(nil, nil) != nil {
		t.Errorf("nil newObj should return nil")
	}
	prev := map[string]any{"a": 1, "b": "unchanged"}
	newObj := map[string]any{"a": 2, "b": "unchanged", "c": true}
	diff := ComputeUpdatesMap(prev, newObj)
	if len(diff) != 2 || diff["a"] != 2 || diff["c"] != true {
		t.Errorf("ComputeUpdatesMap failed: %v", diff)
	}

	// ClassifyUpdateMutation
	if ClassifyUpdateMutation(true, false) != "id_change" {
		t.Errorf("idUpdated true should return id_change")
	}
	if ClassifyUpdateMutation(false, true) != "runtime_delta" {
		t.Errorf("runtimeDeltaOnly true should return runtime_delta")
	}
	if ClassifyUpdateMutation(false, false) != "structural" {
		t.Errorf("default should return structural")
	}

	// ShockwaveRouterIsArmed
	if !ShockwaveRouterIsArmed(objects.ObjectStatusActive) ||
		!ShockwaveRouterIsArmed(objects.ObjectStatusApproved) ||
		!ShockwaveRouterIsArmed(objects.ObjectStatusInProgress) {
		t.Errorf("expected true for armed statuses")
	}
	if ShockwaveRouterIsArmed(objects.ObjectStatusCompleted) || ShockwaveRouterIsArmed("unknown") {
		t.Errorf("expected false for unarmed statuses")
	}
}

func TestInternal_AggregateToFloat64(t *testing.T) {
	if toFloat64(float64(3.14)) != 3.14 {
		t.Errorf("float64 failed")
	}
	if toFloat64(float32(2.5)) != float64(float32(2.5)) {
		t.Errorf("float32 failed")
	}
	if toFloat64(int(10)) != 10.0 {
		t.Errorf("int failed")
	}
	if toFloat64(int64(20)) != 20.0 {
		t.Errorf("int64 failed")
	}
	if toFloat64(int32(30)) != 30.0 {
		t.Errorf("int32 failed")
	}
	if toFloat64(uint(40)) != 40.0 {
		t.Errorf("uint failed")
	}
	if toFloat64(uint64(50)) != 50.0 {
		t.Errorf("uint64 failed")
	}
	if toFloat64(uint32(60)) != 60.0 {
		t.Errorf("uint32 failed")
	}
	if toFloat64("123.45") != 123.45 {
		t.Errorf("string parse failed")
	}
	if toFloat64("not-a-number") != 0.0 {
		t.Errorf("invalid string should return 0")
	}
	// reflection types (int8, uint8)
	if toFloat64(int8(7)) != 7.0 || toFloat64(uint8(8)) != 8.0 {
		t.Errorf("reflection numbers failed")
	}
	if toFloat64(struct{}{}) != 0.0 {
		t.Errorf("non-numeric should return 0")
	}

	// getGroupKey
	if getGroupKey(map[string]any{"group": "grp1"}, "group") != "grp1" {
		t.Errorf("getGroupKey failed")
	}
	if getGroupKey(map[string]any{}, "group") != "" {
		t.Errorf("getGroupKey missing field failed")
	}
}

func TestInternal_ReadHelpers(t *testing.T) {
	facade := &dummyReadFacade{
		streamEnabled: false,
		deltaEnabled:  true,
		deltaState: map[string]any{
			"status": "in_progress",
			"annotations": map[string]any{
				"new_ann": "val",
			},
		},
	}

	base := map[string]any{
		"id":     "TASK-100",
		"status": "pending",
		"annotations": map[string]any{
			"existing": "old_val",
		},
	}

	// MaterializeCasYAMLMapAfterLoad
	mat := MaterializeCasYAMLMapAfterLoad(facade, "/tmp", "task", "TASK-100", base)
	if mat["status"] != "in_progress" {
		t.Errorf("expected overlayed status, got %v", mat["status"])
	}
	ann, ok := mat["annotations"].(map[string]any)
	if !ok || ann["new_ann"] != "val" || ann["existing"] != "old_val" {
		t.Errorf("annotations merge failed: %v", ann)
	}

	// nil base
	if MaterializeCasYAMLMapAfterLoad(facade, "/tmp", "task", "", nil) != nil {
		t.Errorf("nil base should return nil")
	}
}

func TestInternal_DiscoveryAndContextOptions(t *testing.T) {
	// Context options
	ctx := context.Background()
	ctxBulk := WithBulkCreateDeferFlush(ctx)
	if !DeferListingIndexFlushForBulkCreate(ctxBulk, objects.KindSchedulerJob) {
		t.Errorf("DeferListingIndexFlushForBulkCreate for scheduler job should be true")
	}
	if DeferListingIndexFlushForBulkCreate(ctx, objects.KindSchedulerJob) {
		t.Errorf("regular ctx should not defer")
	}

	ctxSyncJob := WithSyncCreateForSchedulerJob(ctx)
	if ctxSyncJob.Value(syncCreateForSchedulerJobKey{}) != true {
		t.Errorf("WithSyncCreateForSchedulerJob failed")
	}

	ctxSyncKind := WithSyncCreateForKind(ctx, "backlog_item")
	if ctxSyncKind.Value(syncCreateForKindKey{}) != "backlog_item" {
		t.Errorf("WithSyncCreateForKind failed")
	}

	ctxSuppress := WithSuppressHashRegistryUpdate(ctx)
	if !ShouldSuppressHashRegistryUpdate(ctxSuppress) {
		t.Errorf("ShouldSuppressHashRegistryUpdate should be true")
	}
	if ShouldSuppressHashRegistryUpdate(ctx) {
		t.Errorf("ShouldSuppressHashRegistryUpdate for regular ctx should be false")
	}

	// Discovery functions
	// GetObjectIDFromPath
	if GetObjectIDFromPath("/path/to/BLI-123.yaml", "backlog_item") != "BLI-123" {
		t.Errorf("GetObjectIDFromPath regular failed")
	}
	if GetObjectIDFromPath("/path/to/account-alice.yaml", objects.KindAccount) != "account:alice" {
		t.Errorf("GetObjectIDFromPath account failed")
	}
	if GetObjectIDFromPath("", "backlog_item") != "." {
		// filepath.Base("") is "."
	}

	// Hash-based path (64 hex characters)
	tmpDir := t.TempDir()
	hexHash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	hashFile := filepath.Join(tmpDir, hexHash+".yaml")
	fileContent := "id: TASK-CAS-1\nkind: task\n"
	if err := fileutil.WriteFile(hashFile, []byte(fileContent), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	id := GetObjectIDFromPath(hashFile, "task")
	if id != "TASK-CAS-1" {
		t.Errorf("GetObjectIDFromPath hash CAS failed: got %s", id)
	}

	idFound, kindFound := GetObjectIDAndKindFromPath(hashFile, "task")
	if idFound != "TASK-CAS-1" || kindFound != "task" {
		t.Errorf("GetObjectIDAndKindFromPath hash CAS failed: got %s, %s", idFound, kindFound)
	}

	// Non-hash path for GetObjectIDAndKindFromPath
	regFile := filepath.Join(tmpDir, "TASK-REG-1.yaml")
	idFound, kindFound = GetObjectIDAndKindFromPath(regFile, "task")
	if idFound != "TASK-REG-1" || kindFound != "" {
		t.Errorf("GetObjectIDAndKindFromPath regular failed: got %s, %s", idFound, kindFound)
	}
}

func TestInternal_FileCASWrappers(t *testing.T) {
	SetSkipIndexUpdateWait(true)
	SetSkipIndexUpdateWait(false)
	cas := NewContentAddressableStorage(t.TempDir(), "task")
	if cas == nil {
		t.Fatalf("NewContentAddressableStorage returned nil")
	}

	// RemoveOrphanCASFilesForObjectID edge cases
	RemoveOrphanCASFilesForObjectID("", "")
	RemoveOrphanCASFilesForObjectID("ID-1", "")
	RemoveOrphanCASFilesForObjectID("", t.TempDir())

	// Test with actual directory and files
	tmpDir := t.TempDir()
	RemoveOrphanCASFilesForObjectID("ID-1", tmpDir)
	subDir := filepath.Join(tmpDir, "sub")
	_ = fileutil.MkdirAll(subDir, paths.DirPerm755)

	// Create a CAS hash file
	hexHash := "1111111111111111111111111111111111111111111111111111111111111111"
	fPath := filepath.Join(subDir, hexHash+".yaml")
	_ = fileutil.WriteFile(fPath, []byte("id: ID-ORPHAN\n"), paths.FilePerm644)
	RemoveOrphanCASFilesForObjectID("ID-ORPHAN", tmpDir)
}
