package app

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clitool "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestTelemetry_RootCommandFlagsAndInstrumentation(t *testing.T) {
	root := NewRootCommand()
	require.NotNil(t, root)

	// Verify telemetry and operational flags
	timeoutFlag := root.PersistentFlags().Lookup("timeout")
	require.NotNil(t, timeoutFlag, "--timeout must be registered")
	assert.Equal(t, "duration", timeoutFlag.Value.Type())

	formatFlag := root.PersistentFlags().Lookup("format")
	require.NotNil(t, formatFlag, "--format must be registered")

	contextFlag := root.PersistentFlags().Lookup("context")
	require.NotNil(t, contextFlag, "--context must be registered")

	testIDFlag := root.PersistentFlags().Lookup("test-id")
	require.NotNil(t, testIDFlag, "--test-id must be registered")
	assert.True(t, testIDFlag.Hidden)

	// Profiling flags
	for _, prof := range []string{"cpuprofile", "cpu-profile", "goroutine-profile", "goroutines-profile"} {
		f := root.PersistentFlags().Lookup(prof)
		require.NotNil(t, f, "profiling flag %s must be present", prof)
		assert.True(t, f.Hidden)
	}

	// Verify subcommands inherit these flags
	for _, child := range root.Commands() {
		assert.NotNil(t, child.Flag("timeout"), "subcommand %s must inherit --timeout", child.Name())
		assert.NotNil(t, child.Flag("format"), "subcommand %s must inherit --format", child.Name())
	}
}

func TestTelemetry_SystemCheckAndAggregatorCommands(t *testing.T) {
	root := NewRootCommand()
	require.NotNil(t, root)

	// System check command
	sysCheckCmd, _, err := root.Find([]string{"system", "check"})
	require.NoError(t, err)
	require.NotNil(t, sysCheckCmd)
	assert.Equal(t, "check", sysCheckCmd.Name())

	// Reports command
	sysReportsCmd, _, err := root.Find([]string{"system", "reports"})
	require.NoError(t, err)
	require.NotNil(t, sysReportsCmd)
	assert.Equal(t, "reports", sysReportsCmd.Name())

	// Pre-commit command
	sysPrecommitCmd, _, err := root.Find([]string{"system", "pre-commit"})
	require.NoError(t, err)
	require.NotNil(t, sysPrecommitCmd)
	assert.Equal(t, "pre-commit", sysPrecommitCmd.Name())

	// Top-level telemetry & engine commands
	for _, cmdName := range []string{"healthchk", "explain", "do", "inspect", "grep", "query", "mutate"} {
		targetCmd, _, err := root.Find([]string{cmdName})
		require.NoError(t, err, "command %s should resolve", cmdName)
		require.NotNil(t, targetCmd, "command %s should not be nil", cmdName)
		assert.Equal(t, cmdName, targetCmd.Name())
	}
}

func TestTelemetry_ValidationTriggerBatchingComprehensive(t *testing.T) {
	// 1. Empty project root does not trigger anything
	var triggeredCount int
	restore := SetValidationTriggerFuncForTest(func(ctx context.Context, eventType, eventKind string, data map[string]any) error {
		triggeredCount++
		return nil
	})
	defer restore()

	AddValidationTrigger("", "backlog_item", "BLI-1", "create")
	assert.Equal(t, 0, triggeredCount)

	// 2. Skipped kinds (e.g. audit_event) do not trigger
	AddValidationTrigger("/tmp/some-project", objects.KindAuditEvent, "AUD-1", "create")
	assert.Equal(t, 0, triggeredCount)

	// 3. Batching up to validationBatchMaxSize (20) triggers single batch
	projectRoot := filepath.Join(t.TempDir(), "proj-batch-test")
	var capturedEventType, capturedEventKind string
	var capturedData map[string]any

	restore2 := SetValidationTriggerFuncForTest(func(ctx context.Context, eventType, eventKind string, data map[string]any) error {
		triggeredCount++
		capturedEventType = eventType
		capturedEventKind = eventKind
		capturedData = data
		return nil
	})
	defer restore2()

	for i := 0; i < validationBatchMaxSize-1; i++ {
		AddValidationTrigger(projectRoot, "task", fmt.Sprintf("TASK-%d", i), "update")
	}
	// 19 items: not flushed yet
	assert.Equal(t, 0, triggeredCount)

	// 20th item: triggers immediate flush
	AddValidationTrigger(projectRoot, "task", "TASK-19", "update")
	assert.Equal(t, 1, triggeredCount)
	assert.Equal(t, schedulerpkg.ObjectValidationEventType, capturedEventType)
	assert.Equal(t, validationEventKindBatch, capturedEventKind)

	itemsRaw, ok := capturedData[validationEventKeyItems].([]any)
	require.True(t, ok)
	assert.Len(t, itemsRaw, validationBatchMaxSize)

	firstItem, ok := itemsRaw[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "task", firstItem[validationItemKeyKind])
	assert.Equal(t, "TASK-0", firstItem[validationItemKeyID])
	assert.Equal(t, "update", firstItem[validationItemKeyOp])

	// 4. Manual flushValidationBatch on unknown root
	flushValidationBatch("unknown-root-123")
	assert.Equal(t, 1, triggeredCount)

	// 5. Add 1 item, then manually call flushValidationBatch
	AddValidationTrigger(projectRoot, "task", "TASK-MANUAL", "delete")
	assert.Equal(t, 1, triggeredCount)
	flushValidationBatch(projectRoot)
	assert.Equal(t, 2, triggeredCount)

	// Calling flush again on empty batch does not re-trigger
	flushValidationBatch(projectRoot)
	assert.Equal(t, 2, triggeredCount)
}

func TestTelemetry_AuditCoordinationAndMetadata(t *testing.T) {
	now := time.Now()
	metric := &clitool.CommandMetric{
		Command:        "test-cmd",
		NormalizedCmd:  "test_cmd",
		Args:           []string{"arg1", "arg2"},
		Flags:          map[string]any{"force": true},
		Duration:       250 * time.Millisecond,
		StartTime:      now,
		EndTime:        now.Add(250 * time.Millisecond),
		Success:        false,
		Error:          "simulated failure",
		ObjectsCreated: []string{"OBJ-NEW"},
		ObjectsUpdated: []string{"OBJ-UPD"},
		ObjectsDeleted: []string{"OBJ-DEL"},
		PriorityPlan:   "PRI-PLAN-1",
		Workstream:     "WS-CORE",
		Milestone:      "MLS-1",
		ActorID:        "ACC-DEV-001",
		ActorRoles:     []string{"core_engineer"},
	}

	// buildOperationDescription
	opDesc := buildOperationDescription(metric)
	assert.Equal(t, "Command execution: test-cmd arg1 arg2", opDesc)

	// determineSeverity
	assert.Equal(t, auditSeverityHigh, determineSeverity(metric))
	metricSuccess := &clitool.CommandMetric{Success: true}
	assert.Equal(t, auditSeverityLow, determineSeverity(metricSuccess))
	metricDeleted := &clitool.CommandMetric{Success: true, ObjectsDeleted: []string{"X"}}
	assert.Equal(t, auditSeverityHigh, determineSeverity(metricDeleted))
	metricCreated := &clitool.CommandMetric{Success: true, ObjectsCreated: []string{"Y"}}
	assert.Equal(t, auditSeverityMedium, determineSeverity(metricCreated))

	// buildAuditMetadata
	meta := buildAuditMetadata("/test/project", metric)
	assert.Equal(t, "cli", meta[auditMetaSource])
	assert.Equal(t, "/test/project", meta[auditMetaProjectRoot])
	assert.Equal(t, "test-cmd", meta[auditMetaCommand])
	assert.Equal(t, int64(250), meta[auditMetaDurationMs])
	assert.Equal(t, "simulated failure", meta[auditMetaError])

	// emitCommandExecutionEventViaCoordinator early returns
	ctx := context.Background()
	emitCommandExecutionEventViaCoordinator(ctx, "", nil, opDesc, auditSeverityHigh, meta, metric, profileHuman)
	emitCommandExecutionEventViaCoordinator(ctx, ".", nil, opDesc, auditSeverityHigh, meta, metric, profileHuman)

	// emit with valid temp project
	proj := testkit.PrepareIsolatedTempProject(t, nil)
	defer func() {
		if proj.FileStorage != nil {
			_ = proj.FileStorage.Shutdown(context.Background())
		}
	}()
	emitCommandExecutionEventViaCoordinator(ctx, proj.Root, proj.FileStorage, opDesc, auditSeverityHigh, meta, metric, profileSystem)
}

func TestTelemetry_RootHooksAndLifecycles(t *testing.T) {
	// argsContainHelpOrVersion
	assert.True(t, argsContainHelpOrVersion([]string{"--help"}))
	assert.True(t, argsContainHelpOrVersion([]string{"-h"}))
	assert.True(t, argsContainHelpOrVersion([]string{"help"}))
	assert.True(t, argsContainHelpOrVersion([]string{"--version"}))
	assert.True(t, argsContainHelpOrVersion([]string{"-v"}))
	assert.True(t, argsContainHelpOrVersion([]string{"version"}))
	assert.False(t, argsContainHelpOrVersion([]string{"inspect", "--all"}))
	assert.False(t, argsContainHelpOrVersion(nil))

	// formatReminderDuration
	assert.Equal(t, "45s", formatReminderDuration(45*time.Second))
	assert.Equal(t, "30m", formatReminderDuration(30*time.Minute))
	assert.Equal(t, "1.5h", formatReminderDuration(90*time.Minute))

	// isMutatingCommand
	cmdCreate := &cobra.Command{Use: "create"}
	assert.True(t, isMutatingCommand(cmdCreate))
	cmdUpdate := &cobra.Command{Use: "update"}
	assert.True(t, isMutatingCommand(cmdUpdate))
	cmdDelete := &cobra.Command{Use: "delete"}
	assert.True(t, isMutatingCommand(cmdDelete))
	cmdWorkflow := &cobra.Command{Use: "workflow"}
	assert.True(t, isMutatingCommand(cmdWorkflow))
	cmdSystem := &cobra.Command{Use: "system"}
	assert.True(t, isMutatingCommand(cmdSystem))

	childOfCreate := &cobra.Command{Use: "task"}
	cmdCreate.AddCommand(childOfCreate)
	assert.True(t, isMutatingCommand(childOfCreate))

	cmdGet := &cobra.Command{Use: "get"}
	assert.False(t, isMutatingCommand(cmdGet))

	// checkCLIReminderInRoot
	checkCLIReminderInRoot("")
	tmpDir := t.TempDir()
	checkCLIReminderInRoot(tmpDir) // flag file doesn't exist, clean no-op

	// Create fresh flag file
	flagDir := filepath.Join(tmpDir, paths.ProjectDataDir)
	require.NoError(t, fileutil.MkdirAll(flagDir, 0755))
	flagFile := filepath.Join(flagDir, "cli_reminder.flag")
	require.NoError(t, fileutil.WriteFile(flagFile, []byte("IMPORTANT REMINDER"), 0644))
	checkCLIReminderInRoot(tmpDir)

	// rootPersistentPostRunE help and version branches
	assert.NoError(t, rootPersistentPostRunE(&cobra.Command{Use: "help"}, nil))
	assert.NoError(t, rootPersistentPostRunE(&cobra.Command{Use: "version"}, nil))

	// rootPersistentPostRunE with nil tracker
	cmdStatus := &cobra.Command{Use: "status"}
	cmdStatus.SetContext(context.Background())
	assert.NoError(t, rootPersistentPostRunE(cmdStatus, nil))

	// rootPersistentPostRunE with tracker
	tracker := clitool.NewCommandExecutionTracker()
	ctx := clitool.WithTracker(context.Background(), tracker)
	cmdStatus.SetContext(ctx)
	assert.NoError(t, rootPersistentPostRunE(cmdStatus, nil))
}

func TestTelemetry_SessionAndContextHelpers(t *testing.T) {
	// Session context
	ctx := context.Background()
	assert.Empty(t, GetZqkSessionIDFromContext(ctx))

	ctxWithSession := WithZqkSessionID(ctx, "ZS-TEST-42")
	assert.Equal(t, "ZS-TEST-42", GetZqkSessionIDFromContext(ctxWithSession))

	// GetSessionIdleTimeout on empty/temp root
	tmpDir := t.TempDir()
	timeout := GetSessionIdleTimeout(tmpDir)
	assert.GreaterOrEqual(t, timeout, time.Duration(0))

	// Max OS threads
	applyMaxOSThreads()
}

func TestTelemetry_CompletionBuilder(t *testing.T) {
	root := &cobra.Command{Use: "zqk"}
	shells := DefaultShells()
	require.Len(t, shells, 3)

	argsBuilder := NewCompletionArgsBuilder(shells)
	validArgs := argsBuilder.ValidArgs()
	assert.ElementsMatch(t, []string{"bash", "zsh", "fish"}, validArgs)
	byName := argsBuilder.ShellByName()
	assert.Contains(t, byName, "bash")
	assert.Contains(t, byName, "zsh")
	assert.Contains(t, byName, "fish")

	builder := NewCompletionBuilder("zqk", root).WithShells(shells)
	compCmd := builder.Build()
	require.NotNil(t, compCmd)
	assert.Equal(t, "completion [bash|zsh|fish]", compCmd.Use)
	assert.ElementsMatch(t, []string{"bash", "zsh", "fish"}, compCmd.ValidArgs)

	// Execute completion for bash
	var buf bytes.Buffer
	compCmd.SetOut(&buf)
	compCmd.SetArgs([]string{"bash"})
	err := compCmd.Execute()
	assert.NoError(t, err)

	// Execute completion for zsh
	buf.Reset()
	compCmd.SetOut(&buf)
	compCmd.SetArgs([]string{"zsh"})
	err = compCmd.Execute()
	assert.NoError(t, err)

	// Execute completion for fish
	buf.Reset()
	compCmd.SetOut(&buf)
	compCmd.SetArgs([]string{"fish"})
	err = compCmd.Execute()
	assert.NoError(t, err)

	// NewCompletionCmd
	rootWithComp := NewCompletionCmd(root)
	assert.NotNil(t, rootWithComp)
}

func TestTelemetry_AuthMiddleware(t *testing.T) {
	// Builtin commands bypass auth
	for _, name := range []string{"help", "version", "completion", "quickstart", "start-here"} {
		cmd := &cobra.Command{Use: name}
		assert.NoError(t, AuthMiddleware(cmd, "/tmp/root"))
	}

	// Test bypass flag
	t.Setenv("ZQK_TEST_BYPASS_AUTH", "1")
	cmd := &cobra.Command{Use: "status"}
	assert.NoError(t, AuthMiddleware(cmd, "/tmp/root"))
	sec := pkgctx.GetSecurityContext(cmd.Context())
	assert.NotNil(t, sec)

	// Test codegen env bypass
	t.Setenv("ZQK_TEST_BYPASS_AUTH", "0")
	t.Setenv("ZQK_DEV_CODEGEN", "1")
	cmd2 := &cobra.Command{Use: "status"}
	assert.NoError(t, AuthMiddleware(cmd2, "/tmp/root"))
}

func TestTelemetry_DraftSweepAuth(t *testing.T) {
	// IsDraftSweepCommand
	assert.True(t, IsDraftSweepCommand("draft-sweep"))
	assert.True(t, IsDraftSweepCommand("sweep"))
	assert.True(t, IsDraftSweepCommand("tpm-groom"))
	assert.True(t, IsDraftSweepCommand("tpm-draft-something"))
	assert.False(t, IsDraftSweepCommand("create"))
	assert.False(t, IsDraftSweepCommand("inspect"))

	// CheckDraftSweepEntitlement nil context
	assert.Error(t, CheckDraftSweepEntitlement(nil))

	// System account allowed
	secSys := &pkgctx.SecurityContext{AccountID: pkgctx.SystemAccountID}
	assert.NoError(t, CheckDraftSweepEntitlement(secSys))

	// Test harness account allowed
	secTest := &pkgctx.SecurityContext{AccountID: pkgctx.TestHarnessAccountID}
	assert.NoError(t, CheckDraftSweepEntitlement(secTest))

	// Agent session token allowed
	secAgent := &pkgctx.SecurityContext{AccountID: accAgentSessionToken}
	assert.NoError(t, CheckDraftSweepEntitlement(secAgent))

	// Context with exact permission
	secPerm := &pkgctx.SecurityContext{
		AccountID:   "dev-1",
		Permissions: []string{PermDraftSweep},
	}
	assert.NoError(t, CheckDraftSweepEntitlement(secPerm))

	// Context with vocabulary marker
	secVocab := &pkgctx.SecurityContext{
		AccountID:               "dev-2",
		ActiveVocabularySchemes: []string{RBAC_SweepEntitated},
	}
	assert.NoError(t, CheckDraftSweepEntitlement(secVocab))

	// Context with persona marker
	secPersona := &pkgctx.SecurityContext{
		AccountID: "dev-3",
		PersonaID: "tpm_sweeper",
	}
	assert.NoError(t, CheckDraftSweepEntitlement(secPersona))

	// Unauthorized context
	secDenied := &pkgctx.SecurityContext{
		AccountID: "guest-user",
	}
	err := CheckDraftSweepEntitlement(secDenied)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires write:tpm-draft-sweep permission")
}
