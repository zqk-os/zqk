package system

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
)

func TestNewEnsureRetentionJobsCmd(t *testing.T) {
	t.Parallel()
	cmd := NewEnsureRetentionJobsCmd()
	if cmd == nil {
		t.Fatal("NewEnsureRetentionJobsCmd() returned nil")
	}
	if cmd.Use != "ensure-retention-jobs" {
		t.Errorf("expected Use \"ensure-retention-jobs\", got %q", cmd.Use)
	}
	if cmd.Short == emptyValue {
		t.Error("command should have a short description")
	}
}

func TestEnsureRetentionJobsCommandHasRunE(t *testing.T) {
	t.Parallel()
	cmd := NewEnsureRetentionJobsCmd()
	if cmd.RunE == nil {
		t.Error("ensure-retention-jobs command should have RunE")
	}
}

func TestEnsureRetentionJobsInProject_EmptyRoot(t *testing.T) {
	t.Parallel()
	logger := logging.GetLoggerFromProfile("test")
	result, err := EnsureRetentionJobsInProject("", logger, nil)
	if err == nil {
		t.Error("expected error when project root is empty")
	}
	if result != nil {
		t.Error("expected nil result when project root is empty")
	}
}

// TestEnsureRetentionJobsResult_HasCachePrewarmAndAutofixFields ensures the result struct
// includes cache_prewarm and autofix_batch_cleanup fields for JSON/YAML output.
func TestEnsureRetentionJobsResult_HasCachePrewarmAndAutofixFields(t *testing.T) {
	t.Parallel()
	r := &EnsureRetentionJobsResult{
		CachePrewarmJobCreated:        true,
		AutofixBatchCleanupJobCreated: true,
	}
	if !r.CachePrewarmJobCreated || !r.AutofixBatchCleanupJobCreated {
		t.Error("EnsureRetentionJobsResult must include CachePrewarmJobCreated and AutofixBatchCleanupJobCreated")
	}
}

func TestParseTemplateScheduleAndEnv(t *testing.T) {
	t.Parallel()
	y := `
schedule_expression: "0 */4 * * *"
environment_variables:
  FOO: "bar"
`
	sched, env, hasEnv, err := parseTemplateScheduleAndEnv([]byte(y))
	if err != nil {
		t.Fatal(err)
	}
	if sched != "0 */4 * * *" {
		t.Errorf("schedule: got %q", sched)
	}
	if !hasEnv || env == nil || env["FOO"] != "bar" {
		t.Errorf("env: %+v hasEnv=%v", env, hasEnv)
	}
	sched2, _, hasEnv2, err := parseTemplateScheduleAndEnv([]byte(`schedule_expression: "x"`))
	if err != nil {
		t.Fatal(err)
	}
	if sched2 != "x" || hasEnv2 {
		t.Errorf("no env: sched=%q hasEnv=%v", sched2, hasEnv2)
	}
}
