package storage

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestListCache_GetMiss(t *testing.T) {
	InvalidateListCache()
	defer InvalidateListCache()

	filter := &ListFilter{Kind: "backlog_item", Limit: 10}
	_, ok := GetListCache("/project", filter, 10)
	if ok {
		t.Fatal("expected cache miss before any Set")
	}

	// nil/empty inputs
	_, ok = GetListCache("", filter, 10)
	if ok {
		t.Fatal("expected miss for empty projectRoot")
	}
	_, ok = GetListCache("/project", nil, 10)
	if ok {
		t.Fatal("expected miss for nil filter")
	}
}

func TestListCache_SetAndGet(t *testing.T) {
	InvalidateListCache()
	defer InvalidateListCache()

	// Unique root per test: global list cache is shared; parallel tests must not share keys or they evict/race.
	projectRoot := filepath.Join(t.TempDir(), "list-cache-set-get")
	filter := &ListFilter{Kind: "backlog_item", Limit: 10, SortBy: "id"}
	effectiveLimit := 10
	result := &QueryResult{
		Objects: []map[string]any{{objects.FieldKeyID: "ITEM-1", objects.FieldKeyTitle: "First"}},
		Groups:  make(map[string][]map[string]any),
		Meta:    map[string]any{"total_count": 1},
	}

	SetListCache(projectRoot, filter, effectiveLimit, result)
	cached, ok := GetListCache(projectRoot, filter, effectiveLimit)
	if !ok {
		t.Fatal("expected cache hit after Set")
	}
	if len(cached.Objects) != 1 || cached.Objects[0][objects.FieldKeyID] != "ITEM-1" {
		t.Errorf("cached result mismatch: got %v", cached.Objects)
	}

	// Returned value is a copy: mutating it must not affect the cache
	cached.Objects[0][objects.FieldKeyTitle] = "Mutated"
	cached2, ok2 := GetListCache(projectRoot, filter, effectiveLimit)
	if !ok2 || cached2 == nil || len(cached2.Objects) < 1 {
		t.Fatalf("expected second cache hit with non-empty Objects, ok=%v len=%d", ok2, len(cached2.Objects))
	}
	if cached2.Objects[0][objects.FieldKeyTitle] == "Mutated" {
		t.Error("cache should return a copy; mutation should not be visible")
	}
}

func TestListCache_KeyIncludesEffectiveLimit(t *testing.T) {
	InvalidateListCache()
	defer InvalidateListCache()

	projectRoot := filepath.Join(t.TempDir(), "list-cache-effective-limit")
	filter := &ListFilter{Kind: "backlog_item", Limit: 100}
	result := &QueryResult{Objects: []map[string]any{}, Groups: map[string][]map[string]any{}, Meta: map[string]any{}}

	SetListCache(projectRoot, filter, 20, result)
	_, ok20 := GetListCache(projectRoot, filter, 20)
	_, ok50 := GetListCache(projectRoot, filter, 50)
	if !ok20 {
		t.Fatal("expected hit for effectiveLimit 20")
	}
	if ok50 {
		t.Fatal("expected miss for effectiveLimit 50 (different key)")
	}
}

func TestListCache_InvalidateAll(t *testing.T) {
	InvalidateListCache()
	defer InvalidateListCache()

	projectRoot := filepath.Join(t.TempDir(), "list-cache-invalidate-all")
	filter := &ListFilter{Kind: "backlog_item", Limit: 10}
	result := &QueryResult{Objects: []map[string]any{}, Groups: map[string][]map[string]any{}, Meta: map[string]any{}}
	SetListCache(projectRoot, filter, 10, result)

	_, ok := GetListCache(projectRoot, filter, 10)
	if !ok {
		t.Fatal("expected hit before InvalidateListCache")
	}

	InvalidateListCache()
	_, ok = GetListCache(projectRoot, filter, 10)
	if ok {
		t.Fatal("expected miss after InvalidateListCache")
	}
}

func TestListCache_InvalidateKind(t *testing.T) {
	InvalidateListCache()
	defer InvalidateListCache()

	projectRoot := filepath.Join(t.TempDir(), "list-cache-invalidate-kind")
	result := &QueryResult{Objects: []map[string]any{}, Groups: map[string][]map[string]any{}, Meta: map[string]any{}}

	SetListCache(projectRoot, &ListFilter{Kind: "backlog_item", Limit: 10}, 10, result)
	SetListCache(projectRoot, &ListFilter{Kind: "milestone", Limit: 10}, 10, result)

	InvalidateListCacheForKind("backlog_item")

	_, okBLI := GetListCache(projectRoot, &ListFilter{Kind: "backlog_item", Limit: 10}, 10)
	_, okMIL := GetListCache(projectRoot, &ListFilter{Kind: "milestone", Limit: 10}, 10)
	if okBLI {
		t.Fatal("expected miss for backlog_item after InvalidateListCacheForKind(backlog_item)")
	}
	if !okMIL {
		t.Fatal("expected hit for milestone after InvalidateListCacheForKind(backlog_item)")
	}
}

func TestListCache_DeterministicKey(t *testing.T) {
	InvalidateListCache()
	defer InvalidateListCache()

	// Same filter (with map) should produce same key so second Set overwrites and Get returns same
	filter := &ListFilter{
		Kind:    "backlog_item",
		Filters: map[string]any{objects.FieldKeyStatus: "planned", objects.FieldKeyPriority: map[string]any{"$gt": float64(1)}},
		Limit:   10,
	}
	result1 := &QueryResult{Objects: []map[string]any{{objects.FieldKeyID: "A"}}, Groups: map[string][]map[string]any{}, Meta: map[string]any{}}
	result2 := &QueryResult{Objects: []map[string]any{{objects.FieldKeyID: "B"}}, Groups: map[string][]map[string]any{}, Meta: map[string]any{}}

	p := filepath.Join(t.TempDir(), "list-cache-deterministic")
	SetListCache(p, filter, 10, result1)
	SetListCache(p, filter, 10, result2)

	cached, ok := GetListCache(p, filter, 10)
	if !ok {
		t.Fatal("expected hit")
	}
	if len(cached.Objects) != 1 || cached.Objects[0][objects.FieldKeyID] != "B" {
		t.Errorf("expected last Set to win: got %v", cached.Objects)
	}
}
