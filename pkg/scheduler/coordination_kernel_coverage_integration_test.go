package scheduler

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CRIT-9042: Coordination kernel test coverage.
// Evidence: scheduler pre-flight honors persisted policy default_action=skip when no execution state exists yet.
func TestCRIT9042_SchedulerIntegration_PolicyDefaultActionSkip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	t.Parallel()

	sched, testRoot, cleanup := setupTestScheduler(t)
	defer cleanup()

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"developer"}, []string{
		"read:scheduler_job", "execute:scheduler_job",
	})
	sched.SetSecurityContext(secCtx)

	jobID := "SCH-9042-POLICY-SKIP"
	createTestJob(t, sched.storage, jobID, "lifecycle_check", "manual", "")

	ctx := pkgctx.NewSystemContext()
	if err := sched.loadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("failed to load jobs: %v", err)
	}

	// Persist policy so PolicyEngine can read it from disk.
	if sched.policyEngine == nil {
		t.Fatalf("expected scheduler.policyEngine to be initialized")
	}
	if err := sched.policyEngine.SavePolicy(&ExecutionPolicy{
		JobID:         jobID,
		DefaultAction: decisionSkip,
		Rules:         []PolicyRule{},
	}); err != nil {
		t.Fatalf("SavePolicy failed: %v", err)
	}

	// Trigger without callback: skip path returns early and doesn't invoke op callbacks.
	if err := sched.TriggerJob(ctx, jobID); err != nil {
		t.Fatalf("failed to trigger job: %v", err)
	}

	eventsLogPath := filepath.Join(testRoot, paths.ProjectDataDir, paths.SchedulerDir, "events", "coordination-bus.jsonl")
	deadline := time.Now().Add(3 * time.Second)
	var sawRequested, sawSkipped, sawStarted bool

	for time.Now().Before(deadline) {
		b, err := fileutil.ReadFile(eventsLogPath)
		if err != nil {
			time.Sleep(50 * time.Millisecond)
			continue
		}
		lines := splitNonEmptyLines(string(b))
		for _, line := range lines {
			var ev Event
			if err := json.Unmarshal([]byte(line), &ev); err != nil {
				continue
			}
			if ev.JobID != jobID {
				continue
			}
			switch ev.Type {
			case "job_execution_requested":
				sawRequested = true
			case "job_execution_skipped":
				sawSkipped = true
			case "job_execution_started":
				sawStarted = true
			}
		}
		if sawRequested && sawSkipped && !sawStarted {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	if !sawRequested || !sawSkipped {
		t.Fatalf("expected events for %q: requested=%v skipped=%v started=%v", jobID, sawRequested, sawSkipped, sawStarted)
	}
	if sawStarted {
		t.Fatalf("did not expect job_execution_started for %q (policy default_action=skip)", jobID)
	}

	// Ensure no JobStateRegistry artifact is created for skipped jobs.
	stateDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.StateDir)
	if entries, err := fileutil.ReadDir(stateDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if filepath.Ext(name) != ".yaml" {
				continue
			}
			if len(name) >= len(jobID)+1 && name[:len(jobID)+1] == jobID+"-" {
				t.Fatalf("unexpected persisted job state artifact for skipped job %q at %s", jobID, filepath.Join(stateDir, name))
			}
		}
	}
}
