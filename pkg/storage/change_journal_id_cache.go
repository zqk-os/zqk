// Package storage: per-project batch cache for change journal entry IDs.
// When stream storage is enabled, change journal IDs come from a sequence file (.zqk/state/.next_CHA_seq).
// Allocating one ID at a time causes severe flock contention under load (hundreds of goroutines blocked).
// This cache allocates IDs in batches (e.g. 100) so we take the sequence file lock once per 100 IDs
// instead of once per ID. See SCHEDULER_SAMPLE_FLOCK_AND_JOB_SETUP_20260313.md.
//
// Lock and goroutine usage follow project patterns: concurrency.RunInLockWithLogger for all mutex
// sections, goroutinelabels.NewGoroutine for async refill (see pkg/storage/id_generation/queue.go,
// pkg/concurrency, docs/architecture/architecture/concurrency-patterns-v1.0.md).

package storage

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	idgen "github.com/lanceman/zqk/pkg/storage/id_generation"
	"github.com/lanceman/zqk/pkg/storage/locknames"
)

const (
	changeJournalIDBatchSize   = 100
	changeJournalIDRefillBelow = 20
)

// changeJournalIDCache holds pre-allocated CHA IDs for one project to reduce sequence file contention.
type changeJournalIDCache struct {
	mu                sync.Mutex
	ids               []string
	generator         *idgen.BatchIDGenerator
	stateDir          string
	idsAllocatedTotal atomic.Int64
	idsDispensedTotal atomic.Int64
}

// GetIDCacheStats returns lifetime counters for allocated and dispensed IDs.
func (c *changeJournalIDCache) GetIDCacheStats() (allocated, dispensed int64) {
	return c.idsAllocatedTotal.Load(), c.idsDispensedTotal.Load()
}

var (
	changeJournalCaches      = make(map[string]*changeJournalIDCache)
	changeJournalCachesMu    sync.RWMutex
	globalCachesCreatedTotal atomic.Int64
	globalCachesReusedTotal  atomic.Int64
)

// GetGlobalChangeJournalIDCacheStats returns lifetime counters for created and reused change journal ID caches.
func GetGlobalChangeJournalIDCacheStats() (created, reused int64) {
	return globalCachesCreatedTotal.Load(), globalCachesReusedTotal.Load()
}

func getChangeJournalIDCacheLogger() concurrency.LockLogger {
	return logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem))
}

// getChangeJournalIDCache returns the cache for the given project root (creates if needed).
// Caller must only pass non-empty projectRoot when stream storage is enabled for change_journal_entry.
func getChangeJournalIDCache(ctx context.Context, projectRoot string) *changeJournalIDCache {
	stateDir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.StateDir)

	var c *changeJournalIDCache
	var exists bool
	_ = concurrency.RunInRLockOrLog(
		&changeJournalCachesMu,
		locknames.LockNameChangeJournalIdCacheGet,
		getChangeJournalIDCacheLogger(),
		func() error {
			var ok bool
			c, ok = changeJournalCaches[stateDir]
			exists = ok
			return nil
		},
	)
	if exists {
		globalCachesReusedTotal.Add(1)
		return c
	}

	_ = concurrency.RunInLockOrLog(
		&changeJournalCachesMu,
		locknames.LockNameChangeJournalIdCacheCreate,
		getChangeJournalIDCacheLogger(),
		func() error {
			if existing, ok := changeJournalCaches[stateDir]; ok {
				c = existing
				globalCachesReusedTotal.Add(1)
				return nil
			}
			generator := idgen.GetBatchIDGenerator(ctx, stateDir, objects.KindChangeJournalEntry, "CHA", 3, 1)
			generator.SetSequenceFileDir(stateDir)
			c = &changeJournalIDCache{
				ids:       make([]string, 0, changeJournalIDBatchSize),
				generator: generator,
				stateDir:  stateDir,
			}
			changeJournalCaches[stateDir] = c
			globalCachesCreatedTotal.Add(1)
			return nil
		},
	)
	return c
}

// nextID returns the next cached ID, refilling the cache with a batch if needed.
// One flock per batch instead of one per ID.
func (c *changeJournalIDCache) nextID(ctx context.Context) (string, error) {
	var id string
	var needRefill bool
	var popOk bool
	_ = concurrency.RunInLockOrLog(
		&c.mu,
		locknames.LockNameChangeJournalIdCacheNext,
		getChangeJournalIDCacheLogger(),
		func() error {
			if len(c.ids) > 0 {
				id = c.ids[0]
				c.ids = c.ids[1:]
				popOk = true
				needRefill = len(c.ids) < changeJournalIDRefillBelow
			}
			return nil
		},
	)
	if popOk {
		c.idsDispensedTotal.Add(1)
		if needRefill {
			refillCtx := context.WithoutCancel(ctx)
			goroutinelabels.NewGoroutine(ConstAuditChangeJournalIdCacheRefill, ConstAuditAsyncRefillOfChangeJournalIdCache).
				StartWithContext(refillCtx, func(refillCtx context.Context) error {
					var _err_82334163 = c.refill(refillCtx)
					if _err_82334163 != nil {
						logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).

							// Cache empty: allocate one batch (one flock) then take one
							Error(ErrMsgSwallowedError,

								_err_82334163).Log()
					}
					return nil
				})
		}
		return id, nil
	}

	if err := c.refill(ctx); err != nil {
		return "", err
	}
	var out string
	err := concurrency.RunInLockWithLogger(
		&c.mu,
		locknames.LockNameChangeJournalIdCacheNext,
		getChangeJournalIDCacheLogger(),
		func() error {
			if len(c.ids) == 0 {
				return errfmt.Errorf(ConstAuditChangeJournalIdCacheRefillProducedNoIds)
			}
			out = c.ids[0]
			c.ids = c.ids[1:]
			return nil
		},
	)
	if err != nil {
		return "", err
	}
	c.idsDispensedTotal.Add(1)
	return out, nil
}

func (c *changeJournalIDCache) refill(ctx context.Context) error {
	ids, err := c.generator.GenerateBatchIDs(changeJournalIDBatchSize)
	if err != nil {
		return err
	}
	c.idsAllocatedTotal.Add(int64(len(ids)))
	_ = concurrency.RunInLockOrLog(
		&c.mu,
		locknames.LockNameChangeJournalIdCacheRefill,
		getChangeJournalIDCacheLogger(),
		func() error {
			c.ids = append(c.ids, ids...)
			return nil
		},
	)
	return nil
}
