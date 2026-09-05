package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestMaybeRunOrchestrateRollupAfterTick_WriteJobOutcome_success(t *testing.T) {
	tmp := t.TempDir()
	old := runCVSOrchestrateRollupCmd
	defer func() { runCVSOrchestrateRollupCmd = old }()

	runCVSOrchestrateRollupCmd = func(ctx context.Context, root, sessionID string, rollupOut string) ([]byte, error) {
		path := ResolveCVSRollupLatestJSONPath(root, rollupOut)
		if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		body := `{"schema_version":"rollup_v1","parent_convergence_session_id":"` + sessionID + `","rollup_status":"satisfied","ready_for_parent_completion":true}`
		if err := fileutil.WriteSecureFile(path, []byte(body)); err != nil {
			t.Fatalf("write: %v", err)
		}
		return nil, nil
	}

	h := NewConvergenceSessionTickHandler(nil, tmp)
	job := &ScheduledJob{
		ID:      "SCH-tick-outcome-test",
		JobType: JobTypeConvergenceSessionTick,
		EnvironmentVariables: map[string]string{
			EnvKeyConvergenceTickRollup: "1",
		},
	}
	sid := "[REDACTED-ID]"
	h.MaybeRunOrchestrateRollupAfterTick(context.Background(), job, sid)

	raw, err := fileutil.ReadFile(JobEventsFilePath(tmp, job.ID))
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	var last map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &last); err != nil {
		t.Fatalf("parse last line: %v", err)
	}
	if last[KeyEventType] != jobLogEventTypeOutcome {
		t.Fatalf("event_type = %v", last[KeyEventType])
	}
	if last[OutcomeKeyConvergenceTickRollupOK] != true {
		t.Fatalf("rollup_ok = %v", last[OutcomeKeyConvergenceTickRollupOK])
	}
	if last[OutcomeKeyConvergenceTickRollupStatus] != "satisfied" {
		t.Fatalf("rollup_status = %v", last[OutcomeKeyConvergenceTickRollupStatus])
	}
}

func TestMaybeRunOrchestrateRollupAfterTick_WriteJobOutcome_failure(t *testing.T) {
	tmp := t.TempDir()
	old := runCVSOrchestrateRollupCmd
	defer func() { runCVSOrchestrateRollupCmd = old }()

	runCVSOrchestrateRollupCmd = func(ctx context.Context, root, sessionID string, _ string) ([]byte, error) {
		return []byte("orch failed"), fmt.Errorf("rollup subprocess failed")
	}

	h := NewConvergenceSessionTickHandler(nil, tmp)
	job := &ScheduledJob{
		ID:      "SCH-tick-outcome-fail",
		JobType: JobTypeConvergenceSessionTick,
		EnvironmentVariables: map[string]string{
			EnvKeyConvergenceTickRollup: "true",
		},
	}
	h.MaybeRunOrchestrateRollupAfterTick(context.Background(), job, "[REDACTED-ID]")

	raw, err := fileutil.ReadFile(JobEventsFilePath(tmp, job.ID))
	if err != nil {
		t.Fatalf("read events: %v", err)
	}
	var last map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(raw))), &last); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if last[OutcomeKeyConvergenceTickRollupOK] != false {
		t.Fatalf("want rollup_ok false, got %v", last[OutcomeKeyConvergenceTickRollupOK])
	}
	gotBC, _ := last[OutcomeKeyConvergenceTickRollupOutputByteCount].(float64)
	if int(gotBC) != len("orch failed") {
		t.Fatalf("output byte count: got %v want %d", last[OutcomeKeyConvergenceTickRollupOutputByteCount], len("orch failed"))
	}
}
