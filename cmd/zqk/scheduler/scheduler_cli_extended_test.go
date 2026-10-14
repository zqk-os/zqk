package scheduler

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func executeSchedulerCmd(t *testing.T, cmd *cobra.Command, args []string, projRoot string) (string, error) {
	t.Helper()
	root := &cobra.Command{Use: "zqk"}
	root.PersistentFlags().StringP("format", "f", "", "Output format")
	root.AddCommand(cmd)

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))

	cctx := cli.ContextForProjectRoot(projRoot)
	cctx.ProjectRoot = projRoot
	cli.SetContext(root, cctx)
	cli.SetContext(cmd, cctx)

	root.SetArgs(append([]string{cmd.Name()}, args...))
	err := root.Execute()
	return buf.String(), err
}

func TestSchedulerConfigCmd_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Initial config read
	out, err := executeSchedulerCmd(t, NewConfigCmd(), []string{"--format", "json"}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, `"jobs_paused": false`)

	// 2. Set jobs-paused
	out, err = executeSchedulerCmd(t, NewConfigCmd(), []string{"--jobs-paused", "--format", "json"}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, `"jobs_paused": true`)

	// 3. Clear jobs-paused with --no-jobs-paused
	out, err = executeSchedulerCmd(t, NewConfigCmd(), []string{"--no-jobs-paused", "--format", "json"}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, `"jobs_paused": false`)

	// 4. Conflicting flags error
	_, err = executeSchedulerCmd(t, NewConfigCmd(), []string{"--jobs-paused", "--no-jobs-paused"}, proj.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot set both --jobs-paused and --no-jobs-paused")
}

func TestSchedulerStateCmd_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Summarize state in text mode
	out, err := executeSchedulerCmd(t, NewStateCmd(), []string{}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "Scheduler state summary")

	// 2. Summarize state in JSON mode
	out, err = executeSchedulerCmd(t, NewStateCmd(), []string{"--format", "json"}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, `"total_files":`)

	// 3. Migrate flag
	out, err = executeSchedulerCmd(t, NewStateCmd(), []string{"--migrate"}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "Migrated")
}

func TestSchedulerDumpCmd_NotRunning(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	_, err := executeSchedulerCmd(t, NewDumpCmd(), []string{}, proj.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler daemon is not running")
}

func TestSchedulerSkipWindowCmd_Lifecycle(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Initial status (no window active)
	out, err := executeSchedulerCmd(t, NewSkipWindowCmd(), []string{"status"}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "no active skip_window")

	// 2. Set skip window for 15m with --match-all
	out, err = executeSchedulerCmd(t, NewSkipWindowCmd(), []string{"set", "--for", "15m", "--match-all"}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "skip_window active until")

	// 3. Status is now active
	out, err = executeSchedulerCmd(t, NewSkipWindowCmd(), []string{"status"}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "match_all: true")

	// 4. Clear skip window
	out, err = executeSchedulerCmd(t, NewSkipWindowCmd(), []string{"clear"}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "skip_window cleared")

	// 5. Conflicting flags error
	_, err = executeSchedulerCmd(t, NewSkipWindowCmd(), []string{"set", "--for", "10m", "--until", "2026-10-15T00:00:00Z"}, proj.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "use either --for or --until, not both")
}

func TestSchedulerServiceCmd_ControlAccess(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Unauthorized control attempt
	t.Setenv(zqkenv.APIKey().Name(), "unauthorized-key")
	t.Setenv(zqkenv.TestBypassAuth().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")

	_, err := executeSchedulerCmd(t, NewServiceCmd(), []string{"install"}, proj.Root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "scheduler service control requires")

	// 2. Service list command
	_, _ = executeSchedulerCmd(t, NewServiceCmd(), []string{"list"}, proj.Root) // Host dependent
}

func TestAuditEventExtractor_Methods(t *testing.T) {
	evt := map[string]any{
		"target_id":   "JOB-TEST-123",
		"target_kind": "scheduler_job",
		"event_type":  "scheduler_job_completed",
		"created_at":  "2026-10-14T20:00:00Z",
		"metadata": map[string]any{
			"duration_ms": 1500,
		},
	}

	fields := ExtractAuditEventFields(evt)
	require.NotNil(t, fields)
	assert.Equal(t, "JOB-TEST-123", fields.TargetID)
	assert.Equal(t, "scheduler_job", fields.TargetKind)
	assert.Equal(t, "scheduler_job_completed", fields.EventType)

	assert.True(t, fields.IsSchedulerJobEvent())
	assert.True(t, fields.IsSchedulerJobEventType())
	assert.True(t, fields.MatchesJobIDFilter(""))
	assert.True(t, fields.MatchesJobIDFilter("JOB-TEST-123"))
	assert.False(t, fields.MatchesJobIDFilter("JOB-OTHER"))
	assert.True(t, fields.IsCompletionEvent())
	assert.True(t, fields.IsCompletedEvent())
	assert.False(t, fields.IsFailedEvent())
	assert.False(t, fields.IsStartedEvent())
	assert.True(t, fields.ShouldProcess("JOB-TEST-123"))

	// Failed event test
	failedEvt := map[string]any{
		"target_id":   "JOB-FAIL-456",
		"target_kind": "scheduler_job",
		"event_type":  "scheduler_job_failed",
	}
	failedFields := ExtractAuditEventFields(failedEvt)
	assert.True(t, failedFields.IsFailedEvent())
	assert.True(t, failedFields.IsCompletionEvent())
	assert.False(t, failedFields.IsCompletedEvent())

	// Started event test
	startedEvt := map[string]any{
		"target_id":   "JOB-START-789",
		"target_kind": "scheduler_job",
		"event_type":  "scheduler_job_started",
	}
	startedFields := ExtractAuditEventFields(startedEvt)
	assert.True(t, startedFields.IsStartedEvent())
	assert.False(t, startedFields.IsCompletionEvent())
}

func TestFlagDuration_FallbackAndParsing(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Duration("duration-flag", 5*time.Second, "duration flag")
	cmd.Flags().String("string-flag", "10m", "string flag")
	cmd.Flags().String("empty-flag", "", "empty flag")
	cmd.Flags().String("invalid-flag", "not-a-duration", "invalid flag")

	// Duration flag lookup
	d1 := flagDuration(cmd, "duration-flag", time.Minute)
	assert.Equal(t, 5*time.Second, d1)

	// String flag lookup parsed as duration
	d2 := flagDuration(cmd, "string-flag", time.Minute)
	assert.Equal(t, 10*time.Minute, d2)

	// Empty string fallback
	d3 := flagDuration(cmd, "empty-flag", time.Minute)
	assert.Equal(t, time.Minute, d3)

	// Invalid string fallback
	d4 := flagDuration(cmd, "invalid-flag", time.Minute)
	assert.Equal(t, time.Minute, d4)

	// Non-existent flag fallback
	d5 := flagDuration(cmd, "missing-flag", time.Hour)
	assert.Equal(t, time.Hour, d5)
}

func TestDetachedDaemonStartArgs(t *testing.T) {
	args1 := detachedDaemonStartArgs("/usr/local/bin/zqk-scheduler")
	assert.Equal(t, []string{"start", "--foreground"}, args1)

	args2 := detachedDaemonStartArgs("/usr/local/bin/zqk")
	assert.Equal(t, []string{"scheduler", "start", "--foreground"}, args2)
}

func TestTestFailuresCommands_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Root command
	cmd := NewTestFailuresCmd()
	require.NotNil(t, cmd)

	// 2. Health command
	_, _ = executeSchedulerCmd(t, NewTestFailuresHealthCmd(), []string{}, proj.Root)

	// 3. Criteria evidence command
	_, _ = executeSchedulerCmd(t, NewTestFailuresCriteriaEvidenceCmd(), []string{}, proj.Root)

	// 4. List command
	_, _ = executeSchedulerCmd(t, NewTestFailuresListCmd(), []string{}, proj.Root)
}
