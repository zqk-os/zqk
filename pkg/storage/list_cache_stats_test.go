package storage

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestListCache_LifetimeCounters(t *testing.T) {
	hitsInit, missesInit, invalidationsInit := GetListCacheStats()

	// 1. Force a miss
	filter := &ListFilter{Kind: "backlog_item"}
	_, found := GetListCache("/tmp/test_project", filter, 100)
	if found {
		t.Fatalf("expected cache miss, found entry")
	}

	_, missesAfterMiss, _ := GetListCacheStats()
	if missesAfterMiss <= missesInit {
		t.Fatalf("expected misses counter to increment, got init=%d after=%d", missesInit, missesAfterMiss)
	}

	// 2. Set cache entry and force a hit
	res := &QueryResult{
		Objects: []map[string]any{{objects.FieldKeyID: "BLI-123"}},
	}
	SetListCache("/tmp/test_project", filter, 100, res)

	_, foundHit := GetListCache("/tmp/test_project", filter, 100)
	if !foundHit {
		t.Fatalf("expected cache hit, got miss")
	}

	hitsAfterHit, _, _ := GetListCacheStats()
	if hitsAfterHit <= hitsInit {
		t.Fatalf("expected hits counter to increment, got init=%d after=%d", hitsInit, hitsAfterHit)
	}

	// 3. Invalidate cache
	InvalidateListCache()

	_, _, invalidationsAfter := GetListCacheStats()
	if invalidationsAfter <= invalidationsInit {
		t.Fatalf("expected invalidations counter to increment, got init=%d after=%d", invalidationsInit, invalidationsAfter)
	}
}
