package system

import (
	stdcontext "context"
	"fmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objectidcache"
	"github.com/lanceman/zqk/pkg/storage"
)

// CacheFreshnessHandler is a function type for performing cache freshness checks.
type CacheFreshnessHandler func(projectRoot, reason, triggerOperation string, affectedKinds []string) int

var cacheFreshnessHandler CacheFreshnessHandler

// RegisterCacheFreshnessHandler registers a handler for cache freshness checks.
func RegisterCacheFreshnessHandler(handler CacheFreshnessHandler) {
	cacheFreshnessHandler = handler
}

// PerformCacheFreshnessCheck validates/cleans object-id-cache entries and updates hash registries.
func PerformCacheFreshnessCheck(projectRoot, reason, triggerOperation string, affectedKinds []string) int {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return 0
	}

	cache := objectidcache.GetGlobalObjectIDCache()
	hashRegistryUpdated := updateHashRegistryForModifiedFiles(cache, projectRoot, affectedKinds)
	staleCount := cache.ValidateAndCleanStale()

	if staleCount > 0 {
		storage.InvalidateListCache()
	}

	if staleCount > 0 || hashRegistryUpdated > 0 {
		createCacheAuditEvent("cache_cleanup", "", "", "",
			fmt.Sprintf("Cleaned %d stale cache entries, updated %d hash registry entries", staleCount, hashRegistryUpdated), "low", "human")
		if err := cache.SaveCache(projectRoot); err != nil {
			l := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			logging.Fluent(l).Warn("Failed to save cache after cleanup").WithError(err).Log()
		}
	}
	return staleCount
}

func updateHashRegistryForModifiedFiles(cache *objectidcache.ObjectIDCache, projectRoot string, affectedKinds []string) int {
	updateCtx := initializeHashRegistryUpdateContext(cache, projectRoot, affectedKinds)
	collectModifiedFiles(updateCtx)

	if len(updateCtx.KindToFiles) == 0 {
		return 0
	}

	type kindWork struct {
		kind      string
		filePaths []string
	}
	var work []kindWork
	for kind, filePaths := range updateCtx.KindToFiles {
		work = append(work, kindWork{kind: kind, filePaths: filePaths})
	}

	results := make(chan int, len(work))
	maxConcurrency := min(8, len(work))
	queueSize := min(len(work), 256)

	poolCtx := stdcontext.Background() // Background: request-or-shutdown derived
	pool := goroutinelabels.NewPool(goroutinelabels.DefaultBudget(), "update_hash_registry", "updating hash registry", maxConcurrency, queueSize)
	pool.Start(poolCtx)
	for _, w := range work {
		kind, filePaths := w.kind, w.filePaths
		_ = pool.Submit(poolCtx, func(stdcontext.Context) error {
			n := updateHashRegistryForKind(updateCtx, kind, filePaths)
			results <- n
			return nil
		})
	}
	pool.Stop()

	updatedCount := 0
	for i := 0; i < len(work); i++ {
		updatedCount += <-results
	}
	return updatedCount
}
