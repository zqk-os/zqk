package id_generation

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// GeneratorPool manages a shared pool of ID generators for low-volume objects
// High-volume objects get dedicated generators, low-volume objects share from the pool
type GeneratorPool struct {
	mu                  sync.RWMutex
	pool                []*BatchIDGenerator          // Shared pool of generators
	poolSize            int                          // Current pool size
	maxPoolSize         int                          // Maximum pool size
	volumeTracker       *VolumeTracker               // Tracks generation volume per kind
	dedicatedGens       map[string]*BatchIDGenerator // Dedicated generators for high-volume kinds
	highVolumeThreshold int64                        // Threshold for high-volume (IDs per period)
}

// VolumeTracker tracks ID generation volume per kind+directory+prefix
type VolumeTracker struct {
	mu         sync.RWMutex
	volumes    map[string]*VolumeStats // Key: kindDir:kind:prefix
	periodSize int64                   // Period size in IDs (for volume calculation)
}

// VolumeStats tracks generation statistics for a kind
type VolumeStats struct {
	TotalGenerated atomic.Int64 // Total IDs generated
	LastReset      int64        // Timestamp of last reset
}

var (
	globalPool     *GeneratorPool
	globalPoolOnce sync.Once
)

// GetGlobalGeneratorPool returns the global generator pool (singleton)
// Pool configuration:
//   - initialSize: 10 generators (starts small, grows as needed)
//   - maxSize: 50 generators (prevents unbounded growth)
//   - highVolumeThreshold: 1000 IDs per period (kinds generating >= 1000 IDs get dedicated generators)
//
// Algorithm: Pool size grows dynamically as low-volume kinds are discovered, up to maxSize.
// High-volume kinds (>= threshold) get dedicated generators and don't count toward pool size.
func GetGlobalGeneratorPool() *GeneratorPool {
	globalPoolOnce.Do(func() {
		// Default configuration: 10 initial, 50 max, 1000 threshold
		// These can be tuned based on observed object counts and generation patterns
		globalPool = NewGeneratorPool(10, 50, 1000)
	})
	return globalPool
}

// NewGeneratorPool creates a new generator pool
// initialSize: Initial pool size
// maxSize: Maximum pool size (will grow up to this)
// highVolumeThreshold: IDs per period to be considered high-volume (gets dedicated generator)
func NewGeneratorPool(initialSize, maxSize int, highVolumeThreshold int64) *GeneratorPool {
	return &GeneratorPool{
		pool:                make([]*BatchIDGenerator, 0, initialSize),
		poolSize:            0,
		maxPoolSize:         maxSize,
		volumeTracker:       NewVolumeTracker(),
		dedicatedGens:       make(map[string]*BatchIDGenerator),
		highVolumeThreshold: highVolumeThreshold,
	}
}

// NewVolumeTracker creates a new volume tracker
func NewVolumeTracker() *VolumeTracker {
	return &VolumeTracker{
		volumes:    make(map[string]*VolumeStats),
		periodSize: 10000, // Reset stats every 10k IDs
	}
}

// GetOrCreateGenerator gets a generator for a kind, allocating from pool or creating dedicated
// Returns the generator and whether it's dedicated (true) or from pool (false)
//
// Allocation Strategy:
// - High-volume (>= threshold): Gets dedicated generator with queue (reduces contention)
// - Low-volume (< threshold): Gets generator from pool (shared pool, memory-efficient)
//
// Each kind+directory+prefix combination gets its own generator instance to ensure
// unique ID sequences. The pool manages lifecycle and memory for low-volume objects.
// High-volume generators use pre-filled queues to eliminate contention.
func (p *GeneratorPool) GetOrCreateGenerator(ctx context.Context, kindDir, kind, prefix string, minDigits, startAt int) (*BatchIDGenerator, bool) {
	return p.GetOrCreateGeneratorWithCAS(ctx, kindDir, kind, prefix, minDigits, startAt, nil)
}

// GetOrCreateGeneratorWithCAS gets a generator with optional CAS support
func (p *GeneratorPool) GetOrCreateGeneratorWithCAS(ctx context.Context, kindDir, kind, prefix string, minDigits, startAt int, casProvider CASIDProvider) (*BatchIDGenerator, bool) {
	key := fmt.Sprintf("%s:%s:%s", kindDir, kind, prefix)

	// Check if we already have a dedicated generator for this kind
	var gen *BatchIDGenerator
	var exists bool
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameGeneratorPoolGetDedicatedCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			gen, ok = p.dedicatedGens[key]
			exists = ok
			return nil
		},
	)

	if exists {
		return gen, true
	}

	// Check volume to determine if this should be dedicated
	volume := p.volumeTracker.GetVolume(key)
	isHighVolume := volume >= p.highVolumeThreshold

	if isHighVolume {
		// High-volume: create dedicated generator with queue (reduces contention)
		_ = concurrency.RunInLockOrLog(
			&p.mu, locknames.LockNameGeneratorPoolGetDedicatedCreate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				// Double-check after acquiring write lock
				if existingGen, ok := p.dedicatedGens[key]; ok {
					gen = existingGen
					exists = true
					return nil
				}
				// High-volume gets queue buffer (500 IDs pre-filled)
				// Background worker maintains queue size
				gen = GetBatchIDGeneratorWithBufferAndCAS(ctx, kindDir, kind, prefix, minDigits, startAt, 500, casProvider)
				p.dedicatedGens[key] = gen
				exists = true
				return nil
			},
		)
		if exists {
			return gen, true
		}
	}

	// Low-volume: try to get from pool or create new
	var poolGen *BatchIDGenerator
	var foundInPool bool
	var poolSize, maxPoolSize int
	_ = concurrency.RunInLockOrLog(
		&p.mu, locknames.LockNameGeneratorPoolGetFromPool, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Try to find existing generator in pool that matches this exact kind+directory+prefix
			// Each kind+directory+prefix gets its own generator to ensure unique sequences
			for _, existingGen := range p.pool {
				// Match must be exact: same kindDir, kind, prefix, minDigits, startAt
				// This ensures each kind has its own sequence counter
				if existingGen.matches(kindDir, kind, prefix, minDigits, startAt) {
					// Found matching generator in pool - reuse it
					poolGen = existingGen
					foundInPool = true
					return nil
				}
			}

			// No matching generator in pool - create new one for this kind
			// Low-volume uses direct generation (no queue) to save memory
			poolGen = NewBatchIDGeneratorWithCAS(ctx, kindDir, kind, prefix, minDigits, startAt, casProvider)

			// Add to pool if we have room (grow pool dynamically up to max)
			// This allows the pool to grow as more low-volume kinds are discovered
			poolSize = p.poolSize
			maxPoolSize = p.maxPoolSize
			if poolSize < maxPoolSize {
				p.pool = append(p.pool, poolGen)
				p.poolSize++
			}
			// If pool is full, return generator anyway (it just won't be pooled)
			// This prevents blocking when pool is exhausted
			return nil
		},
	)

	if foundInPool {
		return poolGen, false
	}
	return poolGen, false
}

// RecordGeneration records that an ID was generated for a kind (for volume tracking)
// This is called automatically by BatchIDGenerator.GenerateBatchIDs()
func (p *GeneratorPool) RecordGeneration(kindDir, kind, prefix string, count int) {
	key := fmt.Sprintf("%s:%s:%s", kindDir, kind, prefix)
	p.volumeTracker.Record(key, int64(count))
}

// matches checks if a generator matches the given parameters
// This allows reusing generators with the same configuration for low-volume objects
// Must be called with generator's lock held (caller should check without lock first)
func (g *BatchIDGenerator) matches(kindDir, kind, prefix string, minDigits, startAt int) bool {
	// No lock needed - we're checking immutable fields
	// The caller should ensure thread-safety at the pool level
	return g.kindDir == kindDir && g.kind == kind && g.prefix == prefix &&
		g.minDigits == minDigits && g.startAt == startAt
}

// GetVolume returns the current volume for a key
func (vt *VolumeTracker) GetVolume(key string) int64 {
	var stats *VolumeStats
	var exists bool
	_ = concurrency.RunInRLockOrLog(
		&vt.mu, locknames.LockNameVolumeTrackerGetVolume, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			stats, ok = vt.volumes[key]
			exists = ok
			return nil
		},
	)

	if !exists {
		return 0
	}
	return stats.TotalGenerated.Load()
}

// Record records ID generation for a key
func (vt *VolumeTracker) Record(key string, count int64) {
	_ = concurrency.RunInLockOrLog(
		&vt.mu, locknames.LockNameVolumeTrackerRecord, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			stats, exists := vt.volumes[key]
			if !exists {
				stats = &VolumeStats{
					// TotalGenerated starts at 0 (default for atomic.Int64)
					LastReset: 0,
				}
				vt.volumes[key] = stats
			}

			// Increment counter
			newTotal := stats.TotalGenerated.Add(count)

			// Reset if we've exceeded period size (prevents unbounded growth)
			if newTotal >= vt.periodSize {
				stats.TotalGenerated.Store(newTotal % vt.periodSize)
				stats.LastReset = newTotal
			}
			return nil
		},
	)
}

// GetPoolStats returns statistics about the pool (for monitoring/debugging)
func (p *GeneratorPool) GetPoolStats() PoolStats {
	var stats PoolStats
	_ = concurrency.RunInRLockOrLog(
		&p.mu, locknames.LockNameGeneratorPoolGetStats, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			stats = PoolStats{
				PoolSize:            p.poolSize,
				MaxPoolSize:         p.maxPoolSize,
				DedicatedCount:      len(p.dedicatedGens),
				HighVolumeThreshold: p.highVolumeThreshold,
			}
			return nil
		},
	)
	return stats
}

// PoolStats contains statistics about the generator pool
type PoolStats struct {
	PoolSize            int
	MaxPoolSize         int
	DedicatedCount      int
	HighVolumeThreshold int64
}

// CalculateOptimalPoolSize calculates optimal pool size based on object counts
// Algorithm: Estimate low-volume kinds (those below threshold) and size pool accordingly
// Formula: poolSize = min(maxLowVolumeKinds, maxPoolSize)
// where maxLowVolumeKinds is estimated from total kinds - high-volume kinds
func CalculateOptimalPoolSize(totalKinds int, estimatedHighVolumeKinds int, maxPoolSize int) int {
	estimatedLowVolumeKinds := totalKinds - estimatedHighVolumeKinds
	if estimatedLowVolumeKinds < 0 {
		estimatedLowVolumeKinds = 0
	}

	// Pool should be sized for low-volume kinds, but not exceed max
	optimalSize := estimatedLowVolumeKinds
	if optimalSize > maxPoolSize {
		optimalSize = maxPoolSize
	}

	// Ensure minimum pool size for efficiency
	minSize := 10
	if optimalSize < minSize {
		optimalSize = minSize
	}

	return optimalSize
}
