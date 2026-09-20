package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// oidBuildCacheDeadlineRecorder records the job context deadline when the object ID cache builder runs.
// Regression: Object ID prewarm must not run under the parallel Tier 3b ~50s context; it must use the
// Tier 3a sequential budget (15m) so large builds are not cancelled before writing object-id-cache.json.
type oidBuildCacheDeadlineRecorder struct {
	mu         sync.Mutex
	remaining  time.Duration
	wasInvoked bool
}

func (r *oidBuildCacheDeadlineRecorder) BuildCache(ctx context.Context, _ string, _ bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.wasInvoked = true
	if dl, ok := ctx.Deadline(); ok {
		r.remaining = time.Until(dl)
	}
	return nil
}

func (r *oidBuildCacheDeadlineRecorder) snapshot() (invoked bool, rem time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.wasInvoked, r.remaining
}

func TestCachePrewarm_ObjectIDBuildCacheGetsSequentialTierDeadlineNotParallel50s(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	storagepkg.BuildPathAliasCacheForProject(env.TestRoot)

	recorder := &oidBuildCacheDeadlineRecorder{}
	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	h := NewCachePrewarmHandler(env.SpecLoader, env.LifecycleLoader, storage, env.TestRoot, recorder)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Hour)
	defer cancel()

	job := &ScheduledJob{ID: "SCH-oid-tier-deadline", JobType: JobTypeCachePrewarm}
	if err := h.(*CachePrewarmHandler).executeCachePrewarmCore(ctx, job); err != nil {
		t.Fatalf("executeCachePrewarmCore: %v", err)
	}

	invoked, rem := recorder.snapshot()
	if !invoked {
		t.Fatal("expected ObjectIDCacheBuilder.BuildCache to run (inject non-nil builder with project root)")
	}
	if rem <= 55*time.Second {
		t.Fatalf("OID BuildCache context deadline too tight (%v); suspected parallel-tier ~50s regression", rem)
	}
}
