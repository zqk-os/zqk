package system

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objectidcache"
)

func init() {
	objectidcache.SetProjectRootResolver(ProjectRootOrResolveDot)
	objectidcache.SetCacheAuditFunc(createCacheAuditEvent)
	objectidcache.SetCacheLifecycleObservers(
		func(projectRoot string, entryCount int, saveDuration time.Duration) {
			emitCacheSaveEventViaCoordinator(context.Background(), projectRoot, nil, entryCount, saveDuration, "system") // Background: request-or-shutdown derived
		},
		func(projectRoot, operation string, entryCount int, forceRebuild bool, buildDuration time.Duration) {
			emitCacheBuildEventViaCoordinator(context.Background(), projectRoot, nil, operation, entryCount, forceRebuild, buildDuration, "system") // Background: request-or-shutdown derived
		},
		func(id string, logger logging.Logger) {
			GetGlobalCacheItemStrategyRegistry().HandleCacheHit(id, logger)
		},
		func(id string, cacheSize int, logger logging.Logger) {
			GetGlobalCacheItemStrategyRegistry().HandleCacheMiss(id, cacheSize, logger)
		},
	)
	objectidcache.SetSidecarsIdleEmitter(func(root string) {
		emitCacheSidecarsIdleViaCoordinator(context.Background(), root) // Background: request-or-shutdown derived
	})
}
