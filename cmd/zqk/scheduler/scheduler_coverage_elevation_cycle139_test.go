package scheduler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

type exitPanic int

func withExitPanic(fn func()) (exitCode int, err error) {
	origExit := healthCheckExit
	defer func() { healthCheckExit = origExit }()
	defer func() {
		if r := recover(); r != nil {
			if code, ok := r.(exitPanic); ok {
				exitCode = int(code)
			} else {
				panic(r)
			}
		}
	}()
	healthCheckExit = func(code int) {
		panic(exitPanic(code))
	}
	fn()
	return 0, nil
}

func runCycle139Cmd(t *testing.T, cmd *cobra.Command, args []string, projRoot string) (string, error) {
	t.Helper()
	return executeSchedulerCmd(t, cmd, args, projRoot)
}

func TestCycle139_HealthCheck_Lifecycle(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{ForceRemoveRootOnCleanup: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, 0755))
	pidPath := paths.SchedulerPIDFilePath(proj.Root)
	keepAlivePath := filepath.Join(schedDir, paths.SchedulerKeepAliveFile)
	require.NoError(t, fileutil.WriteSecureFile(pidPath, []byte("999999")))

	cmd := NewHealthCheckCmd()

	// 1. Not alive, no jobs -> exit code 0
	code1, err1 := withExitPanic(func() {
		_ = runHealthCheck(cmd, []string{})
	})
	require.NoError(t, err1)
	assert.Equal(t, 0, code1)

	// 2. Add timer job -> Not alive, with jobs -> exit code 1
	secCtx := pkgctx.NewSystemSecurityContext()
	jobObj := map[string]any{
		objects.FieldKeyID:                 "SCH-job-health-test",
		objects.FieldKeyKind:               schedulerKindJob,
		objects.FieldKeyTitle:              "Health Test Job",
		objects.FieldKeyStatus:             objects.ObjectStatusActive,
		objects.FieldKeyEnabled:            true,
		objects.FieldKeyTriggerType:        schedulerTriggerTimer,
		objects.FieldKeyScheduleExpression: "*/5 * * * *",
	}
	require.NoError(t, proj.FileStorage.Create(context.Background(), secCtx, jobObj))

	code2, err2 := withExitPanic(func() {
		_ = runHealthCheck(cmd, []string{})
	})
	require.NoError(t, err2)
	assert.Equal(t, 1, code2)

	// 3. Alive fresh -> exit code 0
	require.NoError(t, fileutil.WriteSecureFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid()))))
	require.NoError(t, fileutil.WriteSecureFile(keepAlivePath, []byte(time.Now().UTC().Format(time.RFC3339))))

	code3, err3 := withExitPanic(func() {
		_ = runHealthCheck(cmd, []string{})
	})
	require.NoError(t, err3)
	assert.Equal(t, 0, code3)

	// 4. Alive stale -> exit code 1
	staleTime := time.Now().UTC().Add(-10 * time.Minute)
	require.NoError(t, fileutil.WriteSecureFile(keepAlivePath, []byte(staleTime.Format(time.RFC3339))))

	code4, err4 := withExitPanic(func() {
		_ = runHealthCheck(cmd, []string{})
	})
	require.NoError(t, err4)
	assert.Equal(t, 1, code4)

	// 5. Unhealthy file cleaned when healthy
	writeDaemonUnhealthyFile(proj.Root, "test_reason", 0, "test message")
	unhealthyPath := filepath.Join(schedDir, "daemon-unhealthy.json")
	assert.True(t, fileutil.Exists(unhealthyPath))

	require.NoError(t, fileutil.WriteSecureFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid()))))
	require.NoError(t, fileutil.WriteSecureFile(keepAlivePath, []byte(time.Now().UTC().Format(time.RFC3339))))
	code5, err5 := withExitPanic(func() {
		_ = runHealthCheck(cmd, []string{})
	})
	require.NoError(t, err5)
	assert.Equal(t, 0, code5)
	assert.False(t, fileutil.Exists(unhealthyPath))
}

func TestCycle139_ServiceCommands_Authorized(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{ForceRemoveRootOnCleanup: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)
	t.Setenv(zqkenv.TestBypassAuth().Name(), "1")

	// 1. List
	outList, errList := runCycle139Cmd(t, NewServiceCmd(), []string{"list"}, proj.Root)
	require.NoError(t, errList)
	assert.Contains(t, outList, "[")

	// 2. List orphans
	outOrphans, errOrphans := runCycle139Cmd(t, NewServiceCmd(), []string{"list", "--orphans"}, proj.Root)
	require.NoError(t, errOrphans)
	assert.Contains(t, outOrphans, "[")

	// 3. GC
	outGC, errGC := runCycle139Cmd(t, NewServiceCmd(), []string{"gc"}, proj.Root)
	require.NoError(t, errGC)
	assert.Contains(t, outGC, "null")

	// 4. Rebind validation error
	_, errRebind := runCycle139Cmd(t, NewServiceCmd(), []string{"rebind"}, proj.Root)
	require.Error(t, errRebind)
	assert.Contains(t, errRebind.Error(), "--root-id and --new-path are required")
}

func TestCycle139_EscalateCommand_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{ForceRemoveRootOnCleanup: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Dry run warning escalation
	outDry, errDry := runCycle139Cmd(t, NewEscalateCmd(), []string{"--dry-run", "--title", "Cycle 139 Dry Run Alert"}, proj.Root)
	require.NoError(t, errDry)
	assert.Contains(t, outDry, "alert_title: Cycle 139 Dry Run Alert")
	assert.Contains(t, outDry, "dry_run: true")

	// 2. Critical severity escalation with test mode
	outCrit, errCrit := runCycle139Cmd(t, NewEscalateCmd(), []string{
		"--test",
		"--severity", "critical",
		"--title", "Cycle 139 Critical Test",
		"--issue", "Test issue 1",
		"--suggested-action", "Action 1",
		"--inbox-only",
	}, proj.Root)
	require.NoError(t, errCrit)
	assert.Contains(t, outCrit, "alert_title: Cycle 139 Critical Test")
	assert.Contains(t, outCrit, "delivery_state: success")
}

func TestCycle139_SkipWindow_ValidationErrors(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), tmpDir)

	// 1. Missing duration/until
	_, errEmpty := runCycle139Cmd(t, NewSkipWindowCmd(), []string{"set"}, tmpDir)
	require.Error(t, errEmpty)
	assert.Contains(t, errEmpty.Error(), "set --for (duration) or --until (RFC3339)")

	// 2. Invalid duration format
	_, errInvalidDur := runCycle139Cmd(t, NewSkipWindowCmd(), []string{"set", "--for", "invalid-duration"}, tmpDir)
	require.Error(t, errInvalidDur)
	assert.Contains(t, errInvalidDur.Error(), "parse --for duration")

	// 3. Invalid RFC3339 format
	_, errInvalidRFC := runCycle139Cmd(t, NewSkipWindowCmd(), []string{"set", "--until", "not-a-date"}, tmpDir)
	require.Error(t, errInvalidRFC)
	assert.Contains(t, errInvalidRFC.Error(), "parse --until as RFC3339")

	// 4. Past time error
	_, errPast := runCycle139Cmd(t, NewSkipWindowCmd(), []string{"set", "--until", "2020-01-01T00:00:00Z"}, tmpDir)
	require.Error(t, errPast)
	assert.Contains(t, errPast.Error(), "must end in the future")

	// 5. Missing match-all or filter flags
	_, errNoFilters := runCycle139Cmd(t, NewSkipWindowCmd(), []string{"set", "--for", "1h"}, tmpDir)
	require.Error(t, errNoFilters)
	assert.Contains(t, errNoFilters.Error(), "specify --match-all and/or --job-type")
}

func TestCycle139_CriteriaEvidence_Execution(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{ForceRemoveRootOnCleanup: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. When no events file exists yet
	outMissing, errMissing := runCycle139Cmd(t, NewTestFailuresCriteriaEvidenceCmd(), []string{}, proj.Root)
	require.NoError(t, errMissing)
	assert.Contains(t, outMissing, "No test-bundle events file yet")

	// 2. Create events.jsonl with sample verification evidence row
	eventsDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.LogsDir, "scheduler", "cvs", "test-bundles")
	require.NoError(t, fileutil.MkdirAll(eventsDir, 0755))

	eventsPath := filepath.Join(eventsDir, "events.jsonl")
	line := `{"event_type":"criteria_verification_evidence","timestamp":"2026-10-23T15:00:00Z","job_id":"SCH-test-01","criteria_refs":["CRIT-TEST-001"],"criteria_verification_satisfied":true}` + "\n"
	require.NoError(t, fileutil.WriteSecureFile(eventsPath, []byte(line)))

	outEv, errEv := runCycle139Cmd(t, NewTestFailuresCriteriaEvidenceCmd(), []string{}, proj.Root)
	require.NoError(t, errEv)
	assert.Contains(t, outEv, "CRIT-TEST-001")
	assert.Contains(t, outEv, "satisfied=true")
}

func TestCycle139_Submit_ErrorPaths(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), tmpDir)

	// 1. Missing arguments
	_, errArgs := runCycle139Cmd(t, NewSubmitCmd(), []string{}, tmpDir)
	require.Error(t, errArgs)
	assert.Contains(t, errArgs.Error(), "requires at least 1")

	// 2. Direct runSubmit without context
	cmd := &cobra.Command{Use: "submit"}
	errCtx := runSubmit(cmd, []string{})
	require.Error(t, errCtx)
	assert.Contains(t, errCtx.Error(), "requires at least 1")
}

func TestCycle139_Restart_ErrorPaths(t *testing.T) {
	cmd := &cobra.Command{Use: "restart"}
	err := runRestart(cmd, []string{})
	require.Error(t, err)
}

func TestCycle139_OutputHelpers_QueryCachedEventIDs(t *testing.T) {
	tmpDir := t.TempDir()
	found, ids := queryCachedEventIDs(tmpDir, time.Now().Add(-1*time.Hour), time.Now(), 10)
	assert.False(t, found)
	assert.Nil(t, ids)
}

func TestCycle139_MaskWebhookURL(t *testing.T) {
	assert.Equal(t, "", maskWebhookURL(""))
	assert.Equal(t, "********", maskWebhookURL("short"))
	assert.Equal(t, "https://hooks.slack.com/services/T123/B456/********", maskWebhookURL("https://hooks.slack.com/services/T123/B456/SECRET123"))
}

func TestCycle139_ServiceRootOrCwd(t *testing.T) {
	tmpDir := t.TempDir()
	assert.NotEmpty(t, serviceRootOrCwd(""))
	assert.NotEmpty(t, serviceRootOrCwd(tmpDir))
}

func TestCycle139_IssuesBundleHealth_Lifecycle(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{ForceRemoveRootOnCleanup: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Missing health file
	out1, err1 := runCycle139Cmd(t, NewIssuesBundleHealthCmd(), []string{}, proj.Root)
	require.NoError(t, err1)
	assert.Contains(t, out1, "test-bundles/health.jsonl not found")
	assert.Contains(t, out1, "issues.json: no open issues")

	// 2. Write mock issues.json and run again
	issuesPath := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir, "issues.json")
	issuesData := `{"status":"issues","issues":[{"job_id":"SCH-test-issue","job_type":"test-run","recorded_at":"2026-10-23T15:00:00Z"}]}`
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(issuesPath), 0755))
	require.NoError(t, fileutil.WriteSecureFile(issuesPath, []byte(issuesData)))

	out2, err2 := runCycle139Cmd(t, NewIssuesBundleHealthCmd(), []string{}, proj.Root)
	require.NoError(t, err2)
	assert.Contains(t, out2, "issues.json: status=issues")
	assert.Contains(t, out2, "SCH-test-issue")
}

func TestCycle139_RecordCvsOrchestrateRun_Validation(t *testing.T) {
	cmd := NewRecordCvsOrchestrateRunCmd()
	assert.NotNil(t, cmd)
	assert.Equal(t, "record-cvs-orchestrate-run", cmd.Name())

	// Stdin empty error
	err := runRecordCvsOrchestrateRun(cmd, []string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stdin is empty")
}

func TestCycle139_ListActivityHistory_Scenarios(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{ForceRemoveRootOnCleanup: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. List formats
	outListJSON, errListJSON := runCycle139Cmd(t, NewListCmd(), []string{"--format", "json"}, proj.Root)
	require.NoError(t, errListJSON)
	assert.Contains(t, outListJSON, "[")

	outListYAML, errListYAML := runCycle139Cmd(t, NewListCmd(), []string{"--format", "yaml"}, proj.Root)
	require.NoError(t, errListYAML)
	assert.Contains(t, outListYAML, "[]")

	_, errListTable := runCycle139Cmd(t, NewListCmd(), []string{"--format", "table"}, proj.Root)
	require.NoError(t, errListTable)

	// 2. Activity formats
	outActJSON, errActJSON := runCycle139Cmd(t, NewActivityCmd(), []string{"--format", "json"}, proj.Root)
	require.NoError(t, errActJSON)
	assert.Contains(t, outActJSON, `"busyness":`)

	outActYAML, errActYAML := runCycle139Cmd(t, NewActivityCmd(), []string{"--format", "yaml", "--compact"}, proj.Root)
	require.NoError(t, errActYAML)
	assert.Contains(t, outActYAML, "busyness:")

	outActTable, errActTable := runCycle139Cmd(t, NewActivityCmd(), []string{"--stuck-stale-after", "15m"}, proj.Root)
	require.NoError(t, errActTable)
	assert.Contains(t, outActTable, "Activity")

	// 3. History formats
	outHistJSON, errHistJSON := runCycle139Cmd(t, NewHistoryCmd(), []string{"--format", "json"}, proj.Root)
	require.NoError(t, errHistJSON)
	assert.Contains(t, outHistJSON, "No execution history found")

	outHistTable, errHistTable := runCycle139Cmd(t, NewHistoryCmd(), []string{"--format", "table"}, proj.Root)
	require.NoError(t, errHistTable)
	assert.Contains(t, outHistTable, "No execution history found")
}

func TestCycle139_EventsHealth_EmptyAndPopulated(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{ForceRemoveRootOnCleanup: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. Missing summary file
	outEmpty, errEmpty := runCycle139Cmd(t, NewEventsCmd(), []string{"health"}, proj.Root)
	require.NoError(t, errEmpty)
	assert.Contains(t, outEmpty, "No summary found")

	// 2. Populated summary file
	schedDir := filepath.Join(proj.Root, paths.ProjectDataDir, paths.SchedulerDir)
	require.NoError(t, fileutil.MkdirAll(schedDir, 0755))

	summaryPath := filepath.Join(schedDir, "scheduler-metrics-summary.json")
	summaryData := `{"updated_at":"2026-10-23T15:00:00Z","window_hours":24,"total_runs":10,"jobs":[{"job_id":"SCH-healthy-job","runs":5,"failures":0,"avg_duration_ms":100}]}`
	require.NoError(t, fileutil.WriteSecureFile(summaryPath, []byte(summaryData)))

	outPop, errPop := runCycle139Cmd(t, NewEventsCmd(), []string{"health"}, proj.Root)
	require.NoError(t, errPop)
	assert.Contains(t, outPop, "Health view")
	assert.Contains(t, outPop, "(none)")
}

func TestCycle139_TestFailures_Subcommands(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{ForceRemoveRootOnCleanup: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// 1. List with no logs
	outList, errList := runCycle139Cmd(t, NewTestFailuresCmd(), []string{"list"}, proj.Root)
	require.NoError(t, errList)
	assert.Contains(t, outList, "No test logs found")

	// 2. Rerun with no logs
	_, errRerun := runCycle139Cmd(t, NewTestFailuresCmd(), []string{"rerun"}, proj.Root)
	require.Error(t, errRerun)
	assert.Contains(t, errRerun.Error(), "no callback logs or test-bundle events found")

	// 3. Health with no health file
	outHealth, errHealth := runCycle139Cmd(t, NewTestFailuresCmd(), []string{"health"}, proj.Root)
	require.NoError(t, errHealth)
	assert.Contains(t, outHealth, "No test health file yet")

	// 4. Analyze with no persistent failures
	outAnalyze, errAnalyze := runCycle139Cmd(t, NewTestFailuresCmd(), []string{"analyze"}, proj.Root)
	require.NoError(t, errAnalyze)
	assert.Contains(t, outAnalyze, "No persistent test failures found")
}

func TestCycle139_AuditEventExtractor(t *testing.T) {
	evt := map[string]any{
		"target_id":   "SCH-job-test-1",
		"target_kind": getSchedulerJobKind(),
		"event_type":  schedulerJobCompletedEvent,
		"created_at":  "2026-10-23T12:00:00Z",
		"metadata": map[string]any{
			"success":          true,
			"duration_seconds": 1.25,
		},
	}
	fields := ExtractAuditEventFields(evt)
	require.NotNil(t, fields)
	assert.Equal(t, "SCH-job-test-1", fields.TargetID)
	assert.Equal(t, getSchedulerJobKind(), fields.TargetKind)
	assert.True(t, fields.IsSchedulerJobEvent())
	assert.True(t, fields.IsSchedulerJobEventType())
	assert.True(t, fields.MatchesJobIDFilter("SCH-job-test-1"))
	assert.True(t, fields.MatchesJobIDFilter(""))
	assert.False(t, fields.MatchesJobIDFilter("SCH-other-job"))
	assert.True(t, fields.IsCompletionEvent())
	assert.True(t, fields.IsCompletedEvent())
	assert.False(t, fields.IsFailedEvent())
	assert.False(t, fields.IsStartedEvent())
	assert.True(t, fields.ShouldProcess(""))
	assert.False(t, fields.ShouldProcess("SCH-other-job"))

	emptyFields := ExtractAuditEventFields(map[string]any{})
	require.NotNil(t, emptyFields)
	assert.False(t, emptyFields.IsSchedulerJobEvent())
	assert.Nil(t, emptyFields.Metadata)
}

func TestCycle139_AuditEventFieldsHelper(t *testing.T) {
	names := auditEventFieldNames()
	assert.NotEmpty(t, names)
	assert.Equal(t, "target_id", getAuditEventTargetIDField())
	assert.Equal(t, "target_kind", getAuditEventTargetKindField())
	assert.Equal(t, "event_type", getAuditEventEventTypeField())
	assert.Equal(t, "created_at", getAuditEventCreatedAtField())
	assert.Equal(t, "metadata", getAuditEventMetadataField())
	assert.Equal(t, "success", GetMetadataSuccessField())
	assert.Equal(t, "duration_seconds", GetMetadataDurationField())
	assert.Equal(t, "error", GetMetadataErrorField())
}

func TestCycle139_AutonomySyntheticTelemetry(t *testing.T) {
	tmpDir := t.TempDir()
	res1 := autonomyTelemetryJSON(tmpDir)
	assert.Contains(t, res1, `"available":true`)
	res2 := autonomyUnavailableTelemetryJSON()
	assert.Contains(t, res2, `"available":true`)
}

func TestCycle139_ConvergenceBacklogHelpers(t *testing.T) {
	// 1. backlogItemRefsFromCVS
	refsNil := backlogItemRefsFromCVS(nil)
	assert.Nil(t, refsNil)
	cvsStr := map[string]any{
		objects.FieldKeyBacklogItemRefs: []string{"BLI-1", "  ", "BLI-2"},
	}
	refsStr := backlogItemRefsFromCVS(cvsStr)
	assert.Equal(t, []string{"BLI-1", "BLI-2"}, refsStr)
	cvsAny := map[string]any{
		objects.FieldKeyBacklogItemRefs: []any{"BLI-3", 123, "BLI-4"},
	}
	refsAny := backlogItemRefsFromCVS(cvsAny)
	assert.Equal(t, []string{"BLI-3", "BLI-4"}, refsAny)

	// 2. isCriteriaRefToken
	assert.True(t, isCriteriaRefToken("CRIT-12345"))
	assert.True(t, isCriteriaRefToken("CRIT-abc-def"))
	assert.False(t, isCriteriaRefToken("CRIT-"))
	assert.False(t, isCriteriaRefToken("NOTCRIT-123"))
	assert.False(t, isCriteriaRefToken("CRIT-invalid!@#"))

	// 3. extractVerificationHintLines & verificationHintAcceptable
	hints := extractVerificationHintLines("go test ./pkg/scheduler/...")
	assert.Contains(t, hints, "go test ./pkg/scheduler/...")
	assert.True(t, verificationHintAcceptable("go test -v ./pkg/scheduler/..."))
	assert.False(t, verificationHintAcceptable("go test something_random_not_path"))
	assert.False(t, verificationHintAcceptable("invalid command"))
}

func TestCycle139_ConvergenceRecommendationHelpers(t *testing.T) {
	// 1. rollupStatusTrimmed
	assert.Equal(t, "", rollupStatusTrimmed(nil))
	assert.Equal(t, "", rollupStatusTrimmed(map[string]any{}))
	assert.Equal(t, "satisfied", rollupStatusTrimmed(map[string]any{"rollup_status": " satisfied "}))

	// 2. rollupBlockerCount
	assert.Equal(t, 0, rollupBlockerCount(nil))
	assert.Equal(t, 0, rollupBlockerCount(map[string]any{}))
	coreWithBlockers := map[string]any{
		"blockers": []any{"blocker 1", "blocker 2"},
	}
	assert.Equal(t, 2, rollupBlockerCount(coreWithBlockers))

	// 3. phaseRouterStringFromSuggested
	assert.Equal(t, "", phaseRouterStringFromSuggested(nil, "phase"))
	assert.Equal(t, "", phaseRouterStringFromSuggested(map[string]any{}, "phase"))
	sug := map[string]any{
		"phase_router": map[string]any{
			"phase": " c2_spec ",
		},
	}
	assert.Equal(t, "c2_spec", phaseRouterStringFromSuggested(sug, "phase"))
}

func TestCycle139_IDEPasteAutomation_Basics(t *testing.T) {
	// 1. Default tokens
	defTokens := defaultIDEPastePrefixTokens()
	assert.Equal(t, []string{idePasteTokenCmdY}, defTokens)

	// 2. IDE App Name
	assert.Equal(t, defaultIDEPasteApplicationName, idePasteApplicationName())
	t.Setenv(zqkenv.IDEPasteApp().Name(), "CustomIDE")
	assert.Equal(t, "CustomIDE", idePasteApplicationName())

	// 3. AppleScript quoted
	quoted := appleScriptQuoted(`My "Special" App`)
	assert.Equal(t, `"My \"Special\" App"`, quoted)
}

func TestCycle139_IDEPasteAutomation_TokenParsing(t *testing.T) {
	// 1. Valid tokens & aliases
	raw := "focus_chat,explorer,show_explorer,escape,cmd_l,cmd_y,option_cmd_e"
	parsed, err := parseIDEPastePrefixTokens(raw)
	require.NoError(t, err)
	assert.Len(t, parsed, 7)
	assert.Equal(t, idePasteTokenCmdY, parsed[0])
	assert.Equal(t, idePasteCmdShiftE, parsed[1])
	assert.Equal(t, idePasteCmdShiftE, parsed[2])
	assert.Equal(t, idePasteTokenEscape, parsed[3])
	assert.Equal(t, idePasteTokenCmdL, parsed[4])
	assert.Equal(t, idePasteTokenCmdY, parsed[5])
	assert.Equal(t, idePasteOptionCmdE, parsed[6])

	// 2. Invalid token error
	_, errInvalid := parseIDEPastePrefixTokens("invalid_token")
	require.Error(t, errInvalid)
	assert.Contains(t, errInvalid.Error(), "invalid")

	// 3. Empty after parse
	_, errEmpty := parseIDEPastePrefixTokens("")
	require.Error(t, errEmpty)
}

func TestCycle139_IDEPasteAutomation_LoadAutomation(t *testing.T) {
	// 1. Default load
	t.Setenv(zqkenv.IDEPastePrefixSteps().Name(), "")
	autoDef, errDef := loadIDEPasteAutomation()
	require.NoError(t, errDef)
	require.NotNil(t, autoDef)
	assert.Contains(t, autoDef.appleScript, `tell application "Cursor" to activate`)
	assert.Contains(t, autoDef.osascriptBeginDetail, "steps=activate")
	assert.Contains(t, autoDef.keystrokesSummary, "activate_ide")
	assert.Contains(t, autoDef.prefixStepsHumanShort, "⌘Y")

	// 2. Builtin / default alias
	t.Setenv(zqkenv.IDEPastePrefixSteps().Name(), "builtin")
	autoBuiltin, errBuiltin := loadIDEPasteAutomation()
	require.NoError(t, errBuiltin)
	require.NotNil(t, autoBuiltin)

	t.Setenv(zqkenv.IDEPastePrefixSteps().Name(), "default")
	autoDefAlias, errDefAlias := loadIDEPasteAutomation()
	require.NoError(t, errDefAlias)
	require.NotNil(t, autoDefAlias)

	// 3. Custom token sequence
	t.Setenv(zqkenv.IDEPastePrefixSteps().Name(), "cmd_y,escape,cmd_l,cmd_shift_e,option_cmd_e")
	autoCust, errCust := loadIDEPasteAutomation()
	require.NoError(t, errCust)
	require.NotNil(t, autoCust)
	assert.Contains(t, autoCust.prefixStepsHumanShort, "⌘Y")
	assert.Contains(t, autoCust.prefixStepsHumanShort, "Escape")
	assert.Contains(t, autoCust.prefixStepsHumanShort, "⌘L")
	assert.Contains(t, autoCust.prefixStepsHumanShort, "⌥⌘E")
}

func TestCycle139_IDEPasteAutomation_HumanShortAndValidation(t *testing.T) {
	// 1. Multiple option_cmd_e count rendering
	tokens1 := []string{idePasteOptionCmdE}
	assert.Equal(t, "⌥⌘E", humanShortIDEPasteTokens(tokens1))

	tokens2 := []string{idePasteOptionCmdE, idePasteOptionCmdE}
	assert.Equal(t, "⌥⌘E×2", humanShortIDEPasteTokens(tokens2))

	tokens3 := []string{idePasteOptionCmdE, idePasteOptionCmdE, idePasteOptionCmdE}
	assert.Equal(t, "⌥⌘E×3", humanShortIDEPasteTokens(tokens3))

	// 2. validateBuiltIDEPasteScript error detection
	script := `tell application "Cursor" to activate`
	tokens := []string{idePasteTokenCmdY}
	errVal := validateBuiltIDEPasteScript(script, tokens)
	require.Error(t, errVal)
	assert.Contains(t, errVal.Error(), "cmd_y keystroke count mismatch")
}

func TestCycle139_BundleProgress_PopulatedEvents(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{ForceRemoveRootOnCleanup: true})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	progPath := schedpkg.TestBundlesProgressFilePath(proj.Root)
	require.NoError(t, fileutil.MkdirAll(filepath.Dir(progPath), 0755))

	events := `{"job_id":"SCH-run-001","event_type":"started","timestamp":"2026-10-23T12:00:00Z"}
{"job_id":"SCH-run-001","event_type":"passed","timestamp":"2026-10-23T12:01:00Z"}
{"job_id":"SCH-run-002","event_type":"started","timestamp":"2026-10-23T12:05:00Z"}
{"job_id":"SCH-run-002","event_type":"failed","timestamp":"2026-10-23T12:06:00Z"}
{"job_id":"SCH-run-003","event_type":"pending","timestamp":"2026-10-23T12:10:00Z"}
`
	require.NoError(t, fileutil.WriteSecureFile(progPath, []byte(events)))

	out, err := runCycle139Cmd(t, NewSchedulerBundleProgressCmd(), []string{}, proj.Root)
	require.NoError(t, err)
	assert.Contains(t, out, "Bundle Progress Matrix:")
	assert.Contains(t, out, "SCH-run-001")
	assert.Contains(t, out, "SCH-run-002")
	assert.Contains(t, out, "SCH-run-003")
	assert.Contains(t, out, "Summary:")
}

func TestCycle139_SchedulerCoordination_Emits(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()
	emitSchedulerStartEventViaCoordinator(ctx, tmpDir, nil, "start msg", "human", nil)
	emitSchedulerStopEventViaCoordinator(ctx, tmpDir, nil, "stop msg", 1234, "human", nil)
	emitSchedulerTriggerEventViaCoordinator(ctx, tmpDir, nil, "trig msg", "SCH-1", "human", nil)
	emitSchedulerWarningEventViaCoordinator(ctx, tmpDir, nil, "warn msg", fmt.Errorf("warn err"), "human")
	emitSchedulerDumpEventViaCoordinator(ctx, tmpDir, nil, "started", nil, "human", nil)
	emitSchedulerDumpEventViaCoordinator(ctx, "", nil, "started", nil, "human", nil)
}

func TestCycle139_ConvergenceOverseer_ValidationErrors(t *testing.T) {
	tmpDir := t.TempDir()
	convCmd := NewSchedulerConvergenceCmd()
	_, err := runCycle139Cmd(t, convCmd, []string{"overseer"}, tmpDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--coordinator-session-id is required")
}
