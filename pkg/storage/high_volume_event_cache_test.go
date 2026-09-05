package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/pipeline"
)

func TestHighVolumeEventCache_GetSetInvalidate(t *testing.T) {
	cache := NewHighVolumeEventCache()

	// Test Get on empty cache
	_, exists := cache.Get("AUD-001")
	if exists {
		t.Error("expected cache miss for empty cache")
	}

	// Test Set
	now := time.Now().UTC()
	entry := &HighVolumeEventCacheEntry{
		ID:        "AUD-001",
		Kind:      "audit_event",
		CreatedAt: now,
		EventType: "object_created",
		FilePath:  "/path/to/audit/AUD-001.yaml",
		MTime:     now,
		Exists:    true,
	}
	cache.Set(entry)

	// Test Get after Set
	retrieved, exists := cache.Get("AUD-001")
	if !exists {
		t.Error("expected cache hit after Set")
	}
	if retrieved.ID != "AUD-001" {
		t.Errorf("retrieved.ID = %q, want AUD-001", retrieved.ID)
	}
	if retrieved.EventType != "object_created" {
		t.Errorf("retrieved.EventType = %q, want object_created", retrieved.EventType)
	}

	// Test Invalidate
	cache.Invalidate("AUD-001")
	_, exists = cache.Get("AUD-001")
	if exists {
		t.Error("expected cache miss after Invalidate")
	}
}

func TestHighVolumeEventCache_Count(t *testing.T) {
	cache := NewHighVolumeEventCache()

	// Empty cache
	if count := cache.Count(); count != 0 {
		t.Errorf("Count() on empty cache = %d, want 0", count)
	}

	// Add entries
	now := time.Now().UTC()
	for i := 1; i <= 5; i++ {
		entry := &HighVolumeEventCacheEntry{
			ID:        "AUD-" + string(rune('0'+i)),
			Kind:      "audit_event",
			CreatedAt: now.Add(time.Duration(i) * time.Hour),
			Exists:    true,
		}
		cache.Set(entry)
	}

	if count := cache.Count(); count != 5 {
		t.Errorf("Count() = %d, want 5", count)
	}

	// CountByKind
	if count := cache.CountByKind("audit_event"); count != 5 {
		t.Errorf("CountByKind(audit_event) = %d, want 5", count)
	}
	if count := cache.CountByKind("nonexistent"); count != 0 {
		t.Errorf("CountByKind(nonexistent) = %d, want 0", count)
	}
}

func TestHighVolumeEventCache_QueryByTimeWindow(t *testing.T) {
	cache := NewHighVolumeEventCache()

	baseTime := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add entries with different timestamps
	entries := []struct {
		id        string
		createdAt time.Time
	}{
		{"AUD-001", baseTime.Add(-2 * time.Hour)}, // Before window
		{"AUD-002", baseTime.Add(-1 * time.Hour)}, // In window
		{"AUD-003", baseTime},                     // In window
		{"AUD-004", baseTime.Add(1 * time.Hour)},  // In window
		{"AUD-005", baseTime.Add(2 * time.Hour)},  // After window
	}

	for _, e := range entries {
		cache.Set(&HighVolumeEventCacheEntry{
			ID:        e.id,
			Kind:      "audit_event",
			CreatedAt: e.createdAt,
			Exists:    true,
		})
	}

	// Query window: 1 hour before to 1 hour after baseTime
	startTime := baseTime.Add(-1 * time.Hour)
	endTime := baseTime.Add(1 * time.Hour)
	results := cache.QueryByTimeWindow(startTime, endTime, 0)

	if len(results) != 3 {
		t.Errorf("QueryByTimeWindow returned %d IDs, want 3", len(results))
	}

	// Verify IDs are in window
	expectedIDs := map[string]bool{"AUD-002": true, "AUD-003": true, "AUD-004": true}
	for _, id := range results {
		if !expectedIDs[id] {
			t.Errorf("unexpected ID in results: %s", id)
		}
		delete(expectedIDs, id)
	}
	if len(expectedIDs) > 0 {
		t.Errorf("missing IDs in results: %v", expectedIDs)
	}

	// Test with limit
	limited := cache.QueryByTimeWindow(startTime, endTime, 2)
	if len(limited) != 2 {
		t.Errorf("QueryByTimeWindow with limit 2 returned %d IDs, want 2", len(limited))
	}
}

func TestHighVolumeEventCache_QueryOldestByKind_MultipleKinds(t *testing.T) {
	cache := NewHighVolumeEventCache()
	base := time.Date(2030, 2, 1, 12, 0, 0, 0, time.UTC)
	// Mix audit_event and base_metric with different created_at; QueryOldestByKind must return only requested kind in created_at order.
	for i, kind := range []string{"audit_event", "base_metric", "audit_event", "base_metric"} {
		created := base.Add(time.Duration(i) * time.Hour)
		cache.Set(&HighVolumeEventCacheEntry{
			ID:        fmt.Sprintf("%s-%d", kind, i),
			Kind:      kind,
			CreatedAt: created,
			Exists:    true,
		})
	}
	// Oldest overall: audit_event-0, base_metric-1, audit_event-2, base_metric-3. Per-kind oldest:
	auditIDs := cache.QueryOldestByKind("audit_event", 10)
	if len(auditIDs) != 2 {
		t.Errorf("QueryOldestByKind(audit_event) len = %d, want 2", len(auditIDs))
	}
	if len(auditIDs) >= 2 && (auditIDs[0] != "audit_event-0" || auditIDs[1] != "audit_event-2") {
		t.Errorf("QueryOldestByKind(audit_event) = %v, want [audit_event-0 audit_event-2]", auditIDs)
	}
	metricIDs := cache.QueryOldestByKind("base_metric", 10)
	if len(metricIDs) != 2 {
		t.Errorf("QueryOldestByKind(base_metric) len = %d, want 2", len(metricIDs))
	}
	if len(metricIDs) >= 2 && (metricIDs[0] != "base_metric-1" || metricIDs[1] != "base_metric-3") {
		t.Errorf("QueryOldestByKind(base_metric) = %v, want [base_metric-1 base_metric-3]", metricIDs)
	}
	// Unrelated kind returns empty
	other := cache.QueryOldestByKind("change_journal_entry", 10)
	if len(other) != 0 {
		t.Errorf("QueryOldestByKind(change_journal_entry) = %v, want []", other)
	}
}

func TestHighVolumeEventCache_QueryOldestByKindExcludingStatus(t *testing.T) {
	cache := NewHighVolumeEventCache()
	base := time.Date(2030, 2, 1, 12, 0, 0, 0, time.UTC)
	// audit_event: oldest first; mix statuses so we exclude "pending"
	cache.Set(&HighVolumeEventCacheEntry{ID: "AUD-a", Kind: "audit_event", CreatedAt: base, Status: objects.ObjectStatusCompleted, Exists: true})
	cache.Set(&HighVolumeEventCacheEntry{ID: "AUD-b", Kind: "audit_event", CreatedAt: base.Add(time.Hour), Status: objects.ObjectStatusPending, Exists: true})
	cache.Set(&HighVolumeEventCacheEntry{ID: "AUD-c", Kind: "audit_event", CreatedAt: base.Add(2 * time.Hour), Status: objects.ObjectStatusCompleted, Exists: true})
	cache.Set(&HighVolumeEventCacheEntry{ID: "AUD-d", Kind: "audit_event", CreatedAt: base.Add(3 * time.Hour), Status: objects.ObjectStatusPending, Exists: true})
	ids := cache.QueryOldestByKindExcludingStatus("audit_event", 10, []string{objects.ObjectStatusPending})
	if len(ids) != 2 {
		t.Errorf("QueryOldestByKindExcludingStatus(..., pending) len = %d, want 2", len(ids))
	}
	if len(ids) >= 2 && (ids[0] != "AUD-a" || ids[1] != "AUD-c") {
		t.Errorf("QueryOldestByKindExcludingStatus = %v, want [AUD-a AUD-c]", ids)
	}
	// Entries with empty Status are skipped (safe for retention)
	cache.Set(&HighVolumeEventCacheEntry{ID: "AUD-e", Kind: "audit_event", CreatedAt: base.Add(-time.Hour), Status: objects.ObjectStatusUnspecified, Exists: true})
	ids2 := cache.QueryOldestByKindExcludingStatus("audit_event", 10, []string{"pending"})
	if len(ids2) != 2 {
		t.Errorf("with empty Status entry: len = %d, want 2 (AUD-e skipped)", len(ids2))
	}
}

func TestHighVolumeEventCache_QueryOlderThan(t *testing.T) {
	cache := NewHighVolumeEventCache()

	cutoffTime := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add entries before and after cutoff
	for i := 1; i <= 3; i++ {
		cache.Set(&HighVolumeEventCacheEntry{
			ID:        "AUD-OLD-" + string(rune('0'+i)),
			Kind:      "audit_event",
			CreatedAt: cutoffTime.Add(-time.Duration(i) * time.Hour),
			Exists:    true,
		})
	}
	for i := 1; i <= 2; i++ {
		cache.Set(&HighVolumeEventCacheEntry{
			ID:        "AUD-NEW-" + string(rune('0'+i)),
			Kind:      "audit_event",
			CreatedAt: cutoffTime.Add(time.Duration(i) * time.Hour),
			Exists:    true,
		})
	}

	results := cache.QueryOlderThan(cutoffTime, 0)
	if len(results) != 3 {
		t.Errorf("QueryOlderThan returned %d IDs, want 3", len(results))
	}
}

func TestHighVolumeEventCache_IsPopulatedForProject(t *testing.T) {
	cache := NewHighVolumeEventCache()

	projectRoot := "/test/project"

	// Empty cache
	if cache.IsPopulatedForProject(projectRoot) {
		t.Error("IsPopulatedForProject should return false for empty cache")
	}

	// Add entry (but metadata not set, so still not populated)
	cache.Set(&HighVolumeEventCacheEntry{
		ID:        "AUD-001",
		Kind:      "audit_event",
		CreatedAt: time.Now().UTC(),
		Exists:    true,
	})
	if cache.IsPopulatedForProject(projectRoot) {
		t.Error("IsPopulatedForProject should return false when metadata not set")
	}
}

func TestHighVolumeEventCache_SaveAndLoad(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := tempDir

	cache1 := NewHighVolumeEventCache()

	// Add entries
	now := time.Now().UTC()
	for i := 1; i <= 3; i++ {
		cache1.Set(&HighVolumeEventCacheEntry{
			ID:        "AUD-" + string(rune('0'+i)),
			Kind:      "audit_event",
			CreatedAt: now.Add(time.Duration(i) * time.Hour),
			EventType: "object_created",
			Exists:    true,
		})
	}

	// Save cache
	if err := cache1.SaveCache(projectRoot); err != nil {
		t.Fatalf("SaveCache failed: %v", err)
	}

	// Load into new cache instance
	cache2 := NewHighVolumeEventCache()
	loaded, err := cache2.LoadCache(projectRoot)
	if err != nil {
		t.Fatalf("LoadCache failed: %v", err)
	}
	if !loaded {
		t.Error("LoadCache returned false, expected true")
	}

	// Verify entries
	for i := 1; i <= 3; i++ {
		id := "AUD-" + string(rune('0'+i))
		entry, exists := cache2.Get(id)
		if !exists {
			t.Errorf("entry %s not found after LoadCache", id)
			continue
		}
		if entry.Kind != "audit_event" {
			t.Errorf("entry %s Kind = %q, want audit_event", id, entry.Kind)
		}
		if entry.EventType != "object_created" {
			t.Errorf("entry %s EventType = %q, want object_created", id, entry.EventType)
		}
	}

	// Verify count
	if cache2.Count() != 3 {
		t.Errorf("cache2.Count() = %d, want 3", cache2.Count())
	}
}

// TestHighVolumeEventCache_BuildCache tests BuildCache integration with storage.
// Note: This test is skipped because it requires a full test environment with object specs.
// BuildCache is tested indirectly through integration tests and production usage.
func TestHighVolumeEventCache_BuildCache(t *testing.T) {
	t.Skip("BuildCache requires full test environment with object specs; tested via integration tests")
}

func TestHighVolumeEventCache_ConcurrentAccess(t *testing.T) {
	cache := NewHighVolumeEventCache()

	// Concurrent writes
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		goroutinelabels.NewGoroutine("storage_test", "concurrent cache write").StartSimple(func() {
			func(id int) {
				defer func() { done <- true }()
				for j := 0; j < 10; j++ {
					entry := &HighVolumeEventCacheEntry{
						ID:        fmt.Sprintf("AUD-CONC-%d-%d", id, j),
						Kind:      "audit_event",
						CreatedAt: time.Now().UTC(),
						Exists:    true,
					}
					cache.Set(entry)
				}
			}(i)
		})
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		goroutinelabels.NewGoroutine("storage_test", "concurrent cache read").StartSimple(func() {
			func(id int) {
				defer func() { done <- true }()
				for j := 0; j < 10; j++ {
					id := fmt.Sprintf("AUD-CONC-%d-%d", id, j)
					_, _ = cache.Get(id)
					_ = cache.Count()
				}
			}(i)
		})
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify final count
	if count := cache.Count(); count != 100 {
		t.Errorf("cache.Count() after concurrent access = %d, want 100", count)
	}
}

func TestHighVolumeEventCache_CountByTimeWindow(t *testing.T) {
	cache := NewHighVolumeEventCache()

	baseTime := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

	// Add entries
	for i := 1; i <= 5; i++ {
		cache.Set(&HighVolumeEventCacheEntry{
			ID:        "AUD-" + string(rune('0'+i)),
			Kind:      "audit_event",
			CreatedAt: baseTime.Add(time.Duration(i-3) * time.Hour), // -2h to +2h
			Exists:    true,
		})
	}

	// Count in window: -1h to +1h (should be 3 entries: -1h, 0h, +1h)
	startTime := baseTime.Add(-1 * time.Hour)
	endTime := baseTime.Add(1 * time.Hour)
	count := cache.CountByTimeWindow(startTime, endTime)
	if count != 3 {
		t.Errorf("CountByTimeWindow = %d, want 3", count)
	}
}

func TestHighVolumeEventCache_InvalidateNonExistent(t *testing.T) {
	cache := NewHighVolumeEventCache()

	// Invalidate non-existent entry (should not panic)
	cache.Invalidate("NONEXISTENT")

	// Add entry
	cache.Set(&HighVolumeEventCacheEntry{
		ID:        "AUD-001",
		Kind:      "audit_event",
		CreatedAt: time.Now().UTC(),
		Exists:    true,
	})

	// Invalidate it
	cache.Invalidate("AUD-001")

	// Verify it's gone
	_, exists := cache.Get("AUD-001")
	if exists {
		t.Error("entry should be invalidated")
	}
}

func TestHighVolumeEventCache_LoadInvalidFile(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := tempDir

	cache := NewHighVolumeEventCache()

	// Create invalid cache file
	cachePath := filepath.Join(projectRoot, paths.ProjectDataDir, "cache", highVolumeEventCacheFile)
	if err := fileutil.MkdirAll(filepath.Dir(cachePath), paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := fileutil.WriteFile(cachePath, []byte("invalid json"), paths.FilePerm600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Load should return false (cache invalid)
	loaded, err := cache.LoadCache(projectRoot)
	if err != nil {
		t.Fatalf("LoadCache should not return error for invalid file: %v", err)
	}
	if loaded {
		t.Error("LoadCache should return false for invalid cache file")
	}
}

// TestHighVolumeEventCache_LoadV1Format ensures v1 (flat entries) cache files still load and re-save as v2.
func TestHighVolumeEventCache_LoadV1Format(t *testing.T) {
	tempDir := t.TempDir()
	projectRoot := tempDir

	// Write v1-style cache (flat entries)
	cachePath := filepath.Join(projectRoot, paths.ProjectDataDir, "cache", highVolumeEventCacheFile)
	if err := fileutil.MkdirAll(filepath.Dir(cachePath), paths.DirPerm755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	v1 := struct {
		Metadata *HighVolumeEventCacheMetadata         `json:"metadata"`
		Entries  map[string]*HighVolumeEventCacheEntry `json:"entries"`
	}{
		Metadata: &HighVolumeEventCacheMetadata{
			Version:     "1.0",
			BuildTime:   time.Now().UTC(),
			ProjectRoot: projectRoot,
			EntryCount:  1,
		},
		Entries: map[string]*HighVolumeEventCacheEntry{
			"AUD-V1": {
				ID:        "AUD-V1",
				Kind:      "audit_event",
				CreatedAt: time.Now().UTC(),
				EventType: "object_creation",
				FilePath:  "/audit/AUD-V1.yaml",
				MTime:     time.Now().UTC(),
				Exists:    true,
			},
		},
	}
	data, err := json.Marshal(v1)
	if err != nil {
		t.Fatalf("Marshal v1: %v", err)
	}
	if err := fileutil.WriteFile(cachePath, data, paths.FilePerm600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cache := NewHighVolumeEventCache()
	loaded, err := cache.LoadCache(projectRoot)
	if err != nil {
		t.Fatalf("LoadCache: %v", err)
	}
	if !loaded {
		t.Fatal("LoadCache returned false for valid v1 file")
	}
	entry, ok := cache.Get("AUD-V1")
	if !ok {
		t.Fatal("Get(AUD-V1) after load v1: missing")
	}
	entry, ok = nildecode.DecodeNonNilPayload[*HighVolumeEventCacheEntry](entry)
	if !ok {
		t.Fatal("Get(AUD-V1) after load v1: missing")
	}
	if entry.Kind != "audit_event" || entry.EventType != "object_creation" {
		t.Errorf("entry: kind=%q event_type=%q", entry.Kind, entry.EventType)
	}
	if cache.Count() != 1 {
		t.Errorf("Count() = %d, want 1", cache.Count())
	}

	// Re-save should write v2 (bucketed); next load should still work
	if err := cache.SaveCache(projectRoot); err != nil {
		t.Fatalf("SaveCache: %v", err)
	}
	cache2 := NewHighVolumeEventCache()
	loaded2, err := cache2.LoadCache(projectRoot)
	if err != nil || !loaded2 {
		t.Fatalf("LoadCache after re-save: err=%v loaded=%v", err, loaded2)
	}
	if _, ok := cache2.Get("AUD-V1"); !ok {
		t.Error("Get(AUD-V1) after v2 load: missing")
	}
}

// slowListStorage is a test-only ObjectStorageProvider that blocks in List for a delay then returns empty.
// Used to prove BuildCache does not hold the cache lock during heavy work (only during swap).
type slowListStorage struct {
	delay time.Duration
}

func (s *slowListStorage) List(ctx context.Context, secCtx *pkgctx.SecurityContext, storageCtx *pkgctx.StorageContext, filter ListFilter) (*QueryResult, error) {
	time.Sleep(s.delay)
	return &QueryResult{Objects: []map[string]any{}}, nil
}

func (s *slowListStorage) Create(context.Context, *pkgctx.SecurityContext, map[string]any) error {
	panic("not used")
}
func (s *slowListStorage) Read(context.Context, *pkgctx.SecurityContext, string) (map[string]any, error) {
	panic("not used")
}
func (s *slowListStorage) Update(context.Context, *pkgctx.SecurityContext, string, map[string]any) error {
	panic("not used")
}
func (s *slowListStorage) Delete(context.Context, *pkgctx.SecurityContext, string, bool) error {
	panic("not used")
}
func (s *slowListStorage) Query(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, Query) (*QueryResult, error) {
	panic("not used")
}
func (s *slowListStorage) Search(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, SearchQuery) (*SearchResult, error) {
	panic("not used")
}
func (s *slowListStorage) BeginTransaction(context.Context) (ObjectTransaction, error) {
	panic("not used")
}
func (s *slowListStorage) BulkCreate(context.Context, *pkgctx.SecurityContext, []map[string]any) (*BulkResult, error) {
	panic("not used")
}
func (s *slowListStorage) BulkUpdate(context.Context, *pkgctx.SecurityContext, []BulkUpdateItem) (*BulkResult, error) {
	panic("not used")
}
func (s *slowListStorage) BulkGet(context.Context, *pkgctx.SecurityContext, []string) (*BulkResult, error) {
	panic("not used")
}
func (s *slowListStorage) BulkDelete(context.Context, *pkgctx.SecurityContext, []string, bool) (*BulkResult, error) {
	panic("not used")
}
func (s *slowListStorage) Exists(context.Context, *pkgctx.SecurityContext, string) (bool, error) {
	panic("not used")
}
func (s *slowListStorage) Count(context.Context, *pkgctx.SecurityContext, ListFilter) (int, error) {
	panic("not used")
}
func (s *slowListStorage) Aggregate(context.Context, *pkgctx.SecurityContext, *pkgctx.StorageContext, ListFilter, []Aggregation) (*AggregateResult, error) {
	panic("not used")
}
func (s *slowListStorage) GetRelated(context.Context, *pkgctx.SecurityContext, string, string, int) ([]map[string]any, error) {
	panic("not used")
}
func (s *slowListStorage) GetPath(context.Context, *pkgctx.SecurityContext, string, string) ([]map[string]any, error) {
	panic("not used")
}
func (s *slowListStorage) GetNeighbors(context.Context, *pkgctx.SecurityContext, string, string) ([]map[string]any, error) {
	panic("not used")
}
func (s *slowListStorage) Move(context.Context, *pkgctx.SecurityContext, string, string, bool) error {
	panic("not used")
}

func (s *slowListStorage) Rename(context.Context, *pkgctx.SecurityContext, string, string, bool) error {
	panic("not used")
}

// TestBuildCache_ConcurrentReadersDoNotBlock proves that during BuildCache the lock is only held for the swap.
// While the background build is "in progress" (slow List), concurrent IsPopulatedForProject calls complete quickly.
func TestBuildCache_ConcurrentReadersDoNotBlock(t *testing.T) {
	projectRoot := t.TempDir()
	cache := NewHighVolumeEventCache()
	// List blocks for 300ms so the build goroutine is stuck in buildCacheData (no lock held)
	slow := &slowListStorage{delay: 300 * time.Millisecond}
	ctx := context.Background()

	buildDone := make(chan error, 1)
	goroutinelabels.NewGoroutine("storage_test", "background build cache").StartSimple(func() {
		buildDone <- cache.BuildCache(ctx, projectRoot, slow)
	})

	// Give BuildCache time to enter buildCacheData and call List (which will block 300ms)
	time.Sleep(20 * time.Millisecond)

	// Concurrent readers should complete quickly (lock not held during List/build)
	readerDone := make(chan struct{}, 1)
	start := time.Now()
	goroutinelabels.NewGoroutine("storage_test", "concurrent cache readers").StartSimple(func() {
		for i := 0; i < 10; i++ {
			_ = cache.IsPopulatedForProject(projectRoot)
		}
		readerDone <- struct{}{}
	})
	var waitErr error
	pl := pipeline.NewBuilder("storage.high_volume_cache_reader_wait", nil).
		AddStage(pipeline.StageIngest, func(_ *pipeline.Context, payload any) (any, error) {
			select {
			case <-readerDone:
			case <-time.After(500 * time.Millisecond):
				waitErr = context.DeadlineExceeded
			}
			return payload, nil
		}).
		Build()
	_, _ = pl.Run(&pipeline.Context{Ctx: context.Background(), Outcome: make(map[string]any)}, nil)
	if waitErr != nil {
		t.Fatal("IsPopulatedForProject blocked too long; cache lock may be held during heavy work")
	}
	elapsed := time.Since(start)
	if elapsed > 200*time.Millisecond {
		t.Errorf("10x IsPopulatedForProject took %v; lock should not be held during build (expected <200ms)", elapsed)
	}

	<-buildDone // wait for build to finish
}
func (s *slowListStorage) Shutdown(ctx context.Context) error { return nil }
