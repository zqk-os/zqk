package system

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func executeSystemSubcmd(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestSystemCmd_HierarchyAndSubcommands(t *testing.T) {
	cmd := NewSystemCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "system", cmd.Use)
	assert.True(t, cmd.HasSubCommands())

	// Collect subcommand names
	subCmds := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = true
	}

	expectedSubs := []string{
		"check", "status", "align", "whoami", "validate",
		"policy-interrupts", "resource-hygiene", "disk-usage",
		"stream-gc", "clean-branches", "kernel-integrity",
		"health-data", "data-cells", "dashboard", "init",
		"quickstart", "start-here", "shutdown", "start", "restart",
		"sync", "sync-git-hooks", "metrics", "audit-report",
	}
	for _, expected := range expectedSubs {
		assert.True(t, subCmds[expected], "expected subcommand %s in system cmd", expected)
	}

	out, err := executeSystemSubcmd(t, cmd, "--help")
	assert.NoError(t, err)
	assert.Contains(t, out, "System operations")
}

func TestSystemCheckCmd_FlagsAndHelp(t *testing.T) {
	cmd := NewCheckCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "check", cmd.Name())

	out, err := executeSystemSubcmd(t, cmd, "--help")
	assert.NoError(t, err)
	assert.Contains(t, out, "check")
	assert.Contains(t, out, "--auto-remedy")
	assert.Contains(t, out, "--auto-fix")
	assert.Contains(t, out, "--refresh-cache")
}

func TestSystemStatusCmd_FlagsAndHelp(t *testing.T) {
	cmd := NewStatusCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "status", cmd.Use)

	out, err := executeSystemSubcmd(t, cmd, "--help")
	assert.NoError(t, err)
	assert.Contains(t, out, "status")
}

func TestSystemAlignCmd_FlagsAndHelp(t *testing.T) {
	cmd := NewAlignCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "align", cmd.Use)

	out, err := executeSystemSubcmd(t, cmd, "--help")
	assert.NoError(t, err)
	assert.Contains(t, out, "align")
}

func TestSystemPolicyInterruptsCmd_HierarchyAndHelp(t *testing.T) {
	cmd := NewPolicyInterruptsCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "policy-interrupts", cmd.Use)

	out, err := executeSystemSubcmd(t, cmd, "--help")
	assert.NoError(t, err)
	assert.Contains(t, out, "policy-interrupts")

	// Verify ack subcommand exists
	subCmds := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subCmds[sub.Name()] = true
	}
	assert.True(t, subCmds["ack"], "expected ack subcommand under policy-interrupts")

	ackCmd := NewPolicyInterruptsAckCmd()
	require.NotNil(t, ackCmd)
	ackOut, ackErr := executeSystemSubcmd(t, ackCmd, "--help")
	assert.NoError(t, ackErr)
	assert.Contains(t, ackOut, "--dedupe-key")
}

func TestSystemResourceHygieneAndDiskUsage_Help(t *testing.T) {
	rhCmd := NewResourceHygieneCmd()
	require.NotNil(t, rhCmd)
	rhOut, err := executeSystemSubcmd(t, rhCmd, "--help")
	assert.NoError(t, err)
	assert.NotEmpty(t, rhOut)

	duCmd := NewDiskUsageCmd()
	require.NotNil(t, duCmd)
	duOut, err := executeSystemSubcmd(t, duCmd, "--help")
	assert.NoError(t, err)
	assert.NotEmpty(t, duOut)
}

func TestSystemLifecycleCommands_Help(t *testing.T) {
	cmds := []*cobra.Command{
		NewInitCmd(),
		NewStartHereCmd(),
		NewQuickstartCmd(),
		NewAgentOnboardCmd(),
		NewShutdownCmd(),
		NewStartCmd(),
		NewRestartCmd(),
		NewGenerateAgentConfigsCmd(),
		NewSeedDefaultAgentSeatingCmd(),
		NewRebuildGraphProjectionCmd(),
	}

	for _, c := range cmds {
		require.NotNil(t, c)
		out, err := executeSystemSubcmd(t, c, "--help")
		assert.NoError(t, err, "help failed for %s", c.Name())
		assert.NotEmpty(t, out)
	}
}

func TestSystemValidationAndIntegrity_Help(t *testing.T) {
	cmds := []*cobra.Command{
		NewValidateCmd(),
		NewValidateCommandSpecsCmd(),
		NewValidateAgentRulesCmd(),
		NewValidateScenarioCmd(),
		NewKernelIntegrityCmd(),
		NewTruthSentinelCmd(),
		NewUpdateSpecsCmd(),
		NewSpecOriginationCmd(),
		NewCliHooksCmd(),
		NewVerifyCompletionCmd(),
	}

	for _, c := range cmds {
		require.NotNil(t, c)
		out, err := executeSystemSubcmd(t, c, "--help")
		assert.NoError(t, err, "help failed for %s", c.Name())
		assert.NotEmpty(t, out)
	}
}

func TestSystemMaintenanceAndStorage_Help(t *testing.T) {
	cmds := []*cobra.Command{
		NewSyncCmd(),
		NewSyncGitHooksCmd(),
		NewSyncCASIndexCmd(),
		NewCleanBranchesCmd(),
		NewStreamGCCmd(),
		NewCleanupDuplicatesCmd(),
		NewCleanupQuarantineCmd(),
		NewQuarantineReportCmd(),
		NewRecoverCASCmd(),
		NewRepairCASCorruptionCmd(),
		NewMigrateCasCmd(),
		NewCompactJournalCmd(),
		NewCompactWALCmd(),
	}

	for _, c := range cmds {
		require.NotNil(t, c)
		out, err := executeSystemSubcmd(t, c, "--help")
		assert.NoError(t, err, "help failed for %s", c.Name())
		assert.NotEmpty(t, out)
	}
}

func TestSystemSnapshotCommands_Help(t *testing.T) {
	cmds := []*cobra.Command{
		NewSnapshotCmd(),
		NewSnapshotScenarioCmd(),
		NewSnapshotExpandCmd(),
		NewSnapshotVerifyCmd(),
		NewStateCommitCmd(),
		NewStateDiffCmd(),
		NewStateRestoreCmd(),
	}

	for _, c := range cmds {
		require.NotNil(t, c)
		out, err := executeSystemSubcmd(t, c, "--help")
		assert.NoError(t, err, "help failed for %s", c.Name())
		assert.NotEmpty(t, out)
	}
}

func TestSystemMetricsReportsAndDataCells_Help(t *testing.T) {
	cmds := []*cobra.Command{
		NewMetricsCmd(),
		NewAuditReportCmd(),
		NewImprovementReportCmd(),
		NewAuditMilestonesCmd(),
		NewAuditStreamCmd(),
		NewExportGateCmd(),
		NewHealthDataCmd(),
		NewDataCellsCmd(),
		NewDashboardCmd(),
		NewTelemetryCmd(),
		NewFederateCmd(),
		NewPromptBuilderCmd(),
		NewCheckPolicyCmd(),
		NewEnsureRetentionJobsCmd(),
		NewRetentionToleranceCmd(),
	}

	for _, c := range cmds {
		require.NotNil(t, c)
		out, err := executeSystemSubcmd(t, c, "--help")
		assert.NoError(t, err, "help failed for %s", c.Name())
		assert.NotEmpty(t, out)
	}
}

func TestSystemInternalAndDaemonCommands_Help(t *testing.T) {
	cmds := []*cobra.Command{
		NewAutoFixBatchCmd(),
		NewAutoFixProcessPendingCmd(),
		NewConfigGetCmd(),
		NewSemanticBridgeDaemonCmd(),
		NewFSWatcherDaemonCmd(),
		NewEmergencyManagerCmd(),
		NewOrchestrateBatchCmd(),
		NewSealSkillCmd(),
		NewGitCmd(),
	}

	for _, c := range cmds {
		require.NotNil(t, c)
		out, err := executeSystemSubcmd(t, c, "--help")
		assert.NoError(t, err, "help failed for %s", c.Name())
		assert.NotEmpty(t, out)
	}
}

func TestSystemCommand_NegativeAndBoundaryScenarios(t *testing.T) {
	// Unknown flag rejection
	cmd := NewCheckCmd()
	_, err := executeSystemSubcmd(t, cmd, "--non-existent-system-flag")
	assert.Error(t, err)

	// Policy interrupts ack missing required flag
	ackCmd := NewPolicyInterruptsAckCmd()
	_, err = executeSystemSubcmd(t, ackCmd)
	assert.Error(t, err)

	// Clean branches invalid dry run args
	cbCmd := NewCleanBranchesCmd()
	_, err = executeSystemSubcmd(t, cbCmd, "--unknown-flag")
	assert.Error(t, err)
}

func TestSystemWhoami_ExecutionWithContext(t *testing.T) {
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	zqkenv.ProjectRoot().Set(tmpDir)
	zqkenv.TestRoot().Set(tmpDir)

	cmd := NewWhoamiCmd()
	cmd.SetContext(context.Background())

	out, err := executeSystemSubcmd(t, cmd, "-f", "json")
	if err == nil {
		assert.NotEmpty(t, out)
	}
}

func TestSystemDiskUsage_Execution(t *testing.T) {
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	zqkDir := filepath.Join(tmpDir, paths.ProjectDataDir)
	require.NoError(t, fileutil.MkdirAll(zqkDir, paths.DirPerm755))

	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	t.Setenv("ZQK_TEST_ROOT", tmpDir)
	zqkenv.ProjectRoot().Set(tmpDir)

	cmd := NewDiskUsageCmd()
	_, err := executeSystemSubcmd(t, cmd)
	assert.NoError(t, err)
}

func TestSystemResourceHygiene_Execution(t *testing.T) {
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	cmd := NewResourceHygieneCmd()
	out, err := executeSystemSubcmd(t, cmd)
	if err == nil {
		assert.NotEmpty(t, out)
	}
}
