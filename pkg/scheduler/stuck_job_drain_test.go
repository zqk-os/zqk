package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

func TestActivityEntryOpenStartedAge(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 11, 12, 22, 0, 0, 0, time.UTC)
	started := now.Add(-2 * time.Hour)

	age, ok := activityEntryOpenStartedAge(&ActivityCacheEntry{LastStarted: started}, now)
	require.True(t, ok)
	require.Equal(t, 2*time.Hour, age)

	_, ok = activityEntryOpenStartedAge(&ActivityCacheEntry{
		LastStarted:   started,
		LastCompleted: now.Add(-time.Hour),
	}, now)
	require.False(t, ok, "completed after start is not open")

	_, ok = activityEntryOpenStartedAge(&ActivityCacheEntry{
		LastStarted: started,
		LastFailed:  now.Add(-30 * time.Minute),
	}, now)
	require.False(t, ok, "failed after start is not open")

	_, ok = activityEntryOpenStartedAge(nil, now)
	require.False(t, ok)
}

func TestStuckDrainThresholdForJob(t *testing.T) {
	t.Parallel()
	stale := 30 * time.Minute
	require.Equal(t, stale, stuckDrainThresholdForJob(nil, false, stale))

	job := &ScheduledJob{MaxRuntimeSeconds: 3600}
	require.Equal(t, time.Hour, stuckDrainThresholdForJob(job, true, stale))
	require.Equal(t, stale, stuckDrainThresholdForJob(job, false, stale))
}

func TestDrainStuckStartedJobs_WritesQueryableFailedAudit(t *testing.T) {
	testRoot, fileStorage := prepareHandlersIsolatedTempProject(t)
	s := &Scheduler{
		projectRoot: testRoot,
		storage:     fileStorage,
		jobs:        make(map[string]*ScheduledJob),
		logger:      nil,
	}
	jobID := "SCH-STUCK-DRAIN-HIST-001"
	s.jobs[jobID] = &ScheduledJob{ID: jobID, JobType: "run_wrapper", Category: "manual"}

	cache := GetGlobalActivityCache().(*ActivityCache)
	require.NoError(t, cache.LoadCache(testRoot))
	cache.mu.Lock()
	cache.entries[jobID] = &ActivityCacheEntry{
		JobID:       jobID,
		LastStarted: time.Now().UTC().Add(-2 * time.Hour),
	}
	cache.mu.Unlock()
	t.Cleanup(func() {
		cache.mu.Lock()
		delete(cache.entries, jobID)
		cache.mu.Unlock()
	})

	n := s.DrainStuckStartedJobs(context.Background(), 30*time.Minute)
	require.Equal(t, 1, n)

	secCtx := pkgctx.NewSystemSecurityContext()
	events, err := fileStorage.List(context.Background(), secCtx, nil, storagepkg.ListFilter{
		Kind: "audit_event",
	})
	require.NoError(t, err)

	var matching []map[string]any
	if events != nil {
		for _, ev := range events.Objects {
			if objects.GetString(ev, objects.FieldKeyTargetID) == jobID {
				matching = append(matching, ev)
			}
		}
	}
	require.NotEmpty(t, matching, "expected durable scheduler_job_failed for drained stuck job %s", jobID)

	et := objects.GetString(matching[0], objects.FieldKeyEventType)
	if et == "" {
		if md, ok := matching[0]["metadata"].(map[string]any); ok {
			et, _ = md["event_type"].(string)
		}
	}
	require.Equal(t, auditEventSchedulerFailed, et)

	// Idempotent: second pass must not re-drain.
	require.Equal(t, 0, s.DrainStuckStartedJobs(context.Background(), 30*time.Minute))
}

func TestDrainStuckStartedJobs_NilSafe(t *testing.T) {
	t.Parallel()
	var s *Scheduler
	require.Equal(t, 0, s.DrainStuckStartedJobs(context.Background(), time.Minute))
	require.Equal(t, 0, (&Scheduler{}).DrainStuckStartedJobs(context.Background(), 0))
}
