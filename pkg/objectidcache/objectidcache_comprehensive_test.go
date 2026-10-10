package objectidcache

import (
	stdcontext "context"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestBackgroundWait_LifecycleAndCallbacks(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proj")
	if HasProjectCacheBackgroundState(root) {
		t.Fatalf("expected no background state initially for %s", root)
	}

	var idleFired atomic.Bool
	unreg := RegisterCacheSidecarsIdleCallback(func(r string) {
		if r == root {
			idleFired.Store(true)
		}
	})
	defer unreg()

	var emitterFired atomic.Bool
	SetSidecarsIdleEmitter(func(r string) {
		if r == root {
			emitterFired.Store(true)
		}
	})
	defer SetSidecarsIdleEmitter(nil)

	projectCacheBgIncObjectID(root)
	if !HasProjectCacheBackgroundState(root) {
		t.Fatal("expected background state to exist after inc")
	}

	projectCacheBgIncReverseRef(root)

	ctxCancel, cancel := stdcontext.WithTimeout(stdcontext.Background(), 20*time.Millisecond)
	defer cancel()
	if err := WaitProjectCacheBackgroundWork(ctxCancel, root); err == nil {
		t.Fatal("expected timeout error while work is pending")
	}

	projectCacheBgDecObjectID(root)
	projectCacheBgDecReverseRef(root)

	ctxSuccess, cancelSuccess := stdcontext.WithTimeout(stdcontext.Background(), time.Second)
	defer cancelSuccess()
	if err := WaitProjectCacheBackgroundWork(ctxSuccess, root); err != nil {
		t.Fatalf("expected WaitProjectCacheBackgroundWork success, got %v", err)
	}

	if !idleFired.Load() {
		t.Fatal("expected idle callback to fire")
	}
	if !emitterFired.Load() {
		t.Fatal("expected sidecar emitter to fire")
	}
}

func TestParseReferenceID_Matrix(t *testing.T) {
	cases := []struct {
		input    string
		wantKind string
		wantID   string
	}{
		{"account:ACC-001", objects.KindAccount, "ACC-001"},
		{"domain:organizational:role:ROL-123", "role", "ROL-123"},
		{"custom_kind:ID-999", "custom_kind", "ID-999"},
		{"BLI-456", "bli", "BLI-456"},
		{"REQ-789", "req", "REQ-789"},
		{"rawstring", "", "rawstring"},
	}

	for _, tc := range cases {
		kind, id := parseReferenceID(tc.input)
		if kind != tc.wantKind || id != tc.wantID {
			t.Errorf("parseReferenceID(%q) = (%q, %q); want (%q, %q)",
				tc.input, kind, id, tc.wantKind, tc.wantID)
		}
	}
}

func TestProjectRootResolver_CustomAndDefault(t *testing.T) {
	def := defaultResolveProjectRoot("/custom/path")
	if def != "/custom/path" {
		t.Errorf("defaultResolveProjectRoot gave %s", def)
	}
	_ = defaultResolveProjectRoot("")
	_ = defaultResolveProjectRoot(".")

	prev := resolveProjectRoot
	defer SetProjectRootResolver(prev)

	SetProjectRootResolver(func(p string) string {
		return "/mocked/root"
	})
	if res := resolveProjectRoot("any"); res != "/mocked/root" {
		t.Errorf("expected /mocked/root, got %s", res)
	}
}

func TestAccessorsAndQueries_Comprehensive(t *testing.T) {
	c := NewObjectIDCache()
	count, isNil := c.SnapshotStats()
	if count != 0 || isNil {
		t.Errorf("expected 0 entries and non-nil map, got count=%d, isNil=%v", count, isNil)
	}

	root := t.TempDir()
	c.Set("BLI-100", &ObjectIDCacheEntry{Kind: "backlog_item", FilePath: filepath.Join(root, "process/backlog_items/BLI-100.yaml"), MTime: time.Now()})
	c.Set("REQ-100", &ObjectIDCacheEntry{Kind: "requirement", FilePath: filepath.Join(root, "process/requirements/REQ-100.yaml"), MTime: time.Now()})

	count, _ = c.SnapshotStats()
	if count != 2 {
		t.Errorf("expected 2 entries, got %d", count)
	}

	ids := c.CollectIDs()
	if len(ids) != 2 {
		t.Errorf("expected 2 collected IDs, got %d", len(ids))
	}

	if n := c.KindBucketCount("backlog_item"); n != 1 {
		t.Errorf("expected 1 backlog_item, got %d", n)
	}

	var visitedCount int
	c.ForEachEntry(func(entry *ObjectIDCacheEntry) {
		visitedCount++
	})
	if visitedCount != 2 {
		t.Errorf("expected 2 visited entries, got %d", visitedCount)
	}

	entries := c.GetEntriesByKind("backlog_item")
	if len(entries) != 1 || entries[0].ID != "BLI-100" {
		t.Errorf("unexpected GetEntriesByKind result: %+v", entries)
	}

	forID := c.EntriesForID("BLI-100")
	if len(forID) != 1 || forID[0].ID != "BLI-100" {
		t.Errorf("unexpected EntriesForID result: %+v", forID)
	}

	kinds := c.GetKinds()
	if len(kinds) != 2 {
		t.Errorf("expected 2 kinds, got %d", len(kinds))
	}

	counts := c.CountByKind()
	if counts["backlog_item"] != 1 || counts["requirement"] != 1 {
		t.Errorf("unexpected CountByKind counts: %+v", counts)
	}

	c.ClearInMemoryCache(root)
}

func TestCacheMutationOperations(t *testing.T) {
	root := t.TempDir()
	c := NewObjectIDCache()

	file1 := filepath.Join(root, "process/backlog_items/BLI-201.yaml")
	_ = fileutil.MkdirAll(filepath.Dir(file1), paths.DirPerm755)
	_ = fileutil.WriteSecureFile(file1, []byte("id: BLI-201\n"))

	c.Set("BLI-201", &ObjectIDCacheEntry{Kind: "backlog_item", FilePath: file1, MTime: time.Now()})
	entry, found := c.Get("BLI-201")
	if !found || entry == nil || entry.Kind != "backlog_item" {
		t.Fatalf("expected BLI-201 to be found, got found=%v, entry=%+v", found, entry)
	}

	file1Moved := filepath.Join(root, "process/backlog_items/BLI-201-moved.yaml")
	_ = fileutil.WriteSecureFile(file1Moved, []byte("id: BLI-201\n"))

	if err := c.Update("BLI-201", "backlog_item", file1Moved); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	entry, _ = c.Get("BLI-201")
	if entry == nil || !strings.Contains(entry.FilePath, "BLI-201-moved.yaml") {
		t.Fatalf("expected updated file path, got %+v", entry)
	}

	c.Invalidate("BLI-201")
	if _, found = c.Get("BLI-201"); found {
		t.Fatal("expected BLI-201 to be invalidated")
	}

	c.Set("BLI-202", &ObjectIDCacheEntry{Kind: "backlog_item", FilePath: filepath.Join(root, "process/backlog_items/BLI-202.yaml"), MTime: time.Now()})
	c.Set("BLI-203", &ObjectIDCacheEntry{Kind: "backlog_item", FilePath: filepath.Join(root, "process/backlog_items/BLI-203.yaml"), MTime: time.Now()})
	c.InvalidateKind("backlog_item")
	if c.KindBucketCount("backlog_item") != 0 {
		t.Fatal("expected backlog_item kind bucket to be empty after InvalidateKind")
	}

	c.Set("BLI-204", &ObjectIDCacheEntry{Kind: "backlog_item", FilePath: filepath.Join(root, "process/backlog_items/BLI-204.yaml"), MTime: time.Now()})
	c.Set("BLI-205", &ObjectIDCacheEntry{Kind: "backlog_item", FilePath: filepath.Join(root, "process/backlog_items/BLI-205.yaml"), MTime: time.Now()})
	c.BulkInvalidate([]string{"BLI-204", "BLI-205"})
	if _, found = c.Get("BLI-204"); found {
		t.Fatal("expected BLI-204 to be invalidated after BulkInvalidate")
	}
}

func TestCacheHandleMutationAndDiscovery(t *testing.T) {
	root := t.TempDir()
	c := NewObjectIDCache()
	sub := &objectIDCacheSubscriber{cache: c}

	file301 := filepath.Join(root, "process/backlog_items/BLI-301.yaml")
	_ = fileutil.MkdirAll(filepath.Dir(file301), paths.DirPerm755)
	_ = fileutil.WriteSecureFile(file301, []byte("id: BLI-301\n"))

	ctx := stdcontext.Background()
	putEvent := storage.MutationEvent{
		Op:   storage.MutationOpPut,
		ID:   "BLI-301",
		Kind: "backlog_item",
		Path: file301,
	}
	if err := sub.HandleMutation(ctx, putEvent); err != nil {
		t.Fatalf("HandleMutation put failed: %v", err)
	}
	if _, found := c.Get("BLI-301"); !found {
		t.Fatal("expected BLI-301 to be added via HandleMutation")
	}

	delEvent := storage.MutationEvent{
		Op:   storage.MutationOpDelete,
		ID:   "BLI-301",
		Kind: "backlog_item",
	}
	if err := sub.HandleMutation(ctx, delEvent); err != nil {
		t.Fatalf("HandleMutation delete failed: %v", err)
	}
	if _, found := c.Get("BLI-301"); found {
		t.Fatal("expected BLI-301 to be removed via HandleMutation")
	}

	pathsList := c.GetFilePathsForKind("nonexistent_kind")
	if len(pathsList) != 0 {
		t.Errorf("expected 0 paths, got %d", len(pathsList))
	}

	kind, loc, ok := c.LocateObject("BLI-NONEXISTENT")
	if ok || loc != "" || kind != "" {
		t.Errorf("expected false, empty location for non-existent object, got (%s, %s, %v)", kind, loc, ok)
	}
}

type testNotifier struct {
	lastStatus  string
	lastMessage string
}

func (n *testNotifier) NotifyCacheProgress(status, message string) {
	n.lastStatus = status
	n.lastMessage = message
}

func TestCacheProgressHooks(t *testing.T) {
	c := NewObjectIDCache()
	notifier := &testNotifier{}
	c.progressNotifier.Store(&progressNotifierHolder{n: notifier})

	cb := &objectIDCacheProgressCallback{cache: c}
	cb.OnLoading()
	if notifier.lastStatus != "loading" {
		t.Errorf("expected loading status, got %s", notifier.lastStatus)
	}

	cb.OnLoaded(nil)
	if notifier.lastStatus != "ready" {
		t.Errorf("expected ready status, got %s", notifier.lastStatus)
	}

	cb.OnError(stdcontext.Canceled)
	if notifier.lastStatus != "error" {
		t.Errorf("expected error status, got %s", notifier.lastStatus)
	}

	cb.OnTimeout()
	if notifier.lastStatus != "timeout" {
		t.Errorf("expected timeout status, got %s", notifier.lastStatus)
	}

	c.notifyCacheProgress("building", "custom building message")
	if notifier.lastStatus != "building" {
		t.Errorf("expected building status, got %s", notifier.lastStatus)
	}
}

func TestEnsureObjectIDCacheReady_Coverage(t *testing.T) {
	projectRoot := t.TempDir()
	if err := fileutil.MkdirAll(datacell.ProcessPrimaryDir(projectRoot), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	c := NewObjectIDCache()
	runner := c.GetEnsureRunner()
	if runner == nil {
		t.Fatal("expected non-nil ensure runner")
	}

	loaded := TryLoadObjectIDCacheOnly(projectRoot)
	_ = loaded

	TriggerBackgroundObjectIDCacheBuild(projectRoot)
	TriggerBackgroundObjectIDCacheForceRebuild(projectRoot)

	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 2*time.Second)
	defer cancel()
	_ = WaitProjectCacheBackgroundWork(ctx, projectRoot)
}

func TestAuditAndObservers(t *testing.T) {
	var auditFired atomic.Bool
	SetCacheAuditFunc(func(eventType, targetID, targetKind, targetPath, operation, severity, profile string) {
		auditFired.Store(true)
	})
	defer SetCacheAuditFunc(nil)

	emitCacheAudit("test_event", "OBJ-1", "backlog_item", "/path", "op", "low", "human")
	if !auditFired.Load() {
		t.Fatal("expected audit func to fire")
	}

	var saveFired, buildFired, hitFired, missFired atomic.Bool
	SetCacheLifecycleObservers(
		func(projectRoot string, entryCount int, saveDuration time.Duration) { saveFired.Store(true) },
		func(projectRoot, operation string, entryCount int, forceRebuild bool, buildDuration time.Duration) {
			buildFired.Store(true)
		},
		func(id string, logger logging.Logger) { hitFired.Store(true) },
		func(id string, cacheSize int, logger logging.Logger) { missFired.Store(true) },
	)
	defer SetCacheLifecycleObservers(nil, nil, nil, nil)

	if onCacheSave != nil {
		onCacheSave("root", 1, time.Millisecond)
	}
	if onCacheBuild != nil {
		onCacheBuild("root", "build", 1, false, time.Millisecond)
	}
	if onCacheHit != nil {
		onCacheHit("OBJ-1", nil)
	}
	if onCacheMiss != nil {
		onCacheMiss("OBJ-2", 0, nil)
	}

	if !saveFired.Load() || !buildFired.Load() || !hitFired.Load() || !missFired.Load() {
		t.Fatal("expected all lifecycle observers to fire")
	}
}

func TestGlobalQueriesAndInvalidations(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	if err := fileutil.MkdirAll(datacell.ProcessPrimaryDir(root), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	testFile := filepath.Join(root, "process/backlog_items/BLI-GLOB.yaml")
	_ = fileutil.MkdirAll(filepath.Dir(testFile), paths.DirPerm755)
	_ = fileutil.WriteSecureFile(testFile, []byte("id: BLI-GLOB\n"))

	if err := UpdateObjectIDCache("BLI-GLOB", "backlog_item", testFile); err != nil {
		t.Fatalf("UpdateObjectIDCache failed: %v", err)
	}

	InvalidateObjectIDCache("BLI-GLOB")
	InvalidateObjectIDCacheKind("backlog_item")
	BulkInvalidateObjectIDCache([]string{"BLI-GLOB"}, root)
	CleanStaleCacheEntries(root)

	c := GetGlobalObjectIDCache()
	_ = c.IsPopulatedForProject(root)
	_ = c.GetEntryCount()
	_ = c.GetMetadata()
}

func TestBuildCache_AndReverseReferenceScan(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	kindDir := datacell.CellCASPrimaryDir(root, "backlog_items")
	_ = fileutil.MkdirAll(kindDir, paths.DirPerm755)

	f1 := filepath.Join(kindDir, "BLI-BUILD-1.yaml")
	_ = fileutil.WriteSecureFile(f1, []byte("id: BLI-BUILD-1\nkind: backlog_item\ntitle: Sample Item\n"))

	c := NewObjectIDCache()
	c.NotifyCacheProgress("init", "testing notify")
	c.SetProgressNotifier(nil)

	WarmCASIndexesFromCache(stdcontext.Background(), root, c, nil, 0)

	ctx := stdcontext.Background()
	if err := c.BuildCache(ctx, root, true); err != nil {
		t.Fatalf("BuildCache failed: %v", err)
	}

	kinds := c.kindNamesForReverseReferenceScan()
	_ = kinds

	_ = c.IsStale(root)
}

func TestLoadCache_AndPersistenceCycle(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	procDir := datacell.ProcessPrimaryDir(root)
	_ = fileutil.MkdirAll(procDir, paths.DirPerm755)

	cache := NewObjectIDCache()
	cache.Set("REQ-123", &ObjectIDCacheEntry{
		ID:       "REQ-123",
		Kind:     "requirement",
		FilePath: filepath.Join(procDir, "requirements/REQ-123.yaml"),
		MTime:    time.Now(),
	})

	if err := cache.SaveCache(root); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}

	loadedCache := NewObjectIDCache()
	ok, err := loadedCache.LoadCache(root)
	if err != nil || !ok {
		t.Fatalf("LoadCache failed: ok=%v, err=%v", ok, err)
	}

	entry, found := loadedCache.Get("REQ-123")
	if !found || entry.Kind != "requirement" {
		t.Fatalf("expected REQ-123 in loaded cache")
	}

	// Test ParseObjectIDCacheFile variations
	_, _, _, _, err = ParseObjectIDCacheFile([]byte("invalid json"))
	if err == nil {
		t.Fatal("expected error on invalid json")
	}

	_, _, _, _, err = ParseObjectIDCacheFile([]byte(`{"version": 1}`))
	if err == nil {
		t.Fatal("expected error on unsupported version")
	}
}

func TestReverseReferenceIndexSync_Execution(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	procDir := datacell.ProcessPrimaryDir(root)
	_ = fileutil.MkdirAll(procDir, paths.DirPerm755)

	// Direct invalid timeout
	if TryBuildAndSaveReverseReferenceIndexSync(root, 0) {
		t.Fatal("expected failure on zero timeout")
	}

	// Valid root execution
	_ = TryBuildAndSaveReverseReferenceIndexSync(root, 200*time.Millisecond)

	// ensureReverseReferenceIndexSync with empty kinds and non-empty kinds
	_ = ensureReverseReferenceIndexSync(root, procDir, nil)
	_ = ensureReverseReferenceIndexSync(root, procDir, []string{"backlog_items"})

	triggerBackgroundReverseReferenceIndexBuild(root)
}

func TestStaleAndBucketIndexing(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	cache := NewObjectIDCache()

	missingFile := filepath.Join(root, "non_existent.yaml")
	cache.Set("BLI-MISSING", &ObjectIDCacheEntry{
		ID:       "BLI-MISSING",
		Kind:     "backlog_item",
		FilePath: missingFile,
		MTime:    time.Now(),
	})

	// Test ValidateAndCleanStale
	cleaned := cache.ValidateAndCleanStale()
	if cleaned != 1 {
		t.Fatalf("expected 1 cleaned item, got %d", cleaned)
	}

	// Test removeIDFromBucketIndexed directly
	list := []KindBucketEntry{
		{ID: "BLI-1", Path: "1.yaml"},
		{ID: "BLI-2", Path: "2.yaml"},
	}
	idIdx := map[string]int{"BLI-1": 0, "BLI-2": 1}
	res := removeIDFromBucketIndexed(list, "BLI-1", idIdx)
	if len(res) != 1 || res[0].ID != "BLI-2" {
		t.Fatalf("unexpected removeIDFromBucketIndexed result: %v", res)
	}
}

func TestPendingDrainingAndFlush(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	procDir := datacell.ProcessPrimaryDir(root)
	_ = fileutil.MkdirAll(procDir, paths.DirPerm755)

	drainObjectIDCachePending(root)
	flushPendingObjectIDCachePersist()

	// Test CountByKind
	c := NewObjectIDCache()
	c.Set("TST-1", &ObjectIDCacheEntry{ID: "TST-1", Kind: "test_case", FilePath: "/tmp/t1"})
	counts := c.CountByKind()
	if counts["test_case"] != 1 {
		t.Fatalf("expected count 1, got %v", counts)
	}

	// Test GetFilePathsForKind
	paths := c.GetFilePathsForKind("test_case")
	if len(paths) != 1 {
		t.Fatalf("unexpected paths count: %v", paths)
	}

	// Test LocateObject
	kind, fPath, found := c.LocateObject("TST-1")
	if !found || !strings.HasSuffix(fPath, "t1") || kind != "test_case" {
		t.Fatalf("expected LocateObject to find TST-1, got path=%s kind=%s found=%v", fPath, kind, found)
	}
}

func TestNormalizeByKindKeys(t *testing.T) {
	byKind := map[string][]KindBucketEntry{
		"unknown_key": {
			{ID: "BLI-NORM-1", Path: "backlog_items/item1.yaml"},
		},
		"unresolvable_key": {
			{ID: "RAW-1", Path: "plain_file.yaml"},
		},
	}
	idToKind := map[string]string{
		"BLI-NORM-1": "unknown_key",
		"RAW-1":      "unresolvable_key",
	}
	countByKind := map[string]int{
		"unknown_key":      1,
		"unresolvable_key": 1,
	}

	normalizeByKindKeys(byKind, idToKind, countByKind)

	if idToKind["BLI-NORM-1"] != "backlog_item" {
		t.Fatalf("expected BLI-NORM-1 mapped to backlog_item, got %s", idToKind["BLI-NORM-1"])
	}
	if _, ok := byKind["unknown_key"]; ok {
		t.Fatal("expected unknown_key bucket to be removed")
	}
}

func TestTryLoadObjectIDCacheOnly_AndEnsureReady(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	procDir := datacell.ProcessPrimaryDir(root)
	_ = fileutil.MkdirAll(procDir, paths.DirPerm755)

	// Direct invalid root
	if TryLoadObjectIDCacheOnly("/non/existent/root/path") {
		t.Fatal("expected false for nonexistent root")
	}

	// Save valid cache for global cache
	glob := GetGlobalObjectIDCache()
	glob.Set("GOAL-999", &ObjectIDCacheEntry{
		ID:       "GOAL-999",
		Kind:     "goal",
		FilePath: filepath.Join(procDir, "goals/GOAL-999.yaml"),
		MTime:    time.Now(),
		Exists:   true,
	})
	_ = glob.SaveCache(root)

	// Now TryLoadObjectIDCacheOnly should succeed
	_ = TryLoadObjectIDCacheOnly(root)

	// EnsureObjectIDCacheReady test
	ctx := stdcontext.Background()
	_ = EnsureObjectIDCacheReady(ctx, root, false, nil, nil)
	_ = EnsureObjectIDCacheReady(ctx, root, true, nil, nil)
}

func TestGlobalCacheInvalidationsWithData(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	procDir := datacell.ProcessPrimaryDir(root)
	_ = fileutil.MkdirAll(procDir, paths.DirPerm755)

	glob := GetGlobalObjectIDCache()
	testFile := filepath.Join(procDir, "plans/PRI-TEST-1.yaml")
	_ = fileutil.MkdirAll(filepath.Dir(testFile), paths.DirPerm755)
	_ = fileutil.WriteSecureFile(testFile, []byte("id: PRI-TEST-1\n"))

	glob.Set("PRI-TEST-1", &ObjectIDCacheEntry{
		ID:       "PRI-TEST-1",
		Kind:     "priority_plan",
		FilePath: testFile,
		MTime:    time.Now(),
		Exists:   true,
	})
	glob.Set("PRI-TEST-2", &ObjectIDCacheEntry{
		ID:       "PRI-TEST-2",
		Kind:     "priority_plan",
		FilePath: filepath.Join(procDir, "plans/PRI-TEST-2.yaml"),
		MTime:    time.Now(),
		Exists:   true,
	})

	InvalidateObjectIDCacheKind("priority_plan")

	glob.Set("REQ-AUDIT-1", &ObjectIDCacheEntry{
		ID:       "REQ-AUDIT-1",
		Kind:     "requirement",
		FilePath: "/non/existent/req.yaml",
		MTime:    time.Now(),
		Exists:   true,
	})
	_ = BulkInvalidateObjectIDCache([]string{"REQ-AUDIT-1"}, root)

	glob.Set("BLI-STALE-1", &ObjectIDCacheEntry{
		ID:       "BLI-STALE-1",
		Kind:     "backlog_item",
		FilePath: "/non/existent/bli.yaml",
		MTime:    time.Now(),
		Exists:   true,
	})
	_ = CleanStaleCacheEntries(root)

	glob.ClearInMemoryCache(root)
}

func TestIsStale_Comprehensive(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	procDir := datacell.ProcessPrimaryDir(root)
	_ = fileutil.MkdirAll(filepath.Join(procDir, "goals"), paths.DirPerm755)

	realFile := filepath.Join(procDir, "goals/real.yaml")
	_ = fileutil.WriteSecureFile(realFile, []byte("content"))
	info, _ := fileutil.Stat(realFile)

	c := NewObjectIDCache()
	c.Set("FRESH-ENTRY", &ObjectIDCacheEntry{
		ID:       "FRESH-ENTRY",
		Kind:     "goal",
		FilePath: realFile,
		MTime:    info.ModTime(),
		Exists:   true,
	})
	_ = c.SaveCache(root)

	loaded := NewObjectIDCache()
	_, _ = loaded.LoadCache(root)

	// 1. Entry does not exist
	if loaded.IsStale("NON-EXISTENT") {
		t.Fatal("expected IsStale=false for non-existent entry")
	}

	// 2. Fresh entry
	if loaded.IsStale("FRESH-ENTRY") {
		t.Fatal("expected IsStale=false for fresh entry")
	}

	// 3. Entry file exists but mtime differs
	loaded.Set("STALE-MTIME", &ObjectIDCacheEntry{
		ID:       "STALE-MTIME",
		Kind:     "goal",
		FilePath: realFile,
		MTime:    time.Now().Add(-10 * time.Minute),
		Exists:   true,
	})
	if !loaded.IsStale("STALE-MTIME") {
		t.Fatal("expected IsStale=true for mismatched mtime")
	}

	// 4. Entry file missing
	loaded.Set("STALE-MISSING", &ObjectIDCacheEntry{
		ID:       "STALE-MISSING",
		Kind:     "goal",
		FilePath: filepath.Join(procDir, "goals/missing.yaml"),
		MTime:    time.Now(),
		Exists:   true,
	})
	if !loaded.IsStale("STALE-MISSING") {
		t.Fatal("expected IsStale=true for missing file")
	}
}

func TestDrainObjectIDCachePending_WithJournal(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	procDir := datacell.ProcessPrimaryDir(root)
	_ = fileutil.MkdirAll(procDir, paths.DirPerm755)

	storage.ResetObjectIDCachePendingForTest()
	defer storage.ResetObjectIDCachePendingForTest()

	f1 := filepath.Join(procDir, "goals/GOAL-PENDING-1.yaml")
	_ = fileutil.MkdirAll(filepath.Dir(f1), paths.DirPerm755)
	_ = fileutil.WriteSecureFile(f1, []byte("id: GOAL-PENDING-1\n"))

	storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpUpdate), "GOAL-PENDING-1", "goal", f1, "test")
	storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpInvalidate), "GOAL-PENDING-2", "goal", "/dummy", "test")

	drainObjectIDCachePending(root)
}

func TestBuildCache_FastLoadAndCanceled(t *testing.T) {
	root := t.TempDir()
	t.Cleanup(func() {
		TeardownObjectIDCacheTestRoot(root)
	})
	procDir := datacell.ProcessPrimaryDir(root)
	_ = fileutil.MkdirAll(filepath.Join(procDir, "goals"), paths.DirPerm755)

	f1 := filepath.Join(procDir, "goals/GOAL-LOAD.yaml")
	_ = fileutil.WriteSecureFile(f1, []byte("id: GOAL-LOAD\n"))
	info, _ := fileutil.Stat(f1)

	c := NewObjectIDCache()
	c.Set("GOAL-LOAD", &ObjectIDCacheEntry{
		ID:       "GOAL-LOAD",
		Kind:     "goal",
		FilePath: f1,
		MTime:    info.ModTime(),
		Exists:   true,
	})
	_ = c.SaveCache(root)

	// Test 1: canceled context
	canceledCtx, cancel := stdcontext.WithCancel(stdcontext.Background())
	cancel()
	if err := c.BuildCache(canceledCtx, root, false); err == nil {
		t.Fatal("expected error on canceled context")
	}

	// Test 2: normal load existing cache via BuildCache
	c2 := NewObjectIDCache()
	if err := c2.BuildCache(stdcontext.Background(), root, false); err != nil {
		t.Fatalf("BuildCache with existing cache failed: %v", err)
	}

	// Test 3: warmWorkerCap
	if cap := warmWorkerCap(); cap < 4 || cap > 16 {
		t.Fatalf("unexpected warmWorkerCap: %d", cap)
	}

	// Test 4: getWarmStorageForBuild
	if s := c2.getWarmStorageForBuild(); s != nil {
		t.Fatal("expected nil warm storage initially")
	}
	c2.warmStorageForNextBuild.Store(&warmStorageHolder{storage: nil})
	_ = c2.getWarmStorageForBuild()
}

func TestSet_KindReplacementAndEdgeCases(t *testing.T) {
	c := NewObjectIDCache()

	// Edge cases: nil or empty kind
	c.Set("DUMMY", nil)
	c.Set("DUMMY", &ObjectIDCacheEntry{Kind: ""})

	// Add entry
	c.Set("OBJ-MUTATE", &ObjectIDCacheEntry{
		ID:       "OBJ-MUTATE",
		Kind:     "goal",
		FilePath: "/path/1",
	})
	if c.KindBucketCount("goal") != 1 {
		t.Fatalf("expected 1 goal, got %d", c.KindBucketCount("goal"))
	}

	// Mutate to backlog_item
	c.Set("OBJ-MUTATE", &ObjectIDCacheEntry{
		ID:       "OBJ-MUTATE",
		Kind:     "backlog_item",
		FilePath: "/path/2",
	})
	if c.KindBucketCount("goal") != 0 {
		t.Fatalf("expected 0 goals, got %d", c.KindBucketCount("goal"))
	}
	if c.KindBucketCount("backlog_item") != 1 {
		t.Fatalf("expected 1 backlog_item, got %d", c.KindBucketCount("backlog_item"))
	}
}

func TestRemoveIDFromBucketIndexed_Matrix(t *testing.T) {
	// 1. Empty list
	if res := removeIDFromBucketIndexed(nil, "ID", nil); len(res) != 0 {
		t.Fatal("expected empty list")
	}

	// 2. Index drift -> fallback linear scan finds it
	list := []KindBucketEntry{
		{ID: "A", Path: "a.yaml"},
		{ID: "B", Path: "b.yaml"},
		{ID: "C", Path: "c.yaml"},
	}
	idToIndex := map[string]int{"B": 999} // bad index
	res := removeIDFromBucketIndexed(list, "B", idToIndex)
	if len(res) != 2 || res[1].ID != "C" {
		t.Fatalf("unexpected result after fallback linear scan: %v", res)
	}

	// 3. Fallback linear scan does NOT find it
	res2 := removeIDFromBucketIndexed(res, "NON_EXISTENT", idToIndex)
	if len(res2) != 2 {
		t.Fatal("expected same slice when id not found")
	}

	// 4. Remove last element directly (i == last)
	idToIndex2 := map[string]int{"A": 0, "C": 1}
	res3 := removeIDFromBucketIndexed(res, "C", idToIndex2)
	if len(res3) != 1 || res3[0].ID != "A" {
		t.Fatalf("unexpected result after removing last: %v", res3)
	}
}

func TestCountByKind_AndClearInMemoryWithRoot(t *testing.T) {
	root := "/test/project/root"
	c := NewObjectIDCache()

	// Empty countByKind branch
	c.byKind["goal"] = []KindBucketEntry{{ID: "G1"}}
	c.countByKind = nil
	counts := c.CountByKind()
	if counts["goal"] != 1 {
		t.Fatalf("expected 1 goal from byKind scan, got %d", counts["goal"])
	}

	// Populated countByKind branch
	c.countByKind = map[string]int{"goal": 5}
	counts2 := c.CountByKind()
	if counts2["goal"] != 5 {
		t.Fatalf("expected 5 goals from countByKind map, got %d", counts2["goal"])
	}

	// ClearInMemoryCache with matching root
	c.metadata = &ObjectIDCacheMetadata{ProjectRoot: root}
	c.ClearInMemoryCache(root)
	if c.metadata != nil || len(c.byKind) != 0 {
		t.Fatal("expected cache to be cleared after ClearInMemoryCache with matching root")
	}
}
