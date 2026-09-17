package file

import (
	"fmt"
	"hash/fnv"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	defaultShardCount = 16
)

// ShardedFileLockStrategy partitions file locks across N deterministic shards
// to eliminate coarse write-lock bottlenecks during high-contention operations.
type ShardedFileLockStrategy struct {
	shardCount    int
	lockDir       string
	innerStrategy FileLockStrategy
}

// NewShardedFileLockStrategy creates an initialized ShardedFileLockStrategy.
func NewShardedFileLockStrategy(shardCount int, lockDir string) *ShardedFileLockStrategy {
	if shardCount <= 0 {
		shardCount = defaultShardCount
	}
	if lockDir == "" {
		lockDir = filepath.Join(".", ".zqk", "locks", "shards")
	}

	return &ShardedFileLockStrategy{
		shardCount:    shardCount,
		lockDir:       lockDir,
		innerStrategy: NewAutoCleanupStrategy(),
	}
}

// Name returns the identifier for this strategy.
func (s *ShardedFileLockStrategy) Name() string {
	return "sharded-file-lock"
}

// ShardForKey calculates a deterministic shard index [0..shardCount-1] using FNV-1a hash.
func (s *ShardedFileLockStrategy) ShardForKey(key string) int {
	if s.shardCount <= 0 {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	hashVal := h.Sum32()
	sc := uint32(s.shardCount & 0x7FFFFFFF)
	if sc == 0 {
		return 0
	}
	return int(hashVal % sc)
}

// LockPathForKey returns the absolute or relative lock path for the given resource key.
func (s *ShardedFileLockStrategy) LockPathForKey(key string) string {
	shard := s.ShardForKey(key)
	return filepath.Join(s.lockDir, fmt.Sprintf("shard_%03d.lock", shard))
}

// AcquireShardLock acquires a lock on the shard corresponding to the given resource key.
func (s *ShardedFileLockStrategy) AcquireShardLock(key string, timeout time.Duration) (LockHandle, error) {
	// Ensure lock directory exists
	if err := fileutil.MkdirAll(s.lockDir, 0755); err != nil {
		return nil, errfmt.Newf(ConstMiscFailedToAcquireLock).Wrap(err)
	}

	lockPath := s.LockPathForKey(key)
	return s.innerStrategy.AcquireLock(lockPath, timeout)
}

// AcquireLock implements FileLockStrategy by acquiring the lock directly at lockPath.
func (s *ShardedFileLockStrategy) AcquireLock(lockPath string, timeout time.Duration) (LockHandle, error) {
	return s.innerStrategy.AcquireLock(lockPath, timeout)
}

// CleanupStaleLocks cleans up stale lock files at lockPath.
func (s *ShardedFileLockStrategy) CleanupStaleLocks(lockPath string) error {
	return s.innerStrategy.CleanupStaleLocks(lockPath)
}

// WithShardedFileLock executes fn while holding the shard lock for the specified resource key.
func WithShardedFileLock(strategy *ShardedFileLockStrategy, key string, timeout time.Duration, fn func() error) error {
	if strategy == nil {
		return fn()
	}

	handle, err := strategy.AcquireShardLock(key, timeout)
	if err != nil {
		return err
	}
	defer func() {
		_ = handle.Release()
	}()

	return fn()
}
