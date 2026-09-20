package id_generation

import (
	"context"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage/locknames"
)

// DirectoryScanConfig holds configuration for directory scan operations
// This abstracts timeout and retry configuration to avoid duplication
// Matches RetryConfig pattern from pkg/storage/operation_helper.go
type DirectoryScanConfig struct {
	// ReadDirTimeout is the timeout for ReadDir operations
	// ReadDir can block indefinitely on large directories (e.g., 29k+ files)
	ReadDirTimeout time.Duration

	// Retry configuration (matches RetryConfig from operation_helper.go)
	RetryMaxAttempts   int
	RetryInitialDelay  time.Duration
	RetryMaxDelay      time.Duration
	RetryBackoffFactor float64

	// CacheTTL is how long directory scan results are cached
	CacheTTL time.Duration
}

// DefaultDirectoryScanConfig returns default configuration for directory scans
// Values match defaults from pkg/storage/operation_helper.go RetryConfig
func DefaultDirectoryScanConfig() *DirectoryScanConfig {
	return &DirectoryScanConfig{
		ReadDirTimeout:     5 * time.Second,
		RetryMaxAttempts:   3,
		RetryInitialDelay:  100 * time.Millisecond,
		RetryMaxDelay:      5 * time.Second,
		RetryBackoffFactor: 2.0,
		CacheTTL:           5 * time.Second,
	}
}

var (
	// Global registry of batch ID generators (one per kind+directory combination)
	batchGenerators     = make(map[string]*BatchIDGenerator)
	batchGeneratorsLock sync.RWMutex

	// Shared directory scan cache to avoid redundant scans across generators
	// Key: directory path, Value: (maxSequence, scanTime)
	dirScanCache     = make(map[string]*dirScanResult)
	dirScanCacheLock sync.RWMutex

	// Directory scan configuration (shared across all generators)
	// Can be modified at runtime if needed (e.g., for testing or tuning)
	dirScanConfig = DefaultDirectoryScanConfig()

	// Per-directory scan locks to prevent concurrent scans of the same directory
	dirScanLocks     = make(map[string]*sync.Mutex)
	dirScanLocksLock sync.Mutex
)

// dirScanResult holds cached directory scan results
type dirScanResult struct {
	maxSequence int
	scanTime    time.Time
	mu          sync.RWMutex
}

// getDirScanLock returns a mutex for a specific directory (to serialize scans)
func getDirScanLock(ctx context.Context, dirPath string) *sync.Mutex {
	var lock *sync.Mutex
	var exists bool
	_ = concurrency.RunInLockOrLog(
		&dirScanLocksLock, locknames.LockNameBatchGeneratorGetDirScanLock, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			lock, ok = dirScanLocks[dirPath]
			exists = ok
			if !exists {
				lock = &sync.Mutex{}
				dirScanLocks[dirPath] = lock
			}
			return nil
		},
	)
	return lock
}

// CASIDProvider is an optional function that can query CAS for existing IDs in a specific bucket
// Returns IDs that match the prefix pattern and are in the specified bucket directory
// This allows the batch generator to find IDs stored in CAS (hash-based filenames)
type CASIDProvider func(ctx context.Context, kind, bucketDir, prefix string) ([]string, error)

// BatchIDGenerator provides thread-safe batch ID generation for sequential IDs
// Uses in-memory state with periodic directory scans to minimize contention
// Can optionally use a pre-filled queue to eliminate contention entirely
// When sequenceFileDir is set, uses a cross-process sequence file (flock) for both single and batch allocation
type BatchIDGenerator struct {
	mu              sync.Mutex
	ctx             context.Context // Context for cancellation and timeout propagation
	kindDir         string
	kind            string
	prefix          string
	minDigits       int
	startAt         int
	lastSequence    int            // Last sequence number used (cached)
	initialized     bool           // Whether we've scanned the directory
	scanThreshold   int            // Rescan directory after this many IDs generated
	scanCount       int            // Counter for when to rescan
	seqPattern      *regexp.Regexp // Compiled regex pattern for this prefix
	lastScanTime    time.Time      // When directory was last scanned (for cache invalidation)
	queue           *IDQueue       // Optional: pre-filled queue (if bufferSize > 0)
	casProvider     CASIDProvider  // Optional: function to query CAS for existing IDs in bucket
	sequenceFileDir string         // Optional: when set, allocate from sequence file (cross-process safe)
}

// GetBatchIDGenerator returns (or creates) a thread-safe batch ID generator
// Uses pool-based allocation: high-volume objects get dedicated generators, low-volume share from pool
// If bufferSize > 0, returns a queue-backed generator (reduces contention)
func GetBatchIDGenerator(ctx context.Context, kindDir, kind, prefix string, minDigits, startAt int) *BatchIDGenerator {
	return GetBatchIDGeneratorWithCAS(ctx, kindDir, kind, prefix, minDigits, startAt, nil)
}

// GetBatchIDGeneratorWithCAS returns a batch ID generator with optional CAS support
// casProvider: optional function to query CAS for existing IDs in the bucket directory
func GetBatchIDGeneratorWithCAS(ctx context.Context, kindDir, kind, prefix string, minDigits, startAt int, casProvider CASIDProvider) *BatchIDGenerator {
	return GetBatchIDGeneratorWithBufferAndCAS(ctx, kindDir, kind, prefix, minDigits, startAt, 0, casProvider)
}

// GetBatchIDGeneratorWithBuffer returns a batch ID generator with optional queue buffer
// bufferSize: 0 = no queue (direct generation), >0 = use pre-filled queue
func GetBatchIDGeneratorWithBuffer(ctx context.Context, kindDir, kind, prefix string, minDigits, startAt, bufferSize int) *BatchIDGenerator {
	return GetBatchIDGeneratorWithBufferAndCAS(ctx, kindDir, kind, prefix, minDigits, startAt, bufferSize, nil)
}

// GetBatchIDGeneratorWithBufferAndCAS returns a batch ID generator with optional queue buffer and CAS support
// bufferSize: 0 = no queue (direct generation), >0 = use pre-filled queue
// casProvider: optional function to query CAS for existing IDs in the bucket directory
func GetBatchIDGeneratorWithBufferAndCAS(ctx context.Context, kindDir, kind, prefix string, minDigits, startAt, bufferSize int, casProvider CASIDProvider) *BatchIDGenerator {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	// If buffer size specified, use queue-backed generator
	if bufferSize > 0 {
		queueManager := GetGlobalQueueManager(ctx)
		queue := queueManager.GetOrCreateQueue(kindDir, kind, prefix, minDigits, startAt, bufferSize)

		// Initialize pattern for fallback direct generation
		prefixWithDash := prefix
		if len(prefixWithDash) == 0 || prefixWithDash[len(prefixWithDash)-1] != '-' {
			prefixWithDash += "-"
		}
		seqPattern := regexp.MustCompile(fmt.Sprintf(`^%s(\d+)$`, regexp.QuoteMeta(prefixWithDash)))

		return &BatchIDGenerator{
			ctx:             ctx,
			kindDir:         kindDir,
			kind:            kind,
			prefix:          prefix,
			minDigits:       minDigits,
			startAt:         startAt,
			lastSequence:    startAt - 1,
			initialized:     false,
			scanThreshold:   500,
			scanCount:       0,
			seqPattern:      seqPattern, // Set for fallback direct generation
			lastScanTime:    time.Time{},
			queue:           queue, // Queue-backed generator
			casProvider:     casProvider,
			sequenceFileDir: kindDir, // Cross-process safe allocation for all kinds
		}
	}

	// No buffer - use pool-based direct generation
	pool := GetGlobalGeneratorPool()
	generator, isDedicated := pool.GetOrCreateGeneratorWithCAS(ctx, kindDir, kind, prefix, minDigits, startAt, casProvider)
	_ = isDedicated
	return generator
}

// GetBatchIDGeneratorLegacy returns (or creates) a batch ID generator using the old registry pattern
// DEPRECATED: Use GetBatchIDGenerator for pool-based allocation
func GetBatchIDGeneratorLegacy(ctx context.Context, kindDir, kind, prefix string, minDigits, startAt int) *BatchIDGenerator {
	// Create unique key for this generator (kind + directory)
	key := fmt.Sprintf(ConstSFmtSSSDD, kindDir, kind, prefix, minDigits, startAt)

	// Fast path: check if generator exists (read lock)
	var generator *BatchIDGenerator
	var exists bool
	_ = concurrency.RunInRLockOrLog(
		&batchGeneratorsLock, locknames.LockNameBatchGeneratorLegacyGetFast, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			generator, ok = batchGenerators[key]
			exists = ok
			return nil
		},
	)

	if exists {
		return generator
	}

	// Slow path: create new generator (write lock)
	_ = concurrency.RunInLockOrLog(
		&batchGeneratorsLock, locknames.LockNameBatchGeneratorLegacyGetCreate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Double-check after acquiring write lock (another goroutine might have created it)
			if existingGen, ok := batchGenerators[key]; ok {
				generator = existingGen
				return nil
			}

			// Create new generator
			generator = NewBatchIDGenerator(ctx, kindDir, kind, prefix, minDigits, startAt)
			batchGenerators[key] = generator
			return nil
		},
	)

	return generator
}

// NewBatchIDGenerator creates a new batch ID generator for a specific kind+directory
// The generator caches the last sequence number and periodically rescans the directory
// to ensure accuracy, minimizing lock contention while maintaining correctness
func NewBatchIDGenerator(ctx context.Context, kindDir, kind, prefix string, minDigits, startAt int) *BatchIDGenerator {
	return NewBatchIDGeneratorWithCAS(ctx, kindDir, kind, prefix, minDigits, startAt, nil)
}

// NewBatchIDGeneratorWithCAS creates a new batch ID generator with optional CAS support
// casProvider: optional function to query CAS for existing IDs in the bucket directory
func NewBatchIDGeneratorWithCAS(ctx context.Context, kindDir, kind, prefix string, minDigits, startAt int, casProvider CASIDProvider) *BatchIDGenerator {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}
	// Ensure prefix has trailing dash for pattern matching
	prefixWithDash := prefix
	if len(prefixWithDash) == 0 || prefixWithDash[len(prefixWithDash)-1] != '-' {
		prefixWithDash += "-"
	}

	// Compile regex pattern once for efficiency
	seqPattern := regexp.MustCompile(fmt.Sprintf(`^%s(\d+)$`, regexp.QuoteMeta(prefixWithDash)))

	return &BatchIDGenerator{
		ctx:             ctx,
		kindDir:         kindDir,
		kind:            kind,
		prefix:          prefix,
		minDigits:       minDigits,
		startAt:         startAt,
		lastSequence:    startAt - 1, // Start at (startAt - 1) so first ID is startAt
		initialized:     false,
		scanThreshold:   500, // Rescan directory every 500 IDs (increased to reduce I/O)
		scanCount:       0,
		seqPattern:      seqPattern,
		lastScanTime:    time.Time{},
		queue:           nil, // Direct generation (no queue)
		casProvider:     casProvider,
		sequenceFileDir: kindDir, // Cross-process safe allocation for all kinds
	}
}

// ensurePattern ensures the regex pattern is initialized (for queue-backed generators)
func (g *BatchIDGenerator) ensurePattern() {
	if g.seqPattern != nil {
		return
	}

	// Initialize pattern if not set (for queue-backed generators)
	prefixWithDash := g.prefix
	if len(prefixWithDash) == 0 || prefixWithDash[len(prefixWithDash)-1] != '-' {
		prefixWithDash += "-"
	}
	g.seqPattern = regexp.MustCompile(fmt.Sprintf(`^%s(\d+)$`, regexp.QuoteMeta(prefixWithDash)))
}

// GenerateNextID generates a single ID (thread-safe)
// If queue is available, pops from queue (zero contention)
// Otherwise, uses direct generation with lock
func (g *BatchIDGenerator) GenerateNextID() (string, error) {
	// Fast path: use queue if available (zero contention)
	if g.queue != nil {
		id, err := g.queue.Pop()
		if err == nil {
			return id, nil
		}
		// Queue empty - fallback to direct generation
		// Background worker should refill, but we don't block
	}

	// Fallback: direct generation (for non-queued generators or queue exhaustion)
	ids, err := g.GenerateBatchIDs(1)
	if err != nil {
		return "", err
	}
	if len(ids) == 0 {
		return "", errfmt.Errorf(ConstGeneratedZeroIDs)
	}
	return ids[0], nil
}

// SetSequenceFileDir enables cross-process safe allocation using a sequence file in dir.
// When set, GenerateNextID and GenerateBatchIDs allocate from the file (one lock per batch).
// Call after construction (e.g. from GetAuditIDGenerator).
func (g *BatchIDGenerator) SetSequenceFileDir(dir string) {
	var err_swallow_1 = concurrency.RunInLock(&g.mu, func() error {
		g.sequenceFileDir = dir
		return nil
	})
	if err_swallow_1 !=

		// GenerateBatchIDs generates multiple IDs in a single batch (thread-safe).
		// When sequenceFileDir is set, allocates from the sequence file (atomic range); otherwise uses scan-based allocation.
		nil {
		logging.LogSwallowedError(err_swallow_1)
	}
}

func (g *BatchIDGenerator) GenerateBatchIDs(batchSize int) ([]string, error) {
	if batchSize <= 0 {
		batchSize = 1
	}

	var seqDir string
	var err_swallow_2 = concurrency.RunInLock(&g.mu, func() error {
		seqDir = g.sequenceFileDir
		return nil
	})
	if err_swallow_2 != nil {
		logging.LogSwallowedError(err_swallow_2)
	}
	if seqDir != emptyValue {
		seedMax := func() (int, error) { return g.GetMaxSequence() }
		ids, err := AllocateSequenceRange(seqDir, g.prefix, g.minDigits, g.startAt, batchSize, seedMax)
		if err == nil {
			return ids, nil
		}
		// Do not fall back to scan-based allocation when sequence file was intended:
		// scan-based is not cross-process safe and could hand out duplicate IDs.
		return nil, errfmt.Errorf(ConstSequenceFileAllocationFailedPrefixSW, g.prefix, err)
	}

	var ids []string
	var genErr error
	err := concurrency.RunInLockWithLogger(
		&g.mu, locknames.LockNameBatchGeneratorGenerateBatch, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Initialize or rescan if needed
			// Check shared cache first to avoid redundant scans
			needsScan := !g.initialized || g.scanCount >= g.scanThreshold
			if needsScan {
				// Try to get from shared cache first (avoids redundant directory scans)
				cachedMaxSeq := g.getCachedMaxSequence()
				if cachedMaxSeq >= 0 {
					// Cache hit - use cached value
					if cachedMaxSeq > g.lastSequence {
						g.lastSequence = cachedMaxSeq
					}
					g.initialized = true
					g.scanCount = 0
					g.lastScanTime = time.Now()
				} else {
					// Cache miss or stale - scan directory
					// NOTE: scanDirectory no longer unlocks/re-locks g.mu to avoid double-unlock
					// The lock is managed by WithLockTimeout
					genErr = g.scanDirectory()
					if genErr != nil {
						return errfmt.Newf(ConstFailedToScanDirectory).Wrap(genErr)
					}
					g.initialized = true
					g.scanCount = 0
					g.lastScanTime = time.Now()
				}
			}

			// Generate batch of IDs
			ids = make([]string, 0, batchSize)
			startSequence := g.lastSequence + 1
			if startSequence < g.startAt {
				startSequence = g.startAt
			}

			for i := 0; i < batchSize; i++ {
				sequence := startSequence + i
				// Format with minimum digits
				formatStr := fmt.Sprintf("%%s%%0%dd", g.minDigits)
				prefixWithDash := g.prefix
				if len(prefixWithDash) == 0 || prefixWithDash[len(prefixWithDash)-1] != '-' {
					prefixWithDash += "-"
				}
				id := fmt.Sprintf(formatStr, prefixWithDash, sequence)
				ids = append(ids, id)
			}

			// Update last sequence (atomic within lock)
			g.lastSequence += batchSize
			g.scanCount += batchSize

			// Record generation for volume tracking (for pool allocation decisions)
			pool := GetGlobalGeneratorPool()
			pool.RecordGeneration(g.kindDir, g.kind, g.prefix, batchSize)

			return nil
		},
	)
	if err != nil {
		return nil, err
	}
	if genErr != nil {
		return nil, genErr
	}
	return ids, nil
}

// getCachedMaxSequence gets max sequence from shared cache (if available and fresh)
// Returns -1 if cache miss or stale
// Must be called with g.mu locked
// NOTE: Cache key includes prefix because different prefixes in same directory have different sequences
func (g *BatchIDGenerator) getCachedMaxSequence() int {
	cacheKey := fmt.Sprintf("%s:%s", g.kindDir, g.prefix)
	var cached *dirScanResult
	var exists bool
	_ = concurrency.RunInRLockOrLog(
		&dirScanCacheLock, locknames.LockNameBatchGeneratorGetCachedCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			cached, ok = dirScanCache[cacheKey]
			exists = ok
			return nil
		},
	)

	if !exists {
		return -1 // Cache miss
	}

	var scanTime time.Time
	var maxSeq int
	_ = concurrency.RunInRLockOrLog(
		&cached.mu, locknames.LockNameBatchGeneratorGetCachedRead, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			scanTime = cached.scanTime
			maxSeq = cached.maxSequence
			return nil
		},
	)

	// Check if cache is fresh (within TTL)
	if time.Since(scanTime) < dirScanConfig.CacheTTL {
		return maxSeq
	}

	return -1 // Cache stale
}

// scanDirectory scans the directory to find the highest existing sequence number
// CRITICAL: Releases generator lock during file I/O to prevent blocking other generators
// Uses per-directory scan lock to prevent concurrent scans of the same directory
// Must be called with g.mu locked, but releases lock for I/O
// NOTE: Uses direct mutex unlock/lock because it releases the lock before I/O and re-acquires
// it after, which doesn't fit the timeout wrapper pattern (wrapper expects to manage full lifecycle)

// Ensure pattern is initialized (for queue-backed generators)

// Check cache first (fast path, no I/O)

// Cache is fresh - use it without scanning

// Cache miss or stale - need to scan
// Cache key includes prefix (different prefixes in same directory have different sequences)

// Get per-directory scan lock to prevent concurrent scans of same directory+prefix

// Double-check cache after acquiring directory lock (another goroutine may have scanned)

// Cache is fresh - use it without scanning

// Early return - lock will be released by defer

// CRITICAL: Do NOT unlock here - we're called from WithLockTimeout which manages the lock
// Unlocking here causes "unlock of unlocked mutex" panic when WithLockTimeout's defer tries to unlock
// The lock is managed by WithLockTimeout, so we must keep it held during I/O
// This is acceptable because:
// 1. Directory lock (dirLock) prevents concurrent scans of same directory
// 2. Directory I/O is wrapped with timeout to prevent blocking
// 3. Other generators for different directories can still proceed (they have different locks)
// NOTE: If this becomes a bottleneck, we should refactor to not use WithLockTimeout for scanDirectory

// CRITICAL: Use non-blocking IO with timeout and retry logic per architecture requirements
// ReadDir can block indefinitely on directories with many files (e.g., 29k+ files)
// Follow established pattern: non-blocking goroutine + timeout + retry with exponential backoff
// Per concurrency-patterns-v1.0.md and concurrent-operations-enhancements-v1.0.md
// Use shared DirectoryScanConfig to avoid duplication (matches RetryConfig pattern from operation_helper.go)

// Execute ReadDir with retry logic (per architecture requirements)

// Check context cancellation

// Non-blocking ReadDir with timeout (per IO + Async On-Demand pattern)

// Execute ReadDir in goroutine to prevent blocking

// Wait for result with timeout (non-blocking IO pattern)

// Success or file not found (acceptable) - break out of retry loop

// Check if error is retryable (timeout errors are retryable)

// ReadDir timeout - retryable error

// Context cancelled - non-retryable

// If we got entries successfully, break out of retry loop

// Last attempt, don't wait

// Wait before retry with exponential backoff (per architecture pattern)

// Continue to next attempt

// Exponential backoff

// Check if we failed after all retries

// Success - continue processing entries
// Find the highest existing sequence number (no lock needed for this computation)

// Scan file system for ID-based files

// Extract ID from filename (remove .yaml/.yml extension)

// Try to match pattern and extract sequence
// Files that don't match the pattern (e.g., account:username format) are automatically skipped
// No need for hardcoded kind-specific logic - the regex pattern handles filtering

// Also query CAS if provider is available (for CAS-enabled kinds with hash-based filenames)

// Extract sequence numbers from CAS IDs

// If CAS query fails, continue with file system scan results (best effort)

// Update shared cache (for other generators to use)
// Cache key includes prefix (different prefixes in same directory have different sequences)
// cacheKey was already declared earlier in scanDirectory function

// Lock is still held (we didn't unlock above)
// Update last sequence (use max if higher than current)

// isRetryableReadDirError determines if a ReadDir error is retryable
// Per architecture: timeout errors and temporary errors are retryable

// Timeout errors are retryable

// Temporary errors are retryable

// Permission errors and file not found are NOT retryable

// By default, don't retry (safer)

// GetLastSequence returns the last sequence number used (for testing/debugging)
func (g *BatchIDGenerator) GetLastSequence() int {
	var lastSeq int
	_ = concurrency.RunInLockOrLog(
		&g.mu, locknames.LockNameBatchGeneratorGetLastSequence, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			lastSeq = g.lastSequence
			return nil
		},
	)
	return lastSeq
}

// GetMaxSequence ensures the generator has scanned (if needed) and returns the current
// maximum sequence number without allocating. Used to seed cross-process sequence files
// so the next ID is never handed out twice. Returns (0, nil) if no IDs exist yet.
func (g *BatchIDGenerator) GetMaxSequence() (int, error) {
	var maxSeq int
	var genErr error
	err := concurrency.RunInLockWithLogger(
		&g.mu, locknames.LockNameBatchGeneratorGetMaxSequence, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			needsScan := !g.initialized || g.scanCount >= g.scanThreshold
			if needsScan {
				cachedMaxSeq := g.getCachedMaxSequence()
				if cachedMaxSeq >= 0 {
					if cachedMaxSeq > g.lastSequence {
						g.lastSequence = cachedMaxSeq
					}
					g.initialized = true
					g.scanCount = 0
					g.lastScanTime = time.Now()
				} else {
					genErr = g.scanDirectory()
					if genErr != nil {
						return errfmt.Newf(ConstFailedToScanDirectory).Wrap(genErr)
					}
					g.initialized = true
					g.scanCount = 0
					g.lastScanTime = time.Now()
				}
			}
			maxSeq = g.lastSequence
			if maxSeq < g.startAt {
				maxSeq = g.startAt - 1
			}
			return nil
		},
	)
	if err != nil {
		return 0, err
	}
	if genErr != nil {
		return 0, genErr
	}
	return maxSeq, nil
}

// Reset forces a rescan on the next ID generation (for testing)
func (g *BatchIDGenerator) Reset() {
	_ = concurrency.RunInLockOrLog(
		&g.mu, locknames.LockNameBatchGeneratorReset, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			g.initialized = false
			g.scanCount = 0
			return nil
		},
	)
}
