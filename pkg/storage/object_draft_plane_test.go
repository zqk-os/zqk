package storage

import (
	"path/filepath"
	"strings"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// TRACK: REQ-1785895564241296000-bf266adb

const objectStatusReview = "review"

func draftPlaneDocEntry(id, title, status string) map[string]any {
	return map[string]any{
		objects.FieldKeyID:                id,
		objects.FieldKeyKind:              objects.KindDocEntry,
		objects.FieldKeyTitle:             title,
		objects.FieldKeyStatus:            status,
		objects.FieldKeyPath:              "docs/" + id + ".md",
		objects.FieldKeySummary:           "draft plane test",
		objects.FieldKeyGroup:             "other",
		objects.FieldKeyContentSearchable: true,
		objects.FieldKeyGoalRefs:          []string{},
		objects.FieldKeyWorkstreamRefs:    []string{},
		objects.FieldKeyMilestoneRefs:     []string{},
		objects.FieldKeyRequirementRefs:   []string{},
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:         "ACC-1785920548450214012-68b850c0",
	}
}

func draftPlaneBacklogItem(id, title, status string) map[string]any {
	return map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         title,
		objects.FieldKeyStatus:        status,
		objects.FieldKeyDescription:   "conceptual draft-plane list omit test",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
	}
}

func listContainsID(result *QueryResult, id string) bool {
	if result == nil {
		return false
	}
	for _, obj := range result.Objects {
		if objects.GetString(obj, objects.FieldKeyID) == id {
			return true
		}
	}
	return false
}

func TestObjectDraftPlane_CreateGetPromote(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "DOC-1777000000000000000-draftplane01"
	if err := fileStorage.Create(ctx, secCtx, draftPlaneDocEntry(id, "Draft plane smoke", "draft")); err != nil {
		t.Fatalf("Create draft doc_entry: %v", err)
	}

	draftPath := ObjectDraftPlanePath(tmpDir, objects.KindDocEntry, id)
	if _, err := fileutil.Stat(draftPath); err != nil {
		t.Fatalf("expected draft-plane file at %s: %v", draftPath, err)
	}
	if !strings.Contains(draftPath, filepath.Join(paths.ProjectDataDir, paths.ObjectDraftsDir)) {
		t.Fatalf("draft path not under object_drafts: %s", draftPath)
	}

	cas, casErr := fileStorage.getContentAddressableStorage(objects.KindDocEntry)
	if casErr == nil && cas != nil {
		if _, hashErr := cas.GetHashForID(id); hashErr == nil {
			t.Fatalf("draft create should not register CAS id→hash for %s", id)
		}
	}

	var cachedID, cachedPath string
	id2 := "DOC-1777000000000000000-draftplane-cache"
	prevHandler := GetCacheOperationHandler()
	t.Cleanup(func() { SetCacheOperationHandler(prevHandler) })
	SetCacheOperationHandler(func(cacheCtx *pkgctx.CacheContext) error {
		if cacheCtx.NewID == id2 {
			cachedID = cacheCtx.NewID
			cachedPath = cacheCtx.FilePath
		}
		return nil
	})
	if err := fileStorage.Create(ctx, secCtx, draftPlaneDocEntry(id2, "Draft cache couple", "draft")); err != nil {
		t.Fatalf("Create draft for cache couple: %v", err)
	}
	if cachedID != id2 {
		t.Fatalf("draft create did not couple object-id-cache id=%q", cachedID)
	}
	wantDraft := ObjectDraftPlanePath(tmpDir, objects.KindDocEntry, id2)
	if cachedPath != wantDraft {
		t.Fatalf("cache path=%s want draft plane %s", cachedPath, wantDraft)
	}
	if cas2, casErr2 := fileStorage.getContentAddressableStorage(objects.KindDocEntry); casErr2 == nil && cas2 != nil {
		if _, hashErr := cas2.GetHashForID(id2); hashErr == nil {
			t.Fatalf("draft create should not register CAS id→hash for %s", id2)
		}
	}

	got, err := fileStorage.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("Read after draft create: %v", err)
	}
	if got[objects.FieldKeyTitle] != "Draft plane smoke" {
		t.Fatalf("title=%v", got[objects.FieldKeyTitle])
	}
	exists, err := fileStorage.Exists(ctx, secCtx, id)
	if err != nil || !exists {
		t.Fatalf("Exists after draft create: exists=%v err=%v", exists, err)
	}

	if err := fileStorage.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyStatus: objectStatusReview}); err != nil {
		t.Fatalf("Update to review (materialize): %v", err)
	}
	if _, err := fileutil.Stat(draftPath); !fileutil.IsNotExist(err) {
		t.Fatalf("draft-plane file should be gone after promote, stat=%v", err)
	}
	casPath, pathErr := fileStorage.getObjectFilePath(id, objects.KindDocEntry)
	if pathErr != nil {
		t.Fatalf("CAS path after promote: %v", pathErr)
	}
	if IsObjectDraftPlanePath(tmpDir, casPath) {
		t.Fatalf("after promote path still draft plane: %s", casPath)
	}
	got2, err := fileStorage.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("Read after promote: %v", err)
	}
	if got2[objects.FieldKeyStatus] != "review" {
		t.Fatalf("status after promote=%v", got2[objects.FieldKeyStatus])
	}
}

func TestObjectDraftPlane_GetByID_ListOmitsDrafts(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()
	storageCtx := pkgctx.NewStorageContext()

	draftID := "DOC-1777000000000000000-draftlist01"
	casID := "DOC-1777000000000000000-draftlist02"

	if err := fileStorage.Create(ctx, secCtx, draftPlaneDocEntry(draftID, "Still draft", "draft")); err != nil {
		t.Fatalf("Create draft: %v", err)
	}
	// Draft-first membrane coerces create status to origin; promote via Update for CAS-visible peer.
	// TRACK: BLI-1785443942668406000-1ec5c811 — draft-plane create / promote membrane.
	CreateCASVisible(t, fileStorage, ctx, secCtx, draftPlaneDocEntry(casID, "Already review", "review"), "review")

	// Dual-read: draft is gettable by id even though it is not in CAS.
	got, err := fileStorage.Read(ctx, secCtx, draftID)
	if err != nil {
		t.Fatalf("Read draft by id: %v", err)
	}
	if got[objects.FieldKeyTitle] != "Still draft" {
		t.Fatalf("draft title=%v", got[objects.FieldKeyTitle])
	}
	if !fileStorage.objectDraftPlaneExists(objects.KindDocEntry, draftID) {
		t.Fatal("expected draft on draft plane")
	}
	if cas, e := fileStorage.getContentAddressableStorage(objects.KindDocEntry); e == nil && cas != nil {
		if _, hashErr := cas.GetHashForID(draftID); hashErr == nil {
			t.Fatal("draft id should not be in CAS index")
		}
		if _, hashErr := cas.GetHashForID(casID); hashErr != nil {
			t.Fatalf("review id should be in CAS index: %v", hashErr)
		}
	}

	result, err := fileStorage.List(ctx, secCtx, storageCtx, ListFilter{Kind: objects.KindDocEntry})
	if err != nil {
		t.Fatalf("List doc_entry: %v", err)
	}
	if listContainsID(result, draftID) {
		t.Fatalf("normal List must omit draft-plane id %s", draftID)
	}
	if !listContainsID(result, casID) {
		t.Fatalf("normal List must include post-membrane id %s", casID)
	}

	n, err := fileStorage.Count(ctx, secCtx, ListFilter{Kind: objects.KindDocEntry})
	if err != nil {
		t.Fatalf("Count doc_entry: %v", err)
	}
	if n != len(result.Objects) {
		t.Fatalf("Count=%d List=%d (draft plane must not inflate Count)", n, len(result.Objects))
	}
	if n < 1 {
		t.Fatalf("Count expected >=1 (CAS review), got %d", n)
	}

	// After promote, draft appears in List and leaves draft plane.
	if err := fileStorage.Update(ctx, secCtx, draftID, map[string]any{objects.FieldKeyStatus: objectStatusReview}); err != nil {
		t.Fatalf("promote draft: %v", err)
	}
	if fileStorage.objectDraftPlaneExists(objects.KindDocEntry, draftID) {
		t.Fatal("draft plane file should be gone after promote")
	}
	result2, err := fileStorage.List(ctx, secCtx, storageCtx, ListFilter{Kind: objects.KindDocEntry})
	if err != nil {
		t.Fatalf("List after promote: %v", err)
	}
	if !listContainsID(result2, draftID) {
		t.Fatalf("after promote, List should include %s", draftID)
	}
	_ = tmpDir
}

func TestObjectDraftPlane_ListOmitsConceptualEvenWithStatusDraftFilter(t *testing.T) {
	// `zqk object list --filter status=draft` (and status=conceptual) must never
	// return conceptual objects that exist only on the draft plane.
	// TRACK: BLI-1786689721908382000-6402a858
	_, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := WithCLIOperation(pkgctx.NewSystemContext())
	storageCtx := pkgctx.NewStorageContext()

	id := "BLI-1777000000000000000-conceptual01"
	if err := fileStorage.Create(ctx, secCtx, draftPlaneBacklogItem(id, "Conceptual only", objects.ObjectStatusConceptual)); err != nil {
		t.Fatalf("Create conceptual BLI: %v", err)
	}
	if !fileStorage.objectDraftPlaneExists(objects.KindBacklogItem, id) {
		t.Fatal("expected conceptual BLI on draft plane")
	}
	got, err := fileStorage.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("Get must follow draft plane: %v", err)
	}
	if st := objects.GetString(got, objects.FieldKeyStatus); st != objects.ObjectStatusConceptual && st != objects.ObjectStatusDraft {
		t.Fatalf("status after create=%v want conceptual or draft", st)
	}

	leaked := &QueryResult{Objects: []map[string]any{got}, Meta: map[string]any{"total_count": 1}}
	fileStorage.omitDraftPlaneOnlyFromList(leaked)
	if listContainsID(leaked, id) {
		t.Fatal("omitDraftPlaneOnlyFromList must drop conceptual draft-plane-only payload")
	}

	for _, statusFilter := range []any{nil, objects.ObjectStatusDraft, objects.ObjectStatusConceptual, got[objects.FieldKeyStatus]} {
		var filters map[string]any
		if statusFilter != nil {
			filters = map[string]any{objects.FieldKeyStatus: statusFilter}
		}
		result, listErr := fileStorage.List(ctx, secCtx, storageCtx, ListFilter{Kind: objects.KindBacklogItem, Filters: filters})
		if listErr != nil {
			t.Fatalf("List filters=%v: %v", filters, listErr)
		}
		if listContainsID(result, id) {
			t.Fatalf("List filters=%v must omit conceptual draft-plane-only id %s", filters, id)
		}
	}
}

func TestObjectDraftPlane_UpdateStayDraft_ThenDelete(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	id := "DOC-1777000000000000000-draftupd01"
	if err := fileStorage.Create(ctx, secCtx, draftPlaneDocEntry(id, "Draft title v1", "draft")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := fileStorage.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyTitle: "Draft title v2"}); err != nil {
		t.Fatalf("Update stay draft: %v", err)
	}
	got, err := fileStorage.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got[objects.FieldKeyTitle] != "Draft title v2" {
		t.Fatalf("title=%v want Draft title v2", got[objects.FieldKeyTitle])
	}
	if !fileStorage.objectDraftPlaneExists(objects.KindDocEntry, id) {
		t.Fatal("should remain on draft plane after title update")
	}
	if cas, e := fileStorage.getContentAddressableStorage(objects.KindDocEntry); e == nil && cas != nil {
		if _, hashErr := cas.GetHashForID(id); hashErr == nil {
			t.Fatal("stay-draft update must not materialize CAS")
		}
	}

	cliCtx := WithTestHardDelete(ctx)
	if err := fileStorage.Delete(cliCtx, secCtx, id, false); err != nil {
		t.Fatalf("Delete draft: %v", err)
	}
	if fileStorage.objectDraftPlaneExists(objects.KindDocEntry, id) {
		t.Fatal("draft file should be removed")
	}
	if _, err := fileStorage.Read(ctx, secCtx, id); err == nil {
		t.Fatal("Read after delete should fail")
	}
	_ = tmpDir
}

func TestInventoryObjectDraftPlane(t *testing.T) {
	root := t.TempDir()
	inv := InventoryObjectDraftPlane(root)
	if inv.Total != 0 {
		t.Fatalf("empty root total=%d", inv.Total)
	}
	id := "DOC-inv-0001"
	path := ObjectDraftPlanePath(root, "doc_entry", id)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(path, []byte("id: "+id+"\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	inv = InventoryObjectDraftPlane(root)
	if inv.Total != 1 || inv.ByKind["doc_entry"] != 1 {
		t.Fatalf("inv=%+v", inv)
	}
	if len(inv.SampleIDs) != 1 || inv.SampleIDs[0] != id {
		t.Fatalf("samples=%v", inv.SampleIDs)
	}
	if got := inv.SampleIDsByKind["doc_entry"]; len(got) != 1 || got[0] != id {
		t.Fatalf("SampleIDsByKind=%v", inv.SampleIDsByKind)
	}
	if len(inv.DualPlaneIDs) != 0 {
		t.Fatalf("draft-only id should not be dual-plane, got %v", inv.DualPlaneIDs)
	}
}

func TestObjectDraftPlane_PathHelpers(t *testing.T) {
	root := "/tmp/zqk-draft-plane-test"
	p := ObjectDraftPlanePath(root, "doc_entry", "DOC-abc")
	if !IsObjectDraftPlanePath(root, p) {
		t.Fatalf("IsObjectDraftPlanePath false for %s", p)
	}
	if IsObjectDraftPlanePath(root, filepath.Join(root, paths.ProcessDir, "doc_entries/x.yaml")) {
		t.Fatal("CAS path should not be draft plane")
	}
	if objectDraftShard("a") == "" || len(objectDraftShard("a")) != objectDraftShardHexLen {
		t.Fatalf("bad shard %q", objectDraftShard("a"))
	}
}

func TestShouldUseObjectDraftPlane_glossaryActiveUsesCAS(t *testing.T) {
	_, _, _ = SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	if shouldUseObjectDraftPlane("glossary_term", "active") {
		t.Fatal("glossary_term status=active (lifecycle origin) must use CAS, not the draft plane")
	}
	if !shouldUseObjectDraftPlane(objects.KindDocEntry, "draft") {
		t.Fatal("doc_entry draft should use draft plane")
	}
}

func TestCreate_policyActiveCoercedToDraftPlane(t *testing.T) {
	// TRACK: REQ-1785895564241296000-bf266adb
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := WithCLIOperation(pkgctx.NewSystemContext())

	id := "POL-CODE-997"
	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindPolicy,
		objects.FieldKeyTitle:         "Draft-first membrane smoke",
		objects.FieldKeyStatus:        objects.ObjectStatusActive, // must not permeate CAS on create
		objects.FieldKeyCategory:      "workflow",
		objects.FieldKeyPolicyType:    "standard",
		objects.FieldKeyBody:          "body",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
	}
	if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("Create policy: %v", err)
	}
	draftPath := ObjectDraftPlanePath(tmpDir, objects.KindPolicy, id)
	if _, err := fileutil.Stat(draftPath); err != nil {
		t.Fatalf("expected draft-plane file at %s: %v", draftPath, err)
	}
	got, err := fileStorage.Read(ctx, secCtx, id)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if st := objects.GetString(got, objects.FieldKeyStatus); st != objects.ObjectStatusConceptual && st != objects.ObjectStatusDraft {
		t.Fatalf("status after create=%v want conceptual or draft", got[objects.FieldKeyStatus])
	}
}

func TestDraftPlaneHasTitle(t *testing.T) {
	root := t.TempDir()
	id := "DOC-title-0001"
	path := ObjectDraftPlanePath(root, "doc_entry", id)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	body := "id: " + id + "\nkind: doc_entry\ntitle: Shared Kernel Constraints\n"
	if err := fileutil.WriteFile(path, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if !DraftPlaneHasTitle(root, "doc_entry", "shared kernel constraints") {
		t.Fatal("expected title match")
	}
	if DraftPlaneHasTitle(root, "doc_entry", "other title") {
		t.Fatal("unexpected title match")
	}
}

func TestErrIfDraftCreateWouldDualPlane(t *testing.T) {
	_, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.WithPromoteOnCreate(pkgctx.NewSystemContext())
	id := "POL-CODE-dualplane-001"
	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindPolicy,
		objects.FieldKeyTitle:         "Dual-plane refuse smoke",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeyCategory:      "workflow",
		objects.FieldKeyPolicyType:    "standard",
		objects.FieldKeyBody:          "body",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyCreatedAt:     zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:     "ACC-1785920548450214012-68b850c0",
	}
	if err := fileStorage.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("promote Create: %v", err)
	}
	if fileStorage.objectDraftPlaneExists(objects.KindPolicy, id) {
		t.Fatal("promote-on-create must land CAS, not draft")
	}
	err := fileStorage.errIfDraftCreateWouldDualPlane(id, objects.KindPolicy)
	if err == nil || !strings.Contains(err.Error(), "dual-plane") {
		t.Fatalf("want dual-plane refuse, got %v", err)
	}
}

// Bulk delete routes through the batched CAS commit path, which only knows the
// post-membrane plane. Before the fix it reported SuccessCount for draft-plane
// objects while the YAML stayed on disk and readable — a silent "deleted" lie.
// TRACK: BLI-DRAFT-PLANE-ORCHESTRATE-GHOST-001
func TestObjectDraftPlane_BulkDeleteRemovesDraftFiles(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	// doc_entry is a core kind: BulkDelete requires declared hard-delete intent.
	ctx := WithTestHardDelete(pkgctx.NewSystemContext())

	ids := []string{
		"DOC-1777000000000000000-bulkdel001",
		"DOC-1777000000000000000-bulkdel002",
	}
	for _, id := range ids {
		if err := fileStorage.Create(ctx, secCtx, draftPlaneDocEntry(id, "Bulk delete draft", "draft")); err != nil {
			t.Fatalf("Create draft %s: %v", id, err)
		}
		if _, err := fileutil.Stat(ObjectDraftPlanePath(tmpDir, objects.KindDocEntry, id)); err != nil {
			t.Fatalf("expected draft-plane file for %s: %v", id, err)
		}
	}

	res, err := fileStorage.BulkDelete(ctx, secCtx, ids, false)
	if err != nil {
		t.Fatalf("BulkDelete: %v", err)
	}
	if res.SuccessCount != len(ids) {
		t.Fatalf("SuccessCount=%d want %d (errors: %v)", res.SuccessCount, len(ids), res.Errors)
	}

	for _, id := range ids {
		if _, err := fileutil.Stat(ObjectDraftPlanePath(tmpDir, objects.KindDocEntry, id)); !fileutil.IsNotExist(err) {
			t.Fatalf("draft-plane file for %s still present after bulk delete: stat=%v", id, err)
		}
		if _, err := fileStorage.Read(ctx, secCtx, id); err == nil {
			t.Fatalf("Read(%s) succeeded after bulk delete; object still resolvable", id)
		}
	}
}

func TestObjectDraftPlane_CreateFailsWhenCacheHandlerNil(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()
	id := "DOC-1777000000000000000-draftcachenil"

	prevHandler := GetCacheOperationHandler()
	t.Cleanup(func() { SetCacheOperationHandler(prevHandler) })
	SetCacheOperationHandler(nil)

	err := fileStorage.Create(ctx, secCtx, draftPlaneDocEntry(id, "Cache nil", "draft"))
	if err == nil {
		t.Fatal("Create should fail when object-id-cache handler is nil")
	}
	if _, statErr := fileutil.Stat(ObjectDraftPlanePath(tmpDir, objects.KindDocEntry, id)); !fileutil.IsNotExist(statErr) {
		t.Fatalf("abort should remove draft YAML after nil cache handler, stat=%v", statErr)
	}
}

func TestObjectDraftPlane_CreateFailsWhenCacheHandlerErrors(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()
	id := "DOC-1777000000000000000-draftcachefail"

	prevHandler := GetCacheOperationHandler()
	t.Cleanup(func() { SetCacheOperationHandler(prevHandler) })
	SetCacheOperationHandler(func(*pkgctx.CacheContext) error {
		return errfmt.Errorf("injected cache couple failure")
	})

	err := fileStorage.Create(ctx, secCtx, draftPlaneDocEntry(id, "Cache fail", "draft"))
	if err == nil {
		t.Fatal("Create should fail when object-id-cache couple fails")
	}
	if _, statErr := fileutil.Stat(ObjectDraftPlanePath(tmpDir, objects.KindDocEntry, id)); !fileutil.IsNotExist(statErr) {
		t.Fatalf("abort should remove draft YAML after cache couple failure, stat=%v", statErr)
	}
}

func TestObjectDraftPlane_IsDraftPlaneOnly(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := pkgctx.NewSystemContext()

	draftID := "DOC-1777000000000000000-isdraftonly"
	casID := "DOC-1777000000000000000-casbacked"

	// 1. Create a draft-plane object
	err := fileStorage.Create(ctx, secCtx, draftPlaneDocEntry(draftID, "Draft Only Doc", "draft"))
	if err != nil {
		t.Fatalf("Create draft doc failed: %v", err)
	}

	// Verify IsDraftPlaneOnly
	if !IsDraftPlaneOnly(tmpDir, objects.KindDocEntry, draftID) {
		t.Errorf("expected IsDraftPlaneOnly to return true for %s", draftID)
	}
	if !IsDraftPlaneOnly(tmpDir, "", draftID) {
		t.Errorf("expected IsDraftPlaneOnly with empty kind to return true for %s", draftID)
	}
	if !fileStorage.IsDraftPlaneOnly(draftID) {
		t.Errorf("expected fileStorage.IsDraftPlaneOnly to return true for %s", draftID)
	}

	// Non-existent ID should return false
	if IsDraftPlaneOnly(tmpDir, objects.KindDocEntry, "DOC-non-existent") {
		t.Errorf("expected IsDraftPlaneOnly to return false for non-existent ID")
	}

	// 2. Create a CAS-backed object
	activeDoc := draftPlaneDocEntry(casID, "CAS Backed Doc", "active")
	activeDoc[objects.FieldKeyDescription] = "Substantive description for active CAS doc entry"
	err = fileStorage.Create(ctx, secCtx, activeDoc)
	if err != nil {
		t.Fatalf("Create active CAS doc failed: %v", err)
	}

	if IsDraftPlaneOnly(tmpDir, objects.KindDocEntry, casID) {
		t.Errorf("expected IsDraftPlaneOnly to return false for active CAS doc %s", casID)
	}

	// 3. CAS object referencing draft-plane-only object must fail validateReferences
	invalidCASObj := draftPlaneDocEntry("DOC-1777000000000000000-invalidcas", "Invalid Cross Plane CAS Doc", "active")
	invalidCASObj[objects.FieldKeyDescription] = "Substantive description for invalid cross-plane CAS doc entry"
	invalidCASObj[objects.FieldKeyRequirementRefs] = []string{draftID}

	err = fileStorage.Create(ctx, secCtx, invalidCASObj)
	if err == nil {
		t.Fatalf("expected Create of CAS object referencing draft object to FAIL with cross-plane error, but it succeeded")
	}
	if !strings.Contains(err.Error(), "cross-plane reference prohibited") && !strings.Contains(err.Error(), "no_draft_plane_refs_from_cas") {
		t.Fatalf("expected cross-plane reference error, got: %v", err)
	}
}

func TestEnsureCASIndexFromPaths_ExcludesDraftPlane(t *testing.T) {
	tmpDir, fileStorage, _ := SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := WithCLIOperation(pkgctx.NewSystemContext())

	draftID := "DOC-draft-excl-001"
	draftDoc := draftPlaneDocEntry(draftID, "Draft Doc", "draft")
	if err := fileStorage.Create(ctx, secCtx, draftDoc); err != nil {
		t.Fatalf("Create draft doc: %v", err)
	}

	draftPath := ObjectDraftPlanePath(tmpDir, objects.KindDocEntry, draftID)
	// Try to ensure CAS index with this draft path
	err := fileStorage.EnsureCASIndexFromPaths(objects.KindDocEntry, map[string]string{
		draftID: draftPath,
	})
	if err != nil {
		t.Fatalf("EnsureCASIndexFromPaths returned error: %v", err)
	}

	// Verify that the CAS index does NOT contain this draft ID
	cas, err := fileStorage.getContentAddressableStorage(objects.KindDocEntry)
	if err != nil {
		t.Fatalf("getContentAddressableStorage: %v", err)
	}
	if hash, err := cas.GetIndex().GetHash(draftID); err == nil && hash != "" {
		t.Fatalf("CAS index should NOT map draft-plane ID %s, but got %s", draftID, hash)
	}
}
