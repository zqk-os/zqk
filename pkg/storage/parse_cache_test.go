package storage

import (
	"context"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

// TestParseCacheEvictionShockwave satisfies TST-1789165612728794000-93e85c38.
// Asserts that when an ObjectMutationEvent is dispatched through the InvalidationShockwaveBus,
// the in-memory ParseCache immediately purges the entry for OldHash.
func TestParseCacheEvictionShockwave(t *testing.T) {
	parseCache := GetGlobalParseCache()
	bus := GetGlobalInvalidationBus()

	oldHash := "1111222233334444555566667777888899990000aaaabbbbccccddddeeeeffff"
	newHash := "ffffddddccccbbbbaaaa00009999888877776666555544443333222211110000"

	parsed := &objects.ParsedObject{
		Raw: map[string]any{
			objects.FieldKeyID:   "BLI-TEST-AST-EVICT",
			objects.FieldKeyKind: "backlog_item",
		},
	}

	parseCache.Put(oldHash, parsed)

	// Verify hit before shockwave
	if retrieved, ok := parseCache.Get(oldHash); !ok || retrieved == nil {
		t.Fatalf("expected cache hit for %s before shockwave broadcast", oldHash)
	}

	// Dispatch ObjectMutationEvent across the InvalidationShockwaveBus
	event := ObjectMutationEvent{
		Op:      MutationOpPut,
		Kind:    "backlog_item",
		ID:      "BLI-TEST-AST-EVICT",
		OldHash: oldHash,
		NewHash: newHash,
	}
	bus.Broadcast(context.Background(), event)

	// Assert immediate cache miss for OldHash
	if retrieved, ok := parseCache.Get(oldHash); ok || retrieved != nil {
		t.Fatalf("expected cache miss for %s after shockwave broadcast, got %+v", oldHash, retrieved)
	}
}
