package storage

import (
	"context"
	"testing"
)

func TestCacheManager_LifetimeCounters(t *testing.T) {
	t.Parallel()

	var nilCm *CacheManager
	trNil, coNil, faNil := nilCm.GetCacheManagerStats()
	if trNil != 0 || coNil != 0 || faNil != 0 {
		t.Fatalf("expected nil stats (0, 0, 0), got (%d, %d, %d)", trNil, coNil, faNil)
	}

	ctx := context.Background()
	testStorage := &TestStorage{objects: make(map[string]map[string]any)}
	cm := NewCacheManager(ctx, testStorage)

	trInit, coInit, faInit := cm.GetCacheManagerStats()
	if trInit != 0 || coInit != 0 || faInit != 0 {
		t.Fatalf("expected initial stats (0, 0, 0), got (%d, %d, %d)", trInit, coInit, faInit)
	}

	cm.invalidationsTriggeredTotal.Add(1)
	cm.invalidationsCompletedTotal.Add(1)

	trAfter, coAfter, faAfter := cm.GetCacheManagerStats()
	if trAfter != 1 || coAfter != 1 || faAfter != 0 {
		t.Fatalf("expected after stats (1, 1, 0), got (%d, %d, %d)", trAfter, coAfter, faAfter)
	}
}
