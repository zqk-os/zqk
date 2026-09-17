package objectidcache

import (
	"bytes"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestSaveCache_WritesCompactJSON(t *testing.T) {
	projectRoot := t.TempDir()
	if err := fileutil.MkdirAll(datacell.ProcessPrimaryDir(projectRoot), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	c := NewObjectIDCache()
	if err := c.SaveCache(projectRoot); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, paths.ObjectIDCacheFile)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("\n  ")) {
		t.Fatalf("object-id-cache.json is pretty-printed (%d bytes); SaveCache must compact-marshal", len(data))
	}
	if _, _, _, _, err := ParseObjectIDCacheFile(data); err != nil {
		t.Fatal(err)
	}
}

func TestPersistCacheChanges_CoalescesSaves(t *testing.T) {
	projectRoot := t.TempDir()
	if err := fileutil.MkdirAll(datacell.ProcessPrimaryDir(projectRoot), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	oldDebounce, oldMax := cachePersistDebounce, cachePersistMaxDelay
	cachePersistDebounce = time.Hour
	cachePersistMaxDelay = 2 * time.Hour
	prevSave, prevBuild, prevHit, prevMiss := onCacheSave, onCacheBuild, onCacheHit, onCacheMiss
	t.Cleanup(func() {
		FlushPendingObjectIDCachePersist()
		cachePersistDebounce = oldDebounce
		cachePersistMaxDelay = oldMax
		SetCacheLifecycleObservers(prevSave, prevBuild, prevHit, prevMiss)
	})

	cache := GetGlobalObjectIDCache()
	if err := cache.SaveCache(projectRoot); err != nil {
		t.Fatal(err)
	}

	var saves atomic.Int32
	SetCacheLifecycleObservers(func(string, int, time.Duration) {
		saves.Add(1)
	}, prevBuild, prevHit, prevMiss)

	for range 50 {
		persistCacheChanges(cache, projectRoot, "test coalesce")
	}
	FlushPendingObjectIDCachePersist()
	if got := saves.Load(); got != 1 {
		t.Fatalf("coalesced persist: got %d SaveCache calls, want 1", got)
	}
}

func TestUpdate_DraftPlanePathPreserved(t *testing.T) {
	projectRoot := t.TempDir()
	processDir := datacell.ProcessPrimaryDir(projectRoot)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	draftPath := storage.ObjectDraftPlanePath(projectRoot, "goal", "GOAL-draft-cache-1")
	if err := fileutil.MkdirAll(filepath.Dir(draftPath), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(draftPath, []byte("id: GOAL-draft-cache-1\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	c := NewObjectIDCache()
	c.processDir = processDir
	if err := c.Update("GOAL-draft-cache-1", "goal", draftPath); err != nil {
		t.Fatal(err)
	}
	got, ok := c.Get("GOAL-draft-cache-1")
	if !ok {
		t.Fatal("draft id missing from object-id-cache")
	}
	if got.FilePath != draftPath {
		t.Fatalf("cache path=%s want draft plane %s", got.FilePath, draftPath)
	}
}
