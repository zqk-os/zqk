package scheduler

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// CRIT-9040: ensure Scheduler integrates coordination kernel by persisting state
// and publishing job execution events during a real TriggerJob run.
func TestCRIT9040_SchedulerIntegration_PersistsStateAndPublishesEvents(t *testing.T) {
	t.Parallel()

	sched, testRoot, cleanup := setupTestScheduler(t)
	defer cleanup()

	// Set security context with read + execute permissions.
	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"developer"}, []string{"read:scheduler_job", "execute:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	jobID := "SCH-9040-INTEGRATION"
	createTestJob(t, sched.storage, jobID, "lifecycle_check", "manual", "")

	ctx := pkgctx.NewSystemContext()
	if err := sched.loadAndScheduleJobs(ctx, false); err != nil {
		t.Fatalf("Failed to load jobs: %v", err)
	}

	opCB := &schedulerTestOpCallback{done: make(chan error, 1)}
	if err := sched.TriggerJobWithCallback(ctx, jobID, concurrency.OperationCallback(opCB)); err != nil {
		t.Fatalf("Failed to trigger job: %v", err)
	}

	select {
	case cbErr := <-opCB.done:
		if cbErr != nil {
			t.Fatalf("job execution failed: %v", cbErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for job execution callback")
	}

	waitForJobFinalized(t, sched, jobID, 5*time.Second)

	// 1) JobStateRegistry artifact exists and is completed.
	// State may be nested: state/<job_id_segment>/<execution_id>.yaml (see job_state_registry.go)
	// or legacy flat: state/<job_id>-<execution_id>.yaml.
	stateDir := filepath.Join(testRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.StateDir)
	entries, err := fileutil.ReadDir(stateDir)
	if err != nil {
		t.Fatalf("expected job state dir %q to exist: %v", stateDir, err)
	}

	var foundCompleted bool
	tryFile := func(path string) {
		if foundCompleted {
			return
		}
		b, readErr := fileutil.ReadFile(path)
		if readErr != nil {
			return
		}
		var stY JobExecutionState
		if err := yaml.Unmarshal(b, &stY); err != nil {
			return
		}
		if stY.JobID == jobID && stY.State == jobExecutionStateCompleted && stY.CompletedAt != nil {
			foundCompleted = true
		}
	}

	for _, base := range jobStateDirsForLookup(stateDir, jobID) {
		sub, rerr := fileutil.ReadDir(base)
		if rerr != nil {
			continue
		}
		for _, se := range sub {
			if se.IsDir() || !strings.HasSuffix(se.Name(), ".yaml") {
				continue
			}
			if isReservedStateEntry(se.Name()) {
				continue
			}
			tryFile(filepath.Join(base, se.Name()))
		}
	}
	if !foundCompleted {
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
				continue
			}
			if isReservedStateEntry(e.Name()) {
				continue
			}
			tryFile(filepath.Join(stateDir, e.Name()))
		}
	}
	if !foundCompleted {
		t.Fatalf("expected completed JobExecutionState persisted for %q", jobID)
	}

	// 2) CoordinationChannel event log contains started + completed for the same execution.
	eventsLogPath := filepath.Join(testRoot, paths.ProjectDataDir, paths.SchedulerDir, "events", "coordination-bus.jsonl")
	evBytes, err := fileutil.ReadFile(eventsLogPath)
	if err != nil {
		t.Fatalf("expected event log %q to exist: %v", eventsLogPath, err)
	}

	lines := splitNonEmptyLines(string(evBytes))
	var startedExecID string
	var sawStarted, sawCompleted bool
	for _, line := range lines {
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.JobID != jobID {
			continue
		}
		switch ev.Type {
		case "job_execution_started":
			sawStarted = true
			startedExecID = ev.ExecutionID
		case "job_execution_completed":
			sawCompleted = true
			if startedExecID != emptyValue && ev.ExecutionID != startedExecID {
				t.Fatalf("execution_id mismatch: started=%q completed=%q", startedExecID, ev.ExecutionID)
			}
		}
	}

	if !sawStarted || !sawCompleted {
		t.Fatalf("expected job_execution_started and job_execution_completed events for %q (started=%v completed=%v)", jobID, sawStarted, sawCompleted)
	}
}

func splitNonEmptyLines(s string) []string {
	parts := strings.Split(s, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == emptyValue {
			continue
		}
		out = append(out, p)
	}
	return out
}
