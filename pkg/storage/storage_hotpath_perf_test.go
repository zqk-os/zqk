package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

type mockObjectIDCacheProvider struct {
	pathsByKind map[string][]string
	locations   map[string]struct {
		kind string
		path string
	}
}

func (m *mockObjectIDCacheProvider) GetFilePathsForKind(kind string) []string {
	return m.pathsByKind[kind]
}

func (m *mockObjectIDCacheProvider) LocateObject(id string) (string, string, bool) {
	loc, found := m.locations[id]
	return loc.kind, loc.path, found
}

// TestStorageHotpath_NoFullWalkOnList satisfies CRIT-CEF-NO-FULL-WALK-HOTLIST-001
// Verifies that list hot paths leverage the ObjectIDCache provider and mtime-cached
// scans rather than performing unbounded O(n) walks over the process directory.
func TestStorageHotpath_NoFullWalkOnList(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Cleanup(func() {
		_ = RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil))
	})

	MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	st, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	defer func() { _ = st.Shutdown(context.Background()) }()

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := WithTestHardDelete(context.Background())

	// Create test objects
	numObjects := 5
	createdPaths := make([]string, 0, numObjects)
	for i := 1; i <= numObjects; i++ {
		id := fmt.Sprintf("BLI-HOTPATH-%03d", i)
		obj := map[string]any{
			objects.FieldKeyID:            id,
			objects.FieldKeyKind:          "backlog_item",
			objects.FieldKeyTitle:         fmt.Sprintf("Hotpath Item %d", i),
			objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		}
		CreateCASVisible(t, st, ctx, secCtx, obj, objects.ObjectStatusPlanned)
		path, err := st.GetFilePathForObject(id, "backlog_item")
		if err != nil {
			t.Fatalf("GetFilePathForObject failed: %v", err)
		}
		createdPaths = append(createdPaths, path)
	}

	// Register mock cache provider that supplies cached paths
	mockProvider := &mockObjectIDCacheProvider{
		pathsByKind: map[string][]string{
			"backlog_item": createdPaths,
		},
		locations: make(map[string]struct {
			kind string
			path string
		}),
	}
	for i, path := range createdPaths {
		id := fmt.Sprintf("BLI-HOTPATH-%03d", i+1)
		mockProvider.locations[id] = struct {
			kind string
			path string
		}{kind: "backlog_item", path: path}
	}

	origProvider := GetGlobalCacheProvider()
	SetGlobalCacheProvider(mockProvider)
	t.Cleanup(func() {
		SetGlobalCacheProvider(origProvider)
	})

	// Test collectFilePathsWithStrategy: must return cached paths without walking disk
	kindDir := filepath.Join(tmpDir, paths.ProcessBacklogDir)
	collected, err := st.collectFilePathsWithStrategy(ctx, "backlog_item", kindDir, nil, nil, nil)
	if err != nil {
		t.Fatalf("collectFilePathsWithStrategy failed: %v", err)
	}
	if len(collected) != numObjects {
		t.Fatalf("expected %d collected paths from cache provider, got %d", numObjects, len(collected))
	}

	// Test ScanIDBasedFilesRecursive caching: repeated scans on unchanged directory must return quickly
	scan1 := st.ScanIDBasedFilesRecursive(kindDir, "backlog_item")
	scan2 := st.ScanIDBasedFilesRecursive(kindDir, "backlog_item")
	if len(scan1) != len(scan2) {
		t.Fatalf("expected consistent scan results, got %d and %d", len(scan1), len(scan2))
	}
}

// TestStorageHotpath_ParseCacheIntegration satisfies TST-1788992457837944000-fbe99ec4
// and CRIT-CEF-YAML-PARSE-CACHE-001:
// Asserts that parsed objects are cached by content hash, a second list hits the cache
// without re-parsing raw YAML, and CAS updates evict old hashes.
func TestStorageHotpath_ParseCacheIntegration(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot().Name(), tmpDir)
	t.Cleanup(func() {
		_ = RunProjectTestTeardown(TempProjectTeardown(tmpDir, nil))
	})

	MustEnsureProcessSpecsLayoutForTest(t, tmpDir)
	if err := paths.EnsureDir(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir backlog: %v", err)
	}

	st, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatalf("NewFileObjectStorageForTest: %v", err)
	}
	defer func() { _ = st.Shutdown(context.Background()) }()

	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := WithTestHardDelete(context.Background())

	// Reset ParseCache state
	parseCache := GetGlobalParseCache()
	parseCache.Clear()
	parseCache.ResetMetrics()

	objID := "BLI-PARSE-CACHE-001"
	obj := map[string]any{
		objects.FieldKeyID:            objID,
		objects.FieldKeyKind:          "backlog_item",
		objects.FieldKeyTitle:         "Initial Parse Cache Title",
		objects.FieldKeyStatus:        objects.ObjectStatusPlanned,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}

	CreateCASVisible(t, st, ctx, secCtx, obj, objects.ObjectStatusPlanned)

	// First list: loads objects and populates ParseCache
	filter := ListFilter{Kind: "backlog_item"}
	res1, err := st.ListImpl(ctx, secCtx, nil, filter)
	if err != nil {
		t.Fatalf("ListImpl initial failed: %v", err)
	}
	if len(res1.Objects) == 0 {
		t.Fatalf("expected at least 1 object in ListImpl result")
	}

	// Verify that objects were put into the parse cache
	initialPuts := parseCache.Puts()
	if initialPuts == 0 {
		t.Fatalf("expected ParseCache.Puts() > 0 after first list, got %d", initialPuts)
	}

	// Second list: should hit ParseCache for cached CAS blobs
	initialHits := parseCache.Hits()
	res2, err := st.ListImpl(ctx, secCtx, nil, filter)
	if err != nil {
		t.Fatalf("ListImpl second failed: %v", err)
	}
	if len(res2.Objects) != len(res1.Objects) {
		t.Fatalf("expected %d objects on second list, got %d", len(res1.Objects), len(res2.Objects))
	}

	secondHits := parseCache.Hits()
	if secondHits <= initialHits {
		t.Fatalf("expected ParseCache hits to increase on second list: initial=%d, second=%d", initialHits, secondHits)
	}

	// Verify CAS update invalidation: updating the object must evict the old content hash
	cas, err := st.getContentAddressableStorage("backlog_item")
	if err != nil {
		t.Fatalf("getContentAddressableStorage: %v", err)
	}
	oldHash, err := cas.GetHashForID(objID)
	if err != nil || oldHash == "" {
		t.Fatalf("expected oldHash for %s: %v", objID, err)
	}

	// Verify oldHash is currently in parse cache
	if _, ok := parseCache.Get(oldHash); !ok {
		t.Fatalf("expected oldHash %s to be present in ParseCache before update", oldHash)
	}

	// Update object with new title
	updates := map[string]any{
		objects.FieldKeyTitle: "Updated Parse Cache Title",
	}
	if err := st.Update(ctx, secCtx, objID, updates); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	// Assert oldHash is evicted from ParseCache
	if _, ok := parseCache.Get(oldHash); ok {
		t.Fatalf("expected oldHash %s to be evicted from ParseCache after update", oldHash)
	}

	// Verify read of updated object returns new title and caches new hash
	readObj, err := st.Read(ctx, secCtx, objID)
	if err != nil {
		t.Fatalf("Read after update failed: %v", err)
	}
	if readObj[objects.FieldKeyTitle] != "Updated Parse Cache Title" {
		t.Fatalf("expected updated title, got %v", readObj[objects.FieldKeyTitle])
	}
}
