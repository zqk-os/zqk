package community

import (
	"fmt"
	"testing"
)

func TestConsistentShardRing_EmptyRing(t *testing.T) {
	ring := NewConsistentShardRing(20)
	_, err := ring.GetShard("key-1")
	if err != ErrNoShardsAvailable {
		t.Fatalf("expected ErrNoShardsAvailable, got %v", err)
	}
}

func TestConsistentShardRing_DeterministicRouting(t *testing.T) {
	ring := NewConsistentShardRing(30)
	if err := ring.AddShard("shard-a", 1); err != nil {
		t.Fatalf("AddShard failed: %v", err)
	}
	if err := ring.AddShard("shard-b", 1); err != nil {
		t.Fatalf("AddShard failed: %v", err)
	}
	if err := ring.AddShard("shard-c", 1); err != nil {
		t.Fatalf("AddShard failed: %v", err)
	}

	shard1, err1 := ring.GetShard("partition-alpha")
	shard2, err2 := ring.GetShard("partition-alpha")

	if err1 != nil || err2 != nil {
		t.Fatalf("unexpected error: %v, %v", err1, err2)
	}
	if shard1 != shard2 {
		t.Fatalf("expected deterministic shard assignment, got %s and %s", shard1, shard2)
	}
}

func TestConsistentShardRing_AddRemoveShard(t *testing.T) {
	ring := NewConsistentShardRing(25)
	if err := ring.AddShard("shard-1", 1); err != nil {
		t.Fatalf("AddShard failed: %v", err)
	}
	if err := ring.AddShard("shard-2", 1); err != nil {
		t.Fatalf("AddShard failed: %v", err)
	}

	if ring.ShardCount() != 2 {
		t.Fatalf("expected 2 shards, got %d", ring.ShardCount())
	}

	err := ring.RemoveShard("shard-1")
	if err != nil {
		t.Fatalf("failed to remove shard: %v", err)
	}

	if ring.ShardCount() != 1 {
		t.Fatalf("expected 1 shard, got %d", ring.ShardCount())
	}

	// All queries must now resolve to shard-2
	shard, err := ring.GetShard("arbitrary-key")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if shard != "shard-2" {
		t.Fatalf("expected 'shard-2', got %s", shard)
	}
}

func TestConsistentShardRing_DistributionFairness(t *testing.T) {
	ring := NewConsistentShardRing(50)
	shards := []string{"shard-1", "shard-2", "shard-3", "shard-4"}
	for _, s := range shards {
		if err := ring.AddShard(s, 1); err != nil {
			t.Fatalf("AddShard failed: %v", err)
		}
	}

	counts := make(map[string]int)
	totalKeys := 1000
	for i := 0; i < totalKeys; i++ {
		key := fmt.Sprintf("test-key-%d", i)
		sh, err := ring.GetShard(key)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[sh]++
	}

	// Ensure every shard received at least 15% of keys (fair distribution)
	minExpected := int(float64(totalKeys) * 0.15)
	for _, s := range shards {
		if counts[s] < minExpected {
			t.Fatalf("shard %s under-allocated: %d < %d (counts: %v)", s, counts[s], minExpected, counts)
		}
	}
}
