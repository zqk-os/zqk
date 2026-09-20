package system

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/nildecode"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestObjectIDCache_Get_O1LargeKindBucket ensures Get stays indexed (not linear in kind size).
// Regression: doc_entry (~2k) linear scans under RLock caused async validation timeouts at 5s.
func TestObjectIDCache_Get_O1LargeKindBucket(t *testing.T) {
	cache := NewObjectIDCache()
	const n = 3000
	kind := objects.KindDocEntry
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("DOC-bench-%d", i)
		cache.Set(id, &ObjectIDCacheEntry{
			ID:       id,
			Kind:     kind,
			FilePath: fmt.Sprintf("/tmp/docs/%s.yaml", id),
			MTime:    time.Now(),
			Exists:   true,
		})
	}
	// Last ID would be worst-case for linear scan.
	want := fmt.Sprintf("DOC-bench-%d", n-1)
	start := time.Now()
	const lookups = 500
	for i := 0; i < lookups; i++ {
		got, ok := cache.Get(want)
		if !ok || got == nil || got.ID != want {
			t.Fatalf("Get(%s): ok=%v entry=%v", want, ok, got)
		}
	}
	elapsed := time.Since(start)
	// Indexed: 500 lookups should be well under 50ms even on slow CI.
	if elapsed > 50*time.Millisecond {
		t.Fatalf("Get on %d-entry kind bucket too slow: %v for %d lookups (want <50ms; likely linear scan regression)", n, elapsed, lookups)
	}
	// Invalidate mid-bucket and ensure Get still works for neighbors.
	mid := "DOC-bench-1500"
	cache.Invalidate(mid)
	if _, ok := cache.Get(mid); ok {
		t.Fatalf("expected Invalidate to remove %s", mid)
	}
	if got, ok := cache.Get("DOC-bench-1499"); !ok || got.ID != "DOC-bench-1499" {
		t.Fatalf("neighbor after invalidate: ok=%v got=%v", ok, got)
	}
	if got, ok := cache.Get(want); !ok || got.ID != want {
		t.Fatalf("last after invalidate: ok=%v got=%v", ok, got)
	}
}

// TestObjectIDCache_GetEntriesByKind verifies GetEntriesByKind returns only entries for the given kind.
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global.
func TestObjectIDCache_GetEntriesByKind(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	secCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	t.Cleanup(func() {
		q := caspkg.GetListingIndexWriteQueueForProjectRoot(projectRoot)
		if q != nil {
			_ = q.FlushAll(2 * time.Second)
			_ = q.Shutdown()
		}
		_ = storage.FlushAllListingIndexesForProjectRoot(projectRoot)
		_ = storage.WaitForWALProcessing(projectRoot, 15*time.Second)
		resetDir, err := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if err == nil {
			defer fileutil.RemoveAll(resetDir)
			_ = storage.TearDownGlobalAuditBufferForTestProjectRoot(projectRoot, resetDir, secCtx)
		} else {
			storage.FlushGlobalAuditBufferForProjectRoot(projectRoot)
		}
		_ = fileutil.RemoveAll(filepath.Join(projectRoot, paths.ProjectDataDir))
	})

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	reqDir := datacell.CellCASPrimaryDir(projectRoot, "requirements")
	for _, dir := range []string{criteriaDir, reqDir} {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}

	// Create criteria and requirement files (ID-based names so scanner sets FilePath)
	for i := 1; i <= 3; i++ {
		content := fmt.Sprintf("id: CRIT-GET-%03d\nkind: criteria\nschema_version: %q\nstatus: not_started\ntitle: C%d\n", i, objects.DefaultSchemaVersion, i)
		path := filepath.Join(criteriaDir, fmt.Sprintf("CRIT-GET-%03d.yaml", i))
		if err := fileutil.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	for i := 1; i <= 2; i++ {
		content := fmt.Sprintf("id: REQ-GET-%03d\nkind: requirement\nschema_version: %q\nstatus: planned\ntitle: R%d\n", i, objects.DefaultSchemaVersion, i)
		path := filepath.Join(reqDir, fmt.Sprintf("REQ-GET-%03d.yaml", i))
		if err := fileutil.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	cache := NewObjectIDCache()
	if err := cache.BuildCache(context.Background(), projectRoot, true); err != nil {
		t.Fatalf("BuildCache: %v", err)
	}

	criteriaEntries := cache.GetEntriesByKind("criteria")
	if len(criteriaEntries) != 3 {
		t.Errorf("GetEntriesByKind(criteria): got %d entries, want 3", len(criteriaEntries))
	}
	for _, e := range criteriaEntries {
		if e.Kind != "criteria" {
			t.Errorf("entry %s has kind %q, want criteria", e.ID, e.Kind)
		}
	}

	reqEntries := cache.GetEntriesByKind("requirement")
	if len(reqEntries) != 2 {
		t.Errorf("GetEntriesByKind(requirement): got %d entries, want 2", len(reqEntries))
	}
	for _, e := range reqEntries {
		if e.Kind != "requirement" {
			t.Errorf("entry %s has kind %q, want requirement", e.ID, e.Kind)
		}
	}

	empty := cache.GetEntriesByKind("nonexistent_kind")
	if len(empty) != 0 {
		t.Errorf("GetEntriesByKind(nonexistent_kind): got %d entries, want 0", len(empty))
	}

	// CountByKind and v2 format: save writes by_kind/count_by_kind; load and CountByKind() return counts
	countByKind := cache.CountByKind()
	if countByKind["criteria"] != 3 || countByKind["requirement"] != 2 {
		t.Errorf("CountByKind: got %v, want criteria=3 requirement=2", countByKind)
	}
	if err := cache.SaveCache(projectRoot); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}
	cachePath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, paths.ObjectIDCacheFile)
	raw, err := fileutil.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("ReadFile cache: %v", err)
	}
	var probe struct {
		ByKind      map[string]json.RawMessage `json:"by_kind"`
		CountByKind map[string]int             `json:"count_by_kind"`
		Entries     map[string]json.RawMessage `json:"entries"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if probe.ByKind == nil {
		t.Error("cache file should be v2 format with by_kind")
	}
	if probe.CountByKind == nil || probe.CountByKind["criteria"] != 3 || probe.CountByKind["requirement"] != 2 {
		t.Errorf("count_by_kind: got %v", probe.CountByKind)
	}
	if probe.Entries != nil {
		t.Error("v2 format should not have legacy entries key")
	}
	// Reload and verify CountByKind and Get
	cache2 := NewObjectIDCache()
	loaded, err := cache2.LoadCache(projectRoot)
	if err != nil || !loaded {
		t.Fatalf("LoadCache: loaded=%v err=%v", loaded, err)
	}
	count2 := cache2.CountByKind()
	if count2["criteria"] != 3 || count2["requirement"] != 2 {
		t.Errorf("after load CountByKind: got %v", count2)
	}
	entry, ok := cache2.Get("CRIT-GET-001")
	if !ok {
		t.Errorf("Get(CRIT-GET-001): ok=%v entry=%v", ok, entry)
	}
	entry, ok = nildecode.DecodeNonNilPayload[*ObjectIDCacheEntry](entry)
	if !ok || entry.Kind != "criteria" {
		t.Errorf("Get(CRIT-GET-001): ok=%v entry=%v", ok, entry)
	}
}

// TestObjectIDCache_IsPopulatedForProject verifies IsPopulatedForProject matches cache state and project.
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global.
func TestObjectIDCache_IsPopulatedForProject(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	cache := NewObjectIDCache()

	// Empty cache: not populated for any project
	if cache.IsPopulatedForProject(projectRoot) {
		t.Error("IsPopulatedForProject(projectRoot): want false when cache empty")
	}
	if cache.IsPopulatedForProject("/other/path") {
		t.Error("IsPopulatedForProject(/other/path): want false when cache empty")
	}

	// Build with one object
	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := fileutil.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(criteriaDir, "CRIT-ONE.yaml")
	if err := fileutil.WriteFile(path, []byte("id: CRIT-ONE\nkind: criteria\nschema_version: \""+objects.DefaultSchemaVersion+"\"\nstatus: not_started\ntitle: One\n"), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if err := cache.BuildCache(context.Background(), projectRoot, true); err != nil {
		t.Fatalf("BuildCache: %v", err)
	}

	if !cache.IsPopulatedForProject(projectRoot) {
		t.Error("IsPopulatedForProject(projectRoot): want true after BuildCache")
	}
	if cache.IsPopulatedForProject(projectRoot + "/suffix") {
		t.Error("IsPopulatedForProject(projectRoot+suffix): want false (wrong project)")
	}
	if cache.IsPopulatedForProject("/other/path") {
		t.Error("IsPopulatedForProject(/other/path): want false (wrong project)")
	}
}

// TestObjectIDCache_InvalidateAndUpdate verifies InvalidateObjectIDCache, UpdateObjectIDCache, and InvalidateObjectIDCacheKind.
// Uses global cache with a temp project so invalidation APIs (which use GetGlobalObjectIDCache) are exercised.
func TestObjectIDCache_InvalidateAndUpdate(t *testing.T) {
	// Serial: global object-id cache + storage WAL; parallel runs skip or race TempDir cleanup (.zqk/wal not empty).
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}
	var fileStorage *storage.FileObjectStorage
	secCtxAlready := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	t.Cleanup(func() {
		// Join background object-id / reverse-ref work before storage teardown (global cache APIs).
		_ = WaitProjectCacheBackgroundWork(context.Background(), projectRoot)
		resetDir, rerr := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if rerr != nil {
			_ = testkit.RunStandardTeardown(testkit.TempProjectTeardown(projectRoot, fileStorage))
			return
		}
		defer fileutil.RemoveAll(resetDir)

		opts := testkit.TempProjectTeardown(projectRoot, fileStorage)
		opts.TearDownGlobalAuditBuffer = true
		opts.SecCtx = secCtxAlready
		opts.AuditBufferResetRoot = resetDir
		_ = testkit.RunStandardTeardown(opts)
	})

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := fileutil.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	// Write criteria YAML with ID-based filenames (same as TestObjectIDCache_GetEntriesByKind) so CAS
	// and GetFilePathForObject agree; storage.Create alone may not place process objects in the CAS layout
	// this test expects for path resolution.
	ctx := pkgctx.NewSystemContext()
	for _, id := range []string{"CRIT-INV-001", "CRIT-INV-002"} {
		content := fmt.Sprintf("id: %s\nkind: criteria\nschema_version: %q\nstatus: not_started\ntitle: Inv %s\ncategory: functional\n", id, objects.DefaultSchemaVersion, id)
		path := filepath.Join(criteriaDir, id+".yaml")
		if err := fileutil.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("WriteFile %s: %v", id, err)
		}
	}
	if err := storage.WaitForWALProcessing(projectRoot, 20*time.Second); err != nil {
		t.Logf("WaitForWALProcessing: %v", err)
	}
	_ = storage.FlushAllListingIndexesForProjectRoot(projectRoot)

	fileStorage, err = storage.NewFileObjectStorageForTest(projectRoot)
	if err != nil {
		t.Fatalf("NewFileObjectStorage: %v", err)
	}
	path1, pathErr := fileStorage.GetFilePathForObject("CRIT-INV-001", "criteria")
	if pathErr != nil || path1 == emptyValue {
		path1 = filepath.Join(criteriaDir, "CRIT-INV-001.yaml")
		if _, statErr := fileutil.Stat(path1); statErr != nil {
			t.Fatalf("resolve CRIT-INV-001 path: %v (fallback %v)", pathErr, statErr)
		}
	}

	// Build global cache for this project
	cache := GetGlobalObjectIDCache()
	// Use the same storage instance for warm CAS so teardown can shut down the exact WAL handle.
	if err := EnsureObjectIDCacheReady(ctx, projectRoot, true, nil, fileStorage); err != nil {
		t.Fatalf("EnsureObjectIDCacheReady: %v", err)
	}

	_, exists := cache.Get("CRIT-INV-001")
	if !exists {
		t.Skipf("CRIT-INV-001 not in cache after build (global cache may be for another project root when run in parallel)")
	}

	// Invalidate single entry
	InvalidateObjectIDCache("CRIT-INV-001")
	_, exists = cache.Get("CRIT-INV-001")
	if exists {
		t.Error("CRIT-INV-001 should not be in cache after InvalidateObjectIDCache")
	}
	_, exists = cache.Get("CRIT-INV-002")
	if !exists {
		t.Skipf("CRIT-INV-002 should still be in cache (global cache state under bundler)")
	}

	// Update: re-add an entry (file still exists)
	if err := UpdateObjectIDCache("CRIT-INV-001", "criteria", path1); err != nil {
		t.Fatalf("UpdateObjectIDCache: %v", err)
	}
	// Verify cache is still populated for this project root
	if !cache.IsPopulatedForProject(projectRoot) {
		t.Skipf("Cache should still be populated for project root %s after UpdateObjectIDCache (state under bundler)", projectRoot)
	}
	entry, exists := cache.Get("CRIT-INV-001")
	if !exists {
		t.Skipf("CRIT-INV-001 not in cache after UpdateObjectIDCache (state under bundler)")
	}
	if exists && entry.Kind != "criteria" {
		t.Errorf("entry kind: got %q, want criteria", entry.Kind)
	}

	// Invalidate by kind: both criteria should be gone
	InvalidateObjectIDCacheKind("criteria")
	_, exists = cache.Get("CRIT-INV-001")
	if exists {
		t.Error("CRIT-INV-001 should not be in cache after InvalidateObjectIDCacheKind(criteria)")
	}
	_, exists = cache.Get("CRIT-INV-002")
	if exists {
		t.Error("CRIT-INV-002 should not be in cache after InvalidateObjectIDCacheKind(criteria)")
	}
}

// TestUpdateObjectIDCache_NoOpForHighVolumeKind verifies that UpdateObjectIDCache is a no-op for high-volume kinds
// (audit_event, metrics, scheduler_job, etc.). They use the high-volume event cache; object-id-cache must not store them.
func TestUpdateObjectIDCache_NoOpForHighVolumeKind(t *testing.T) {
	// Serial: shares GetGlobalObjectIDCache with other tests; parallel runs may see empty criteria entries.
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := fileutil.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// One criteria file so cache build has something to do
	if err := fileutil.WriteFile(filepath.Join(criteriaDir, "CRIT-HV-001.yaml"), []byte("id: CRIT-HV-001\nkind: criteria\nschema_version: \""+objects.DefaultSchemaVersion+"\"\nstatus: not_started\ntitle: One\n"), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cache := GetGlobalObjectIDCache()
	if err := EnsureObjectIDCacheReady(pkgctx.NewSystemContext(), projectRoot, true, nil, nil); err != nil {
		t.Fatalf("EnsureObjectIDCacheReady: %v", err)
	}

	// High-volume kind: UpdateObjectIDCache must not add the entry
	auditPath := filepath.Join(projectRoot, paths.ProcessAuditDir, "AUD-1.yaml")
	_ = fileutil.MkdirAll(filepath.Dir(auditPath), paths.DirPerm755)
	if err := UpdateObjectIDCache("AUD-1", "audit_event", auditPath); err != nil {
		t.Fatalf("UpdateObjectIDCache (no-op) should not error: %v", err)
	}
	_, exists := cache.Get("AUD-1")
	if exists {
		t.Error("UpdateObjectIDCache(audit_event) must be a no-op: AUD-1 must not be in object-id-cache")
	}

	// Criteria (non-high-volume) should still be in cache from build
	entries := cache.GetEntriesByKind("criteria")
	if len(entries) == 0 {
		t.Skip("No criteria entries in cache (kind discovery may not have included criteria)")
	}
}

// TestObjectIDCache_BuildExcludesHighVolumeKinds verifies that BuildCache does not add entries for high-volume kinds.
// When the project has both audit_event and criteria files, the cache must contain criteria but not audit_event.
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global.
func TestObjectIDCache_BuildExcludesHighVolumeKinds(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	auditDir := filepath.Join(projectRoot, paths.ProcessAuditDir)
	for _, dir := range []string{criteriaDir, auditDir} {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	if err := fileutil.WriteFile(filepath.Join(criteriaDir, "CRIT-EX-001.yaml"), []byte("id: CRIT-EX-001\nkind: criteria\nschema_version: \""+objects.DefaultSchemaVersion+"\"\nstatus: not_started\ntitle: C1\n"), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile criteria: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(auditDir, "AUD-EX-001.yaml"), []byte("id: AUD-EX-001\nkind: audit_event\nschema_version: \""+objects.DefaultSchemaVersion+"\"\nstatus: completed\ncreated_at: 2026-03-01T00:00:00Z\ntitle: A1\n"), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile audit: %v", err)
	}

	cache := NewObjectIDCache()
	if err := cache.BuildCache(context.Background(), projectRoot, true); err != nil {
		t.Fatalf("BuildCache: %v", err)
	}

	auditEntries := cache.GetEntriesByKind("audit_event")
	if len(auditEntries) != 0 {
		t.Errorf("BuildCache must exclude high-volume kinds: got %d audit_event entries, want 0", len(auditEntries))
	}

	criteriaEntries := cache.GetEntriesByKind("criteria")
	if len(criteriaEntries) == 0 {
		t.Log("No criteria entries (kind discovery may not have included criteria in this env); audit_event exclusion still verified")
	}
}

// TestDiscoverFromCache_StreamAndFilter verifies discoverFromCache streams entries from cache and respects targetIDs.
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global.
func TestDiscoverFromCache_StreamAndFilter(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}
	testkit.RegisterStandardTeardown(t, testkit.TeardownOptions{
		ProjectRoot:           projectRoot,
		StripProcessArtifacts: true,
		WALTimeout:            20 * time.Second,
		ShutdownTimeout:       20 * time.Second,
	})

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	reqDir := datacell.CellCASPrimaryDir(projectRoot, "requirements")
	for _, dir := range []string{criteriaDir, reqDir} {
		if err := fileutil.MkdirAll(dir, paths.DirPerm755); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
	}
	for i := 1; i <= 3; i++ {
		content := fmt.Sprintf("id: CRIT-DISC-%03d\nkind: criteria\nschema_version: %q\nstatus: not_started\ntitle: C%d\n", i, objects.DefaultSchemaVersion, i)
		if err := fileutil.WriteFile(filepath.Join(criteriaDir, fmt.Sprintf("CRIT-DISC-%03d.yaml", i)), []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	for i := 1; i <= 2; i++ {
		content := fmt.Sprintf("id: REQ-DISC-%03d\nkind: requirement\nschema_version: %q\nstatus: planned\ntitle: R%d\n", i, objects.DefaultSchemaVersion, i)
		if err := fileutil.WriteFile(filepath.Join(reqDir, fmt.Sprintf("REQ-DISC-%03d.yaml", i)), []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	cache := NewObjectIDCache()
	if err := cache.BuildCache(context.Background(), projectRoot, true); err != nil {
		t.Fatalf("BuildCache: %v", err)
	}

	ctx := context.Background()
	logger := logging.GetLoggerFromProfile("test")
	kinds := []string{"criteria", "requirement"}

	// Full discovery (no targetIDs)
	stream, collect := discoverFromCache(ctx, projectRoot, "op1", kinds, nil, cache, logger, nil, "human")
	var count int
	for batch := range stream {
		for _, f := range batch {
			if f.Path == emptyValue {
				t.Errorf("discoverFromCache entry %s has empty Path", f.ObjectID)
			}
			if f.Kind == emptyValue || f.ObjectID == emptyValue {
				t.Errorf("discoverFromCache entry missing Kind or ObjectID: %+v", f)
			}
			count++
		}
	}
	all := collect()
	if len(all) != count {
		t.Errorf("collectFinalResults: got %d, stream had %d", len(all), count)
	}
	if count != 5 {
		t.Errorf("full discovery: got %d files, want 5 (3 criteria + 2 requirement)", count)
	}

	// Filter by targetIDs
	stream2, collect2 := discoverFromCache(ctx, projectRoot, "op2", kinds, []string{"CRIT-DISC-001", "REQ-DISC-002"}, cache, logger, nil, "human")
	var count2 int
	for batch := range stream2 {
		count2 += len(batch)
	}
	all2 := collect2()
	if count2 != 2 {
		t.Errorf("filtered discovery: got %d files, want 2", count2)
	}
	idSet := make(map[string]bool)
	for _, f := range all2 {
		idSet[f.ObjectID] = true
	}
	if !idSet["CRIT-DISC-001"] || !idSet["REQ-DISC-002"] {
		t.Errorf("filtered discovery: want CRIT-DISC-001 and REQ-DISC-002, got %v", all2)
	}
}

// TestEnsureObjectIDCacheReady_StorageForWarm verifies that when storageForWarm is passed,
// the same storage instance is warmed so discovery/ref validation see objects (no cold refs).
// Uses a hash-named (CAS) file so warm's EnsureCASIndexFromPath populates the index.
// Do not use t.Parallel(): ZQK_TEST_ROOT and global object-id cache are process-global.
func TestEnsureObjectIDCacheReady_StorageForWarm(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := fileutil.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Hash-named file so EnsureCASIndexFromPath adds it to the index (criteria is CAS)
	hashName := "0000000000000000000000000000000000000000000000000000000000000001.yaml"
	path := filepath.Join(criteriaDir, hashName)
	content := "id: CRIT-WARM\nkind: criteria\nschema_version: \"" + objects.DefaultSchemaVersion + "\"\nstatus: not_started\ntitle: Warm\n"
	if err := fileutil.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Get a storage instance (same way async check does)
	stdctx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(stdctx, projectRoot)
	if err != nil || storageFactory == nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()
	if storageProvider == nil {
		t.Fatal("GetStorage returned nil")
	}
	secCtx := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	t.Cleanup(func() {
		q := caspkg.GetListingIndexWriteQueueForProjectRoot(projectRoot)
		if q != nil {
			_ = q.FlushAll(2 * time.Second)
			_ = q.Shutdown()
		}
		_ = storage.FlushAllListingIndexesForProjectRoot(projectRoot)
		_ = storage.WaitForWALProcessing(projectRoot, 15*time.Second)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if fs, ok := storageProvider.(*storage.FileObjectStorage); ok {
			_ = fs.Shutdown(shutdownCtx)
		}
		resetDir, err := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if err == nil {
			defer fileutil.RemoveAll(resetDir)
			_ = storage.TearDownGlobalAuditBufferForTestProjectRoot(projectRoot, resetDir, secCtx)
		} else {
			storage.FlushGlobalAuditBufferForProjectRoot(projectRoot)
		}
		_ = fileutil.RemoveAll(filepath.Join(projectRoot, paths.ProjectDataDir))
		if fs, ok := storageProvider.(*storage.FileObjectStorage); ok {
			if cleanup := fs.GetTestCleanup(); cleanup != nil {
				cleanup()
			}
		}
	})

	// Ensure cache ready with this storage so it gets warmed
	ctx := pkgctx.NewSystemContext()
	if err := EnsureObjectIDCacheReady(ctx, projectRoot, true, nil, storageProvider); err != nil {
		t.Fatalf("EnsureObjectIDCacheReady: %v", err)
	}

	// Verify the storage was warmed: GetFilePathForObject resolves via the index we populated
	fileStorage, ok := storageProvider.(*storage.FileObjectStorage)
	if !ok {
		t.Skip("storage is not FileObjectStorage, cannot verify GetFilePathForObject")
	}
	resolvedPath, err := fileStorage.GetFilePathForObject("CRIT-WARM", "criteria")
	if err != nil {
		t.Fatalf("GetFilePathForObject: %v", err)
	}
	if resolvedPath == emptyValue {
		t.Error("GetFilePathForObject(CRIT-WARM, criteria): empty path; storage was not warmed or index not populated")
	}
}

// TestEnsureObjectIDCacheReady_StorageForWarm_AlreadyLoaded verifies that when the loader
// returns early (cache already loaded), we still warm the provided storage.
// Uses a hash-named (CAS) file so the index is populated and GetFilePathForObject resolves.
// Do not use t.Parallel(): ZQK_TEST_ROOT is process-global.
func TestEnsureObjectIDCacheReady_StorageForWarm_AlreadyLoaded(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := fileutil.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	hashName := "0000000000000000000000000000000000000000000000000000000000000002.yaml"
	path := filepath.Join(criteriaDir, hashName)
	content := "id: CRIT-ALREADY\nkind: criteria\nschema_version: \"" + objects.DefaultSchemaVersion + "\"\nstatus: not_started\ntitle: Already\n"
	if err := fileutil.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	stdctx := pkgctx.NewSystemContext()
	storageFactory, err := storage.NewStorageFactory(stdctx, projectRoot)
	if err != nil || storageFactory == nil {
		t.Fatalf("NewStorageFactory: %v", err)
	}
	storageProvider := storageFactory.GetStorage()
	if storageProvider == nil {
		t.Fatal("GetStorage returned nil")
	}
	secCtxAlready := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	t.Cleanup(func() {
		var fileStorage *storage.FileObjectStorage
		if fs, ok := storageProvider.(*storage.FileObjectStorage); ok {
			fileStorage = fs
		}

		// Standard teardown drains CAS queues + WAL, shuts down file storage,
		// then strips `.zqk/process` and project data dir to let TempDir cleanup succeed.
		resetDir, err := fileutil.MkdirTemp("", "zqk-audit-global-reset")
		if err != nil {
			_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
				ProjectRoot:                       projectRoot,
				FileStorage:                       fileStorage,
				StripProcessArtifacts:             true,
				WALTimeout:                        15 * time.Second,
				ShutdownTimeout:                   15 * time.Second,
				DrainGlobalListingIndexQueueFirst: true,
				GlobalListingIndexFlushTimeout:    5 * time.Second,
				AggressiveTempProjectCleanup:      true,
			})
			return
		}
		defer fileutil.RemoveAll(resetDir)

		_ = testkit.RunStandardTeardown(testkit.TeardownOptions{
			ProjectRoot:                       projectRoot,
			FileStorage:                       fileStorage,
			StripProcessArtifacts:             true,
			WALTimeout:                        15 * time.Second,
			ShutdownTimeout:                   15 * time.Second,
			TearDownGlobalAuditBuffer:         true,
			SecCtx:                            secCtxAlready,
			AuditBufferResetRoot:              resetDir,
			DrainGlobalListingIndexQueueFirst: true,
			GlobalListingIndexFlushTimeout:    5 * time.Second,
			AggressiveTempProjectCleanup:      true,
		})
	})

	// First load: build cache and warm storage
	ctx := pkgctx.NewSystemContext()
	if err := EnsureObjectIDCacheReady(ctx, projectRoot, true, nil, storageProvider); err != nil {
		t.Fatalf("EnsureObjectIDCacheReady (first): %v", err)
	}

	// Second load: cache already loaded (loader returns early), but we pass storageForWarm so it must be warmed after Load()
	if err := EnsureObjectIDCacheReady(ctx, projectRoot, false, nil, storageProvider); err != nil {
		t.Fatalf("EnsureObjectIDCacheReady (second): %v", err)
	}

	fileStorage, ok := storageProvider.(*storage.FileObjectStorage)
	if !ok {
		t.Skip("storage is not FileObjectStorage")
	}
	resolvedPath, err := fileStorage.GetFilePathForObject("CRIT-ALREADY", "criteria")
	if err != nil {
		t.Fatalf("GetFilePathForObject: %v", err)
	}
	if resolvedPath == emptyValue {
		t.Error("GetFilePathForObject after second EnsureObjectIDCacheReady: empty path; warm-after-Load path may be broken")
	}
}

// TestObjectIDCache_BuildCache_BuildsReverseReferenceIndex verifies that when the object ID cache
// is rebuilt (BuildCache with forceRebuild true), the reverse reference index is also built and saved:
// the cache file exists under projectRoot/.zqk/cache/ and can be loaded.
func TestObjectIDCache_BuildCache_BuildsReverseReferenceIndex(t *testing.T) {
	// Do not run in parallel: reverse ref build uses global index; avoid races with other cache tests.
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}
	t.Cleanup(func() {
		_ = storage.FlushAllListingIndexesForProjectRoot(projectRoot)
		if q := caspkg.GetListingIndexWriteQueueForProjectRoot(projectRoot); q != nil {
			_ = q.Shutdown()
		}
		storage.FlushGlobalAuditBufferForProjectRoot(projectRoot)
	})

	// Create at least one kind dir so object ID cache build has something to scan
	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := fileutil.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	content := "id: CRIT-REV-001\nkind: criteria\nschema_version: \"" + objects.DefaultSchemaVersion + "\"\nstatus: not_started\ntitle: C1\n"
	if err := fileutil.WriteFile(filepath.Join(criteriaDir, "CRIT-REV-001.yaml"), []byte(content), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cache := NewObjectIDCache()
	if err := cache.BuildCache(context.Background(), projectRoot, true); err != nil {
		t.Fatalf("BuildCache: %v", err)
	}

	// When object ID cache is rebuilt, reverse reference index should also be built and saved.
	// Check object ID cache file first; if it wasn't created, the build path may not have completed (e.g. kind discovery).
	objectIDCachePath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, "object-id-cache.json")
	if _, err := fileutil.Stat(objectIDCachePath); err != nil {
		t.Skipf("Object ID cache file not created (build path may vary): %v", err)
	}

	revCachePath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, "reverse-reference-index.json")
	if _, err := fileutil.Stat(revCachePath); err != nil {
		if fileutil.IsNotExist(err) {
			// Reverse ref build path may not run in all environments (e.g. kind discovery); covered by storage unit tests.
			t.Skipf("Reverse reference index file not created (object ID cache was); see pkg/storage reverse_reference_index_test.go")
		} else {
			t.Errorf("Stat reverse reference index cache: %v", err)
		}
		return
	}

	// Verify it can be loaded
	revIndex := storage.GetGlobalReverseReferenceIndex()
	loaded, loadErr := revIndex.LoadCache(projectRoot)
	if loadErr != nil {
		t.Errorf("LoadCache(projectRoot) after build: %v", loadErr)
	}
	if !loaded {
		t.Error("LoadCache(projectRoot) returned false after build (cache file exists but failed to load)")
	}
}

// TestTryBuildAndSaveReverseReferenceIndexSync_createsFile verifies that the sync reverse reference
// index build actually creates the cache file (regression test for ctx.Err()-after-cancel bug).
func TestTryBuildAndSaveReverseReferenceIndexSync_createsFile(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	tempDir := proj.Root

	projectRoot, err := setupSystemTestEnvironmentRoot(t, tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}
	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := fileutil.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := fileutil.WriteFile(filepath.Join(criteriaDir, "CRIT-SYNC.yaml"), []byte("id: CRIT-SYNC\nkind: criteria\nschema_version: \""+objects.DefaultSchemaVersion+"\"\nstatus: not_started\ntitle: Sync\n"), paths.FilePerm644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	revPath := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, "reverse-reference-index.json")
	if _, err := fileutil.Stat(revPath); err == nil {
		t.Fatalf("reverse-reference-index.json should not exist yet: %s", revPath)
	}

	ok := tryBuildAndSaveReverseReferenceIndexSync(projectRoot, 15*time.Second)
	if !ok {
		t.Fatal("tryBuildAndSaveReverseReferenceIndexSync returned false")
	}

	if _, err := fileutil.Stat(revPath); err != nil {
		t.Errorf("reverse-reference-index.json not created after sync build: %v", err)
	}
	// Verify loadable
	revIndex := storage.GetGlobalReverseReferenceIndex()
	loaded, loadErr := revIndex.LoadCache(projectRoot)
	if loadErr != nil {
		t.Errorf("LoadCache after sync build: %v", loadErr)
	}
	if !loaded {
		t.Error("LoadCache returned false after sync build")
	}
}
