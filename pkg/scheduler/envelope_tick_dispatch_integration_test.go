package scheduler

import (
	"context"
	"testing"

	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// TestDispatchEnvelopeTickResolvedJobs_onModeTriggersRegisteredJob verifies ENVELOPE_TICK_DISPATCH_MODE=on
// reaches TriggerJob for a resolved allowlisted job_type backed by an enabled scheduler_job (execution depth).
// Uses setupSchedulerCompleteTestEnvironment — do not call t.Parallel (ZQK_TEST_ROOT).
func TestDispatchEnvelopeTickResolvedJobs_onModeTriggersRegisteredJob(t *testing.T) {
	env := setupSchedulerCompleteTestEnvironment(t, nil)
	defer env.Cleanup()

	storagepkg.BuildPathAliasCacheForProject(env.TestRoot)

	storage := env.Storage.(storagepkg.ObjectStorageProvider)
	sched := NewSchedulerWithProjectRoot(storage, env.SpecLoader, env.LifecycleLoader, env.TestRoot, nil)

	jobID := "SCH-envelope-dispatch-on-test"
	createTestJob(t, storage, jobID, JobTypeLifecycleCheck, TriggerTypeManual, "")
	if err := sched.ReloadJobs(context.Background()); err != nil {
		t.Fatalf("reload jobs: %v", err)
	}

	envJob := &ScheduledJob{
		ID: "SCH-dce-tick",
		EnvironmentVariables: map[string]string{
			EnvKeyEnvelopeTickDispatchMode: envelopeTickDispatchModeOn,
		},
	}
	out := sched.DispatchEnvelopeTickResolvedJobs(context.Background(), envJob, []string{JobTypeLifecycleCheck})
	if out.Mode != envelopeTickDispatchModeOn {
		t.Fatalf("mode: %#v", out)
	}
	if out.TriggeredOK != 1 || out.TriggerFailed != 0 {
		t.Fatalf("want TriggeredOK=1 TriggerFailed=0; got %#v", out)
	}
}
