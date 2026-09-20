package file

import (
	"path/filepath"
	"testing"
	"time"
)

func TestShardedFileLockStrategy_DeterministicSharding(t *testing.T) {
	tempDir := t.TempDir()
	shardCount := 16
	strategy := NewShardedFileLockStrategy(shardCount, tempDir)

	key1 := "BLI-CACHE-001"
	key2 := "BLI-CACHE-002"

	shard1a := strategy.ShardForKey(key1)
	shard1b := strategy.ShardForKey(key1)
	if shard1a != shard1b {
		t.Fatalf("expected deterministic shard for key %s, got %d and %d", key1, shard1a, shard1b)
	}

	if shard1a < 0 || shard1a >= shardCount {
		t.Fatalf("shard index out of bounds: %d (expected 0..%d)", shard1a, shardCount-1)
	}

	shard2 := strategy.ShardForKey(key2)
	if shard2 < 0 || shard2 >= shardCount {
		t.Fatalf("shard index out of bounds: %d", shard2)
	}

	path1 := strategy.LockPathForKey(key1)
	expectedPrefix := filepath.Join(tempDir, "shard_")
	if len(path1) == 0 || filepath.Dir(path1) != tempDir {
		t.Fatalf("unexpected lock path: %s (expected dir: %s)", path1, expectedPrefix)
	}
}

func TestShardedFileLockStrategy_AcquireAndRelease(t *testing.T) {
	tempDir := t.TempDir()
	strategy := NewShardedFileLockStrategy(8, tempDir)

	resourceKey := "PRI-TEST-001"
	handle, err := strategy.AcquireShardLock(resourceKey, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("failed to acquire shard lock: %v", err)
	}

	if !handle.IsLocked() {
		t.Fatalf("expected handle to be locked")
	}

	if err := handle.Release(); err != nil {
		t.Fatalf("failed to release shard lock: %v", err)
	}

	if handle.IsLocked() {
		t.Fatalf("expected handle to be released")
	}
}

// TestShardedFileLock_BLI_STORAGE_SHARDED_LOCK_001 verifies granular sharded locking for CAS paths per BLI-STORAGE-SHARDED-LOCK-001.
func TestShardedFileLock_BLI_STORAGE_SHARDED_LOCK_001(t *testing.T) {
	tempDir := t.TempDir()
	strategy := NewShardedFileLockStrategy(16, tempDir)

	casKey := "CAS-SHARD-EVIDENCE-001"
	handle, err := strategy.AcquireShardLock(casKey, 1*time.Second)
	if err != nil {
		t.Fatalf("expected shard lock acquisition for BLI-STORAGE-SHARDED-LOCK-001: %v", err)
	}
	defer handle.Release()

	if !handle.IsLocked() {
		t.Fatalf("expected handle to be actively locked")
	}
}
