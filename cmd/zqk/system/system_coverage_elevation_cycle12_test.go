package system

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentonboard"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/healthcheck"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func executeCycle12Command(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	cmd.SetContext(pkgctx.WithCommandOutputWriter(ctx, buf))
	err := cmd.Execute()
	return buf.String(), err
}

func setupCycle12ProjectEnv(t *testing.T) string {
	t.Helper()
	tempRoot := t.TempDir()
	dotZqk := filepath.Join(tempRoot, paths.ProjectDataDir)
	err := fileutil.MkdirAll(dotZqk, paths.DirPerm755)
	require.NoError(t, err)

	processDir := filepath.Join(dotZqk, "process")
	err = fileutil.MkdirAll(processDir, paths.DirPerm755)
	require.NoError(t, err)

	t.Setenv(zqkenv.ProjectRoot().Name(), tempRoot)
	t.Setenv(zqkenv.TestRoot().Name(), tempRoot)
	zqkenv.ProjectRoot().Set(tempRoot)
	zqkenv.TestRoot().Set(tempRoot)

	return tempRoot
}

func TestCycle12_AgentLockdown_Execution(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewAgentLockdownCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "agent-lockdown", cmd.Use)

	// Help output
	out, err := executeCycle12Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	// Ensure mocked agent directories exist
	for _, d := range []string{".gemini", ".ide", ".claude"} {
		_ = fileutil.MkdirAll(filepath.Join(tempDir, d), paths.DirPerm755)
	}

	// Test lock (default)
	lockCmd := NewAgentLockdownCmd()
	out, err = executeCycle12Command(t, lockCmd)
	assert.NoError(t, err)
	assert.Contains(t, out, "locked down")

	// Test unlock
	unlockCmd := NewAgentLockdownCmd()
	out, err = executeCycle12Command(t, unlockCmd, "--unlock")
	assert.NoError(t, err)
	assert.Contains(t, out, "unlocked")
}

func TestCycle12_AgentOnboard_ExecutionAndSummary(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewAgentOnboardCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "agent-onboard", cmd.Name())

	// Test help
	out, err := executeCycle12Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	// Test dry-run and detect-only execution
	detectCmd := NewAgentOnboardCmd()
	_, err = executeCycle12Command(t, detectCmd, "--detect-only")
	assert.NoError(t, err)

	dryRunCmd := NewAgentOnboardCmd()
	_, err = executeCycle12Command(t, dryRunCmd, "--dry-run")
	assert.NoError(t, err)

	// Exercise renderAgentOnboardSummary across different result shapes
	testCmd := &cobra.Command{}

	// 1. Detected vendors + files + created seating + git directory with hooks
	gitHooksDir := filepath.Join(tempDir, ".git", "hooks")
	_ = fileutil.MkdirAll(gitHooksDir, paths.DirPerm755)
	_ = fileutil.WriteStandardFile(filepath.Join(gitHooksDir, "pre-commit"), []byte("#!/bin/sh\nexit 0\n"))
	_ = fileutil.WriteStandardFile(filepath.Join(gitHooksDir, "pre-push"), []byte("#!/bin/sh\nexit 0\n"))
	_ = os.Chmod(filepath.Join(gitHooksDir, "pre-commit"), 0755)
	_ = os.Chmod(filepath.Join(gitHooksDir, "pre-push"), 0755)

	resWithVendors := &agentonboard.Result{
		Status: agentonboard.ResultSuccess,
		Stages: map[string]agentonboard.StageResult{
			agentonboard.StageDetect: {
				Status: "ok",
				Detail: map[string]any{
					"detected": []agentonboard.DetectedVendor{
						{DisplayName: "Cursor", Markers: []string{".cursor"}},
					},
				},
			},
			agentonboard.StagePrimeWorkspace: {
				Status: "ok",
				Detail: map[string]any{
					"files":   []string{".agents/AGENTS.md"},
					"skipped": []string{".cursor/mcp.json"},
				},
			},
			agentonboard.StageSeat: {
				Status: "ok",
				Detail: map[string]any{
					"created": 3,
				},
			},
		},
	}
	renderAgentOnboardSummary(testCmd, resWithVendors)

	// 2. Generic headless + zero created seating
	resHeadless := &agentonboard.Result{
		Status: agentonboard.ResultSuccess,
		Stages: map[string]agentonboard.StageResult{
			agentonboard.StageDetect: {
				Status: "ok",
				Detail: map[string]any{
					"detected": []agentonboard.DetectedVendor{},
				},
			},
			agentonboard.StageSeat: {
				Status: "ok",
				Detail: map[string]any{
					"created": 0,
				},
			},
		},
	}
	renderAgentOnboardSummary(testCmd, resHeadless)
}

func TestCycle12_Telemetry_Execution(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewTelemetryCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "telemetry", cmd.Name())

	// Test default table/text format
	out, err := executeCycle12Command(t, cmd)
	require.NoError(t, err)
	assert.Contains(t, out, "Swarm Telemetry:")

	// Test json format
	jsonCmd := NewTelemetryCmd()
	out, err = executeCycle12Command(t, jsonCmd, "-f", "json")
	require.NoError(t, err)
	assert.Contains(t, out, "agent_tasks")

	// Test yaml format
	yamlCmd := NewTelemetryCmd()
	out, err = executeCycle12Command(t, yamlCmd, "-f", "yaml")
	require.NoError(t, err)
	assert.Contains(t, out, "agent_tasks:")
}

func TestCycle12_HealthData_TableAndOutputs(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewHealthDataCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "health-data", cmd.Name())

	// Test direct writeHealthDataTable with comprehensive coverage across all branches
	testCmd := &cobra.Command{}

	data := &HealthDataOutput{
		Quarantine: &QuarantineReportData{
			QuarantineRoot: "/tmp/quarantine",
			TotalFiles:     2,
			Subfolders: []QuarantineSubfolder{
				{Name: "hash-duplicates", Count: 2, Source: "test"},
			},
		},
		CheckSummary: &CheckSummaryData{
			Source:        "tier1-latest.json",
			Blocking:      1,
			Warnings:      2,
			Informational: 3,
		},
		SchedulerMetrics: &SchedulerMetricsData{
			PoolCreationDeclinedCount: 4,
			DispatchPressureDropped: scheduler.DispatchPressureMetrics{
				Total:              10,
				Cron:               2,
				MissedJobRecovery:  1,
				TriggerJobSubmit:   3,
				ImmediateJobSubmit: 4,
			},
		},
		VolumeRates: &VolumeRatesData{
			ObjectVolume: &healthcheck.Result{Summary: "50 objects (10/hr)"},
			StreamVolume: &healthcheck.Result{Summary: "100 events (20/hr)"},
		},
		TestBundleHealth: &scheduler.TestBundleHealthStreamSummary{
			PathAlias:         "bundle-1",
			LineCount:         50,
			ByteSize:          1024,
			UpdatedAtRFC3339:  time.Now().UTC().Format(time.RFC3339),
		},
		RetentionAdvisory: &RetentionAdvisoryData{
			OverTarget:      true,
			Message:         "object count exceeds tolerance by 15%",
			SuggestedAction: "run system retention-status and prune",
		},
	}

	err := writeHealthDataTable(testCmd, data)
	require.NoError(t, err)

	// Branch with RetentionAdvisory under target and Quarantine nil
	dataUnderTarget := &HealthDataOutput{
		RetentionAdvisory: &RetentionAdvisoryData{
			OverTarget: false,
		},
	}
	err = writeHealthDataTable(testCmd, dataUnderTarget)
	require.NoError(t, err)

	// Test loadVolumeRatesForHealthData helper
	assert.Nil(t, loadVolumeRatesForHealthData(nil, ""))
	_ = loadVolumeRatesForHealthData(context.Background(), tempDir)

	// Test CLI command execution in valid project environment
	tableCmd := NewHealthDataCmd()
	_, err = executeCycle12Command(t, tableCmd)
	assert.NoError(t, err)
}

func TestCycle12_CleanupDuplicates_Execution(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewCleanupDuplicatesCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "cleanup-duplicates [kind]", cmd.Use)

	// Help output
	out, err := executeCycle12Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "Scans hash-based files")

	// Dry run with all kinds
	dryRunCmd := NewCleanupDuplicatesCmd()
	_, err = executeCycle12Command(t, dryRunCmd, "--dry-run")
	assert.NoError(t, err)

	// With positional kind argument
	kindCmd := NewCleanupDuplicatesCmd()
	_, err = executeCycle12Command(t, kindCmd, "goal", "--dry-run")
	assert.NoError(t, err)

	// With hash-duplicates and delete-hash-duplicates
	hashCmd := NewCleanupDuplicatesCmd()
	_, err = executeCycle12Command(t, hashCmd, "--dry-run", "--hash-duplicates", "--delete-hash-duplicates")
	assert.NoError(t, err)
}

func TestCycle12_CleanupQuarantine_Execution(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewCleanupQuarantineCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "cleanup-quarantine", cmd.Use)

	// Help output
	out, err := executeCycle12Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "quarantine")

	// Dry run
	dryRunCmd := NewCleanupQuarantineCmd()
	_, err = executeCycle12Command(t, dryRunCmd, "--dry-run", "--older-than", "1h")
	assert.NoError(t, err)

	// Verbose run
	verboseCmd := NewCleanupQuarantineCmd()
	_, err = executeCycle12Command(t, verboseCmd, "--dry-run", "--verbose")
	assert.NoError(t, err)
}

func TestCycle12_ValidateAgentRules_Execution(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewValidateAgentRulesCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "validate-agent-rules", cmd.Use)

	// Help output
	out, err := executeCycle12Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "Ensures the repo registry YAML matches")

	// Dry run writing manifest
	rulesDir := filepath.Join(tempDir, ".agent", "rules")
	_ = fileutil.MkdirAll(rulesDir, paths.DirPerm755)
	_ = fileutil.WriteStandardFile(filepath.Join(rulesDir, "RULE-001.mdc"), []byte("---\ndescription: Test Rule\n---\nRule Content"))

	writeCmd := NewValidateAgentRulesCmd()
	_, err = executeCycle12Command(t, writeCmd, "--project-root", tempDir, "--rules-dir", ".agent/rules", "--write-manifest")
	assert.NoError(t, err)

	// Validate against disk
	valCmd := NewValidateAgentRulesCmd()
	_, err = executeCycle12Command(t, valCmd, "--project-root", tempDir, "--rules-dir", ".agent/rules")
	assert.NoError(t, err)
}

func TestCycle12_AggregateAudit_WindowsAndExecution(t *testing.T) {
	t.Parallel()

	// 1. parseTimeWindowWithNow tests
	now := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

	// Default window 24h
	cmdDefault := NewAggregateAuditCmd()
	s, e, err := parseTimeWindowWithNow(cmdDefault, now)
	require.NoError(t, err)
	assert.Equal(t, now, e)
	assert.Equal(t, now.Add(-24*time.Hour), s)

	// Custom window 7d
	cmd7d := NewAggregateAuditCmd()
	cmd7d.Flags().Set("window", "7d")
	s, e, err = parseTimeWindowWithNow(cmd7d, now)
	require.NoError(t, err)
	assert.Equal(t, now.Add(-7*24*time.Hour), s)

	// Custom window 1w
	cmd1w := NewAggregateAuditCmd()
	cmd1w.Flags().Set("window", "1w")
	s, e, err = parseTimeWindowWithNow(cmd1w, now)
	require.NoError(t, err)
	assert.Equal(t, now.Add(-7*24*time.Hour), s)

	// Invalid window
	cmdBadWin := NewAggregateAuditCmd()
	cmdBadWin.Flags().Set("window", "invalid_dur")
	_, _, err = parseTimeWindowWithNow(cmdBadWin, now)
	assert.Error(t, err)

	// Explicit start & end
	cmdExplicit := NewAggregateAuditCmd()
	cmdExplicit.Flags().Set("start", "2026-10-01T00:00:00Z")
	cmdExplicit.Flags().Set("end", "2026-10-02T00:00:00Z")
	s, e, err = parseTimeWindowWithNow(cmdExplicit, now)
	require.NoError(t, err)
	assert.Equal(t, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), s)
	assert.Equal(t, time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC), e)

	// Explicit start with invalid format
	cmdBadStart := NewAggregateAuditCmd()
	cmdBadStart.Flags().Set("start", "not-a-date")
	_, _, err = parseTimeWindowWithNow(cmdBadStart, now)
	assert.Error(t, err)

	// Explicit end with invalid format
	cmdBadEnd := NewAggregateAuditCmd()
	cmdBadEnd.Flags().Set("start", "2026-10-01T00:00:00Z")
	cmdBadEnd.Flags().Set("end", "not-a-date")
	_, _, err = parseTimeWindowWithNow(cmdBadEnd, now)
	assert.Error(t, err)

	// parseTimeWindow live call
	cmdLive := NewAggregateAuditCmd()
	sLive, eLive, errLive := parseTimeWindow(cmdLive)
	require.NoError(t, errLive)
	assert.True(t, eLive.After(sLive))
}

func TestCycle12_SyncGlossaryFromSpecs_Execution(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewSyncGlossaryFromSpecsCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "sync-glossary-from-specs", cmd.Name())

	// Help
	out, err := executeCycle12Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	// Dry run with custom empty directories
	specsDir := filepath.Join(tempDir, "specs")
	cmdSpecsDir := filepath.Join(tempDir, paths.CLICommandSpecsDir)
	lifecyclesDir := filepath.Join(tempDir, "lifecycles")
	configsDir := filepath.Join(tempDir, "configs")
	_ = fileutil.MkdirAll(specsDir, paths.DirPerm755)
	_ = fileutil.MkdirAll(cmdSpecsDir, paths.DirPerm755)
	_ = fileutil.MkdirAll(lifecyclesDir, paths.DirPerm755)
	_ = fileutil.MkdirAll(configsDir, paths.DirPerm755)

	dryRunCmd := NewSyncGlossaryFromSpecsCmd()
	out, err = executeCycle12Command(t, dryRunCmd,
		"--dry-run",
		"--specs-dir", specsDir,
		"--lifecycles-dir", lifecyclesDir,
		"--configs-dir", configsDir,
	)
	assert.NoError(t, err)
	assert.Contains(t, out, "detected_candidates: 0")
}

func TestCycle12_InitCommands_RulesOnlyAndSeating(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewInitCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "init", cmd.Name())

	// Rules only generation
	rulesCmd := NewInitCmd()
	out, err := executeCycle12Command(t, rulesCmd, "--rules-only", "--project-name", "test-project")
	assert.NoError(t, err)
	assert.Contains(t, out, "Generated multi-host IDE rules")

	// Re-run with force
	forceCmd := NewInitCmd()
	out, err = executeCycle12Command(t, forceCmd, "--rules-only", "--force", "--project-name", "test-project")
	assert.NoError(t, err)
	assert.Contains(t, out, "Generated multi-host IDE rules")

	// Test SeedDefaultAgentSeatingPack with empty projectRoot (returns error cleanly)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	_, err = SeedDefaultAgentSeatingPack("", logger)
	assert.Error(t, err)

	// Exercise in temp project (best-effort seating call)
	_, _ = SeedDefaultAgentSeatingPack(tempDir, logger)
}

func TestCycle12_CheckCommand_ExecutionVariants(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	cmd := NewCheckCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "check [kind] [id...]", cmd.Use)

	// Help
	out, err := executeCycle12Command(t, cmd, "--help")
	require.NoError(t, err)
	assert.Contains(t, out, "Check object health and compliance")

	// Fast check
	fastCmd := NewCheckCmd()
	out, _ = executeCycle12Command(t, fastCmd, "--fast")
	assert.Contains(t, out, "System check complete")

	// Tier filter
	tierCmd := NewCheckCmd()
	_, _ = executeCycle12Command(t, tierCmd, "--fast", "--tier", "1", "--details")
}

func TestCycle12_AutoFixCommands_ExecutionVariants(t *testing.T) {
	tempDir := setupCycle12ProjectEnv(t)
	testkit.RegisterTempProjectTeardown(t, tempDir, nil)

	// 1. AutoFixBatchCmd
	batchCmd := NewAutoFixBatchCmd()
	require.NotNil(t, batchCmd)
	assert.Equal(t, "auto-fix-batch", batchCmd.Name())

	out, err := executeCycle12Command(t, batchCmd, "--help")
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	// Missing batch-file flag gives error
	missingCmd := NewAutoFixBatchCmd()
	_, err = executeCycle12Command(t, missingCmd)
	assert.Error(t, err)

	// 2. AutoFixProcessPendingCmd
	pendingCmd := NewAutoFixProcessPendingCmd()
	require.NotNil(t, pendingCmd)
	assert.Equal(t, "auto-fix-process-pending", pendingCmd.Name())

	out, err = executeCycle12Command(t, pendingCmd, "--help")
	require.NoError(t, err)
	assert.NotEmpty(t, out)

	dryPendingCmd := NewAutoFixProcessPendingCmd()
	_, err = executeCycle12Command(t, dryPendingCmd, "--project-root", tempDir)
	assert.NoError(t, err)
}

func TestCycle12_Whoami_HelpersAndOutput(t *testing.T) {
	cmd := NewWhoamiCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "whoami", cmd.Name())

	// parseMCPRoles
	assert.Nil(t, parseMCPRoles(""))
	assert.Equal(t, []string{"admin", "editor"}, parseMCPRoles("admin, editor"))

	// parseMCPPermissions
	assert.Nil(t, parseMCPPermissions(""))
	assert.Equal(t, []string{"read", "write"}, parseMCPPermissions("read, write"))

	// buildWhoamiOutputData
	info := &WhoamiAccountInfo{
		AccountID:   "ACC-TEST-001",
		Roles:       []string{"role1"},
		Permissions: []string{"perm1"},
		PersonaID:   "PER-TEST-001",
		Lane:        "doer",
		AccountDetails: map[string]any{
			objects.FieldKeyDisplayName: "Tester",
			objects.FieldKeyEmail:       "test@example.com",
			objects.FieldKeyUsername:    "tester1",
			objects.FieldKeyTitle:       "Craftsman",
		},
	}
	data := buildWhoamiOutputData(info)
	require.NotNil(t, data)
	assert.Equal(t, "ACC-TEST-001", data[objects.FieldKeyAccountID])

	// format table helpers
	assert.Contains(t, formatWhoamiTableAccount(data), "ACC-TEST-001")
	assert.Contains(t, formatWhoamiTableLane(data), "doer")
	assert.Contains(t, formatWhoamiTableName(data), "Tester")
	assert.Contains(t, formatWhoamiTableUserInfo(data), "tester1")
	assert.Contains(t, formatWhoamiTablePersona(data), "PER-TEST-001")

	// outputWhoamiTable
	bufCmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	bufCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))
	err := outputWhoamiTable(bufCmd, data)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Current User Account")
}

func TestCycle12_CheckFastContract_Coverage(t *testing.T) {
	assert.NoError(t, refuseReducedSurfaceWithMutatingFlags(nil))
	assert.NoError(t, refuseFastWithMutatingFlags(nil))
	assert.False(t, checkRefsReduced(nil))

	c := &cobra.Command{}
	c.Flags().Bool(checkFlagFast, false, "")
	c.Flags().Bool(checkFlagCheckRefs, true, "")
	c.Flags().Bool(checkFlagAutoFix, false, "")
	c.Flags().Bool(checkFlagForce, false, "")

	assert.False(t, checkRefsReduced(c))
	assert.False(t, checkFastModeEnabled(c))
	assert.NoError(t, refuseReducedSurfaceWithMutatingFlags(c))

	// With fast flag set
	_ = c.Flags().Set(checkFlagFast, "true")
	assert.True(t, checkRefsReduced(c))
	assert.True(t, checkFastModeEnabled(c))
	assert.NotEmpty(t, checkFastPartialPassLine())

	// Combining fast with auto-fix upgrades surface
	_ = c.Flags().Set(checkFlagAutoFix, "true")
	err := refuseReducedSurfaceWithMutatingFlags(c)
	assert.NoError(t, err)
	fastVal, _ := c.Flags().GetBool(checkFlagFast)
	assert.False(t, fastVal)
}

func TestCycle12_RetentionStatus_OutputAndReminders(t *testing.T) {
	cmd := NewRetentionStatusCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "retention-status", cmd.Name())

	// writeRetentionStatusTable at or under target
	bufCmd := &cobra.Command{}
	buf := new(bytes.Buffer)
	bufCmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), buf))

	dataUnder := RetentionStatusData{
		TotalInternal:   100,
		TargetTotal:     3000,
		AtOrUnderTarget: true,
	}
	err := writeRetentionStatusTable(bufCmd, dataUnder)
	assert.NoError(t, err)
	assert.Contains(t, buf.String(), "Status: at or under target")

	// writeRetentionStatusTable over target
	bufOver := new(bytes.Buffer)
	bufCmdOver := &cobra.Command{}
	bufCmdOver.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), bufOver))

	dataOver := RetentionStatusData{
		TotalInternal:   3500,
		TargetTotal:     3000,
		AtOrUnderTarget: false,
		OverTarget: []RetentionStatusOverKind{
			{Kind: "event", Current: 2000, Target: 1000, OverBy: 1000},
		},
		SuggestedAction: "run prune",
	}
	err = writeRetentionStatusTable(bufCmdOver, dataOver)
	assert.NoError(t, err)
	assert.Contains(t, bufOver.String(), "Status: over target")
	assert.Contains(t, bufOver.String(), "event: 2000 / 1000")

	// GetRetentionDriftReminder with empty project root
	visible, _, _ := GetRetentionDriftReminder(context.Background(), "")
	assert.False(t, visible)
}

func TestCycle12_DataCells_HelpersAndEnvelope(t *testing.T) {
	cmd := NewDataCellsCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "data-cells", cmd.Name())

	// filterDataCellsByKind empty
	descs := []datacell.CellKindDescriptor{
		{Kind: "goal", CellID: "CELL-1"},
	}
	filtered, err := filterDataCellsByKind(descs, "")
	require.NoError(t, err)
	assert.Len(t, filtered, 1)

	// filterDataCellsByKind non-existent
	_, err = filterDataCellsByKind(descs, "nonexistent_kind")
	assert.Error(t, err)

	// formatTestBundleHealthSummaryFooter
	assert.Empty(t, formatTestBundleHealthSummaryFooter(testBundleHealthSummary{}, false))
	footer := formatTestBundleHealthSummaryFooter(testBundleHealthSummary{
		LineCount:        42,
		UpdatedAtRFC3339: "2026-11-01T00:00:00Z",
	}, true)
	assert.Contains(t, footer, "42 line(s)")

	// testBundleHealthSummaryFooter with empty path
	assert.Empty(t, testBundleHealthSummaryFooter(t.TempDir()))
}

func TestCycle12_CheckTableSummary_NormalizeAndSummary(t *testing.T) {
	// 1. Scheduler job normalization
	assert.Equal(t, "job SCH-run-bundle-* failed", normalizeErrorMessage("job SCH-run-bundle-42 failed"))

	// 2. Hex YAML normalization
	hex64 := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef.yaml"
	assert.Equal(t, "file <hash>.yaml missing", normalizeErrorMessage("file "+hex64+" missing"))

	// 3. Object ID normalization
	assert.Equal(t, "error for object <id>", normalizeErrorMessage("error for object GOAL-001"))

	// 4. Path normalization
	assert.Equal(t, "at <path>", normalizeErrorMessage("at /Users/test/zqk/.zqk/process"))

	// 5. File mod normalization
	assert.Equal(t, "warn file modified <duration> ago", normalizeErrorMessage("warn file modified 5m30s ago"))

	// 6. Hex dots normalization
	assert.Equal(t, "hash <hash>...", normalizeErrorMessage("hash 12345678abcdef01..."))

	// isStaticConfigKind checks
	assert.True(t, isStaticConfigKind("policy"))
	assert.True(t, isStaticConfigKind("command_spec"))
	assert.False(t, isStaticConfigKind("goal"))
	assert.False(t, isStaticConfigKind("backlog_item"))

	// preliminaryCASIssue
	_, isPrelim := preliminaryCASIssue(CheckResult{ObjectKind: "policy", Status: "active"})
	assert.False(t, isPrelim)
}
