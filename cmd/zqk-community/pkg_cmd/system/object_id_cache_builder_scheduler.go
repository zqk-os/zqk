package system

import (
	"context"

	schedulerpkg "github.com/lanceman/zqk/pkg/scheduler"
)

// objectIDCacheBuilderForScheduler implements scheduler.ObjectIDCacheBuilder so the
// scheduler (daemon or in-process) can build the object ID cache in the background when
// cache_prewarm jobs run. This keeps all prewarmable caches built in the background.
var _ schedulerpkg.ObjectIDCacheBuilder = (*objectIDCacheBuilderForScheduler)(nil)

type objectIDCacheBuilderForScheduler struct{}

// BuildCache builds the object ID cache; called by the scheduler when executing cache_prewarm.
// Uses EnsureObjectIDCacheReady so the scheduler shares the same serialized load/build path as
// the CLI (avoids concurrent BuildCache with TriggerBackgroundObjectIDCacheBuild and lock contention).
func (objectIDCacheBuilderForScheduler) BuildCache(ctx context.Context, projectRoot string, forceRebuild bool) error {
	if projectRoot == emptyValue {
		return nil // nowhere to write; skip (handler already checks h.projectRoot != emptyValue)
	}
	return EnsureObjectIDCacheReady(ctx, projectRoot, forceRebuild, nil, nil)
}

// NewObjectIDCacheBuilderForScheduler returns an ObjectIDCacheBuilder for use by the scheduler.
// Pass it to NewSchedulerWithProjectRoot so cache_prewarm jobs can build the object ID cache
// in the background (daemon started via `zqk scheduler start` or in-process during system check).
func NewObjectIDCacheBuilderForScheduler() schedulerpkg.ObjectIDCacheBuilder {
	return &objectIDCacheBuilderForScheduler{}
}
