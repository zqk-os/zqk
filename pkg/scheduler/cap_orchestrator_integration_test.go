package scheduler

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TestCapOrchestrator_Integration verifies the cap_orchestrator job lifecycle
// inside a full scheduler instance. It addresses core-backlog.
func TestCapOrchestrator_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	sched, testRoot, cleanup := setupTestScheduler(t)
	defer cleanup()

	// Set security context with read + execute permissions.
	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"developer"}, []string{"read:scheduler_job", "execute:scheduler_job"})
	sched.SetSecurityContext(secCtx)

	jobID := "SCH-CAP-INTEGRATION"
	createTestJob(t, sched.storage, jobID, "cap_orchestrator", "manual", "")

	// Create a mock zqk binary to intercept the subprocess calls
	tempDir := t.TempDir()
	mockZqk := filepath.Join(tempDir, "zqk")
	script := `#!/bin/bash
if [[ "$1" == "workflow" && "$2" == "whats-next" ]]; then
	cat <<EOF
{"agent_instruction":"wait"}
EOF
	exit 0
fi
exit 0
`
	if err := fileutil.WriteFile(mockZqk, []byte(script), paths.DirPerm755); err != nil {
		t.Fatalf("failed to write mock zqk: %v", err)
	}

	// Override PATH so resolveCLIExecutable finds our mock zqk
	oldPath := zqkenv.OSPath().Get()
	t.Setenv(zqkenv.OSPath().Name(), tempDir+string(fileutil.PathListSeparator)+oldPath)

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
}
