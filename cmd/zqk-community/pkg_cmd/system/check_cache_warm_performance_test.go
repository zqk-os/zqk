package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/testkit"
	"github.com/lanceman/zqk/pkg/zqkenv"

	"github.com/lanceman/zqk/pkg/objects"
)

// TestWarmCASIndexesFromCache_Isolated isolates the warm-CAS logic and runs it with a short
// flush timeout and per-project queue so the test completes in seconds. Verifies that
// warmCASIndexesFromCache (cache → byKind → EnsureCASIndexFromPaths per kind → flush) works
// and stays fast.
func TestWarmCASIndexesFromCache_Isolated(t *testing.T) {
	// Not t.Parallel(): SetListingIndexWriteQueueFactoryToPerProjectRoot is process-global; parallel tests can contend.
	const (
		numEntries   = 8
		flushTimeout = 2 * time.Second
		maxDuration  = 5 * time.Second
	)
	tempDir := t.TempDir()
	projectRoot, err := setupSystemTestEnvironmentRoot(tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	// Per-project queue so flush only waits for this test's work (no global queue contention).
	storage.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	defer storage.SetListingIndexWriteQueueFactory(nil)

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := os.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("create criteria dir: %v", err)
	}

	// Create a few hash-named criterion files.
	for i := 0; i < numEntries; i++ {
		hashName := fmt.Sprintf("%064x", i) + ".yaml"
		id := fmt.Sprintf("CRIT-ISO-%03d", i+1)
		content := fmt.Sprintf("id: %s\nkind: criteria\nschema_version: %q\nstatus: not_started\ntitle: Isolated %d\n", id, objects.DefaultSchemaVersion, i+1)
		path := filepath.Join(criteriaDir, hashName)
		if err := os.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	// Populate cache with id→path so warm has something to merge (no directory scan in warm).
	cache := NewObjectIDCache()
	for i := 0; i < numEntries; i++ {
		hashName := fmt.Sprintf("%064x", i) + ".yaml"
		id := fmt.Sprintf("CRIT-ISO-%03d", i+1)
		path := filepath.Join(criteriaDir, hashName)
		cache.Set(id, &ObjectIDCacheEntry{
			ID: id, Kind: "criteria", FilePath: path, Exists: true,
		})
	}

	ctx := context.Background()
	storageProvider := getStorageProviderForCache(projectRoot)
	if storageProvider == nil {
		t.Fatal("getStorageProviderForCache returned nil")
	}

	start := time.Now()
	warmCASIndexesFromCache(ctx, projectRoot, cache, storageProvider, flushTimeout)
	elapsed := time.Since(start)
	if elapsed > maxDuration {
		t.Errorf("warmCASIndexesFromCache took %v; expected < %v (isolated warm with %s flush)",
			elapsed, maxDuration, flushTimeout)
	}
	t.Logf("warmCASIndexesFromCache (isolated, %d entries, %s flush) completed in %v", numEntries, flushTimeout, elapsed)
}

// TestWarmCASIndexesFromCache_Performance prevents regression where warmCASIndexesFromCache
// called GetFilePathForObject for every entry (O(entries × files_per_kind) for CAS kinds),
// causing timeouts (e.g. 4m40s) with hundreds of criteria.
// The fix uses EnsureCASIndexFromPath when entry.FilePath is set (O(entries)).
// This test builds a cache with many CAS entries and asserts build+warm completes in bounded time.
// Uses per-project CAS queue so flush only waits for this test's work (keeps test fast).
// Do not use t.Parallel(): same *testing.T uses t.Setenv(ZQK_TEST_ROOT).
func TestWarmCASIndexesFromCache_Performance(t *testing.T) {
	const (
		numCASEntries = 150
		maxDuration   = 15 * time.Second
	)
	tempDir := t.TempDir()
	t.Setenv(zqkenv.TestRoot(), tempDir)
	testkit.RegisterStandardTeardown(t, testkit.TempProjectTeardown(tempDir, nil))

	storage.SetListingIndexWriteQueueFactoryToPerProjectRoot()
	defer storage.SetListingIndexWriteQueueFactory(nil)

	projectRoot, err := setupSystemTestEnvironmentRoot(tempDir)
	if err != nil {
		t.Fatalf("SetupTestEnvironment: %v", err)
	}

	criteriaDir := datacell.CellCASPrimaryDir(projectRoot, "criteria")
	if err := os.MkdirAll(criteriaDir, paths.DirPerm755); err != nil {
		t.Fatalf("create criteria dir: %v", err)
	}

	// Create many hash-named criterion files so cache build populates entries with FilePath.
	// Scanner reads each file for id (hash filenames); createCacheEntry sets FilePath.
	for i := 0; i < numCASEntries; i++ {
		hashName := fmt.Sprintf("%064x", i) + ".yaml"
		id := fmt.Sprintf("CRIT-WARM-%03d", i+1)
		content := fmt.Sprintf("id: %s\nkind: criteria\nschema_version: %q\nstatus: not_started\ntitle: Warm test %d\n", id, objects.DefaultSchemaVersion, i+1)
		path := filepath.Join(criteriaDir, hashName)
		if err := os.WriteFile(path, []byte(content), paths.FilePerm644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}

	cache := NewObjectIDCache()
	start := time.Now()
	err = cache.BuildCache(context.Background(), projectRoot, true)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("BuildCache: %v", err)
	}
	if elapsed > maxDuration {
		t.Errorf("BuildCache (with warm) took %v; expected < %v to avoid O(n×scan) regression (use EnsureCASIndexFromPath for entries with FilePath)",
			elapsed, maxDuration)
	}
	t.Logf("BuildCache with %d CAS entries completed in %v", numCASEntries, elapsed)
}
