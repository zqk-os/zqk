package system

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/cliapp"

	"github.com/spf13/cobra"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/testkit"
)

// TestInitializeAsyncCheckContext_EnqueuesCachePrewarmTrigger verifies that when system check
// runs with --auto-fix and scheduler batching, a trigger for the cache_prewarm job (SCH-cache-prewarm)
// is enqueued so it runs once when the scheduler starts instead of waiting for the next timer tick.
func TestInitializeAsyncCheckContext_EnqueuesCachePrewarmTrigger(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, projectRoot, nil)
	cmd := &cobra.Command{}
	cmd.Flags().Bool("auto-fix", false, "")
	cmd.Flags().Bool("auto-fix-scheduler", true, "")
	_ = cmd.Flags().Set("auto-fix", "true")
	_ = cmd.Flags().Set("auto-fix-scheduler", "true")

	ctx := cli.ContextForProjectRoot(projectRoot)
	_, err := initializeAsyncCheckContext(cmd, ctx)
	if err != nil {
		t.Fatalf("initializeAsyncCheckContext: %v", err)
	}
	defer func() {
		for i := 0; i < 50; i++ {
			if sched := schedulerpkg.GetGlobalScheduler(); sched != nil {
				sched.Stop()
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}()

	queue := schedulerpkg.NewJobTriggerQueue(projectRoot)
	reqs, err := queue.PeekTriggerRequests()
	if err != nil {
		t.Fatalf("PeekTriggerRequests: %v", err)
	}
	var found bool
	for _, r := range reqs {
		if r.JobID == schedulerpkg.DefaultCachePrewarmJobID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected trigger queue to contain %q after init with auto-fix; got %d request(s): %v",
			schedulerpkg.DefaultCachePrewarmJobID, len(reqs), reqs)
	}
}

// TestDefaultCachePrewarmJobID_IsSCH007 ensures the well-known cache_prewarm job ID
// matches the remedial policy so enqueue logic and docs stay in sync.
func TestDefaultCachePrewarmJobID_IsSCH007(t *testing.T) {
	t.Parallel()
	if schedulerpkg.DefaultCachePrewarmJobID != "SCH-cache-prewarm" {
		t.Errorf("DefaultCachePrewarmJobID = %q, want SCH-cache-prewarm (remedial policy)", schedulerpkg.DefaultCachePrewarmJobID)
	}
}
