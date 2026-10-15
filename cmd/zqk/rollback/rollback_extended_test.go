package rollback_test

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	rollbackcmd "github.com/zqk-os/zqk/cmd/zqk/rollback"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/rollback"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testenvroot"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := fileutil.Getwd()
	require.NoError(t, err)
	for {
		if _, err := fileutil.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func setupRollbackTestProject(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Cleanup(func() {
		cleanupErr := storage.RunProjectTestTeardown(storage.TempProjectTeardown(tmpDir, nil))
		if cleanupErr != nil {
			t.Logf("cleanup teardown notice: %v", cleanupErr)
		}
	})
	t.Setenv("ZQK_TEST_ROOT", tmpDir)

	repoRoot := findModuleRoot(t)
	bootstrapErr := testenvroot.BootstrapRoot(tmpDir, repoRoot)
	require.NoError(t, bootstrapErr)
	copyLifecycleErr := testenvroot.CopyLifecyclesFromProject(tmpDir, repoRoot)
	require.NoError(t, copyLifecycleErr)
	copySpecErr := testenvroot.CopyObjectSpecsFromProject(tmpDir, repoRoot)
	require.NoError(t, copySpecErr)

	return tmpDir
}

func executeRollbackCmd(t *testing.T, cmd *cobra.Command, root string, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if root != "" {
		cli.SetContext(cmd, cli.ContextForProjectRoot(root))
	}
	cmd.SetContext(pkgctx.WithCommandOutputWriter(pkgctx.NewSystemContext(), &buf))
	cmd.SetArgs(args)
	cmdErr := cmd.Execute()
	return buf.String(), cmdErr
}

func TestRollbackListCmd_ValidationAndExecution(t *testing.T) {
	testRoot := setupRollbackTestProject(t)

	// 1. Empty list initially
	cmdList := rollbackcmd.NewRollbackCmd()
	out1, err1 := executeRollbackCmd(t, cmdList, testRoot, "list")
	require.NoError(t, err1)
	assert.Contains(t, out1, "points")

	// 2. Capture a rollback point
	pointID, captureErr := rollback.Capture(testRoot, "test_scope", "SCOPE-001", func() ([]rollback.ObjectState, error) {
		return []rollback.ObjectState{
			{
				ID:    "BLI-001",
				Kind:  "backlog_item",
				State: map[string]any{"id": "BLI-001", "title": "Initial State"},
			},
		}, nil
	})
	require.NoError(t, captureErr)
	require.NotEmpty(t, pointID)

	// 3. List with --last and --within
	cmdList2 := rollbackcmd.NewRollbackCmd()
	out2, err2 := executeRollbackCmd(t, cmdList2, testRoot, "list", "--last", "5", "--within", "1h")
	require.NoError(t, err2)
	assert.Contains(t, out2, pointID)

	// 4. Invalid --within duration format
	cmdList3 := rollbackcmd.NewRollbackCmd()
	_, err3 := executeRollbackCmd(t, cmdList3, testRoot, "list", "--within", "not-a-duration")
	require.Error(t, err3)
	assert.Contains(t, err3.Error(), "invalid --within duration")
}

func TestRollbackApplyCmd_ExecutionAndErrors(t *testing.T) {
	testRoot := setupRollbackTestProject(t)

	// 1. Missing pointID argument fails
	cmdApply1 := rollbackcmd.NewRollbackCmd()
	_, err1 := executeRollbackCmd(t, cmdApply1, testRoot, "apply")
	require.Error(t, err1)

	// 2. Non-existent point reports not-found with reconstruct hint
	cmdApply2 := rollbackcmd.NewRollbackCmd()
	_, err2 := executeRollbackCmd(t, cmdApply2, testRoot, "apply", "RBP-NONEXISTENT")
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "zqk rollback reconstruct")

	// 3. Capture and apply valid rollback point
	pointID, captureErr := rollback.Capture(testRoot, "scope", "SC-100", func() ([]rollback.ObjectState, error) {
		return []rollback.ObjectState{
			{
				ID:    "BLI-100",
				Kind:  "backlog_item",
				State: map[string]any{"id": "BLI-100", "title": "Captured Item"},
			},
		}, nil
	})
	require.NoError(t, captureErr)

	cmdApply3 := rollbackcmd.NewRollbackCmd()
	cmdApply3.SetContext(pkgctx.NewSystemContext())
	cli.SetContext(cmdApply3, cli.ContextForProjectRoot(testRoot))
	proc, procErr := cli.NewProcessor(cmdApply3)
	require.NoError(t, procErr)
	sysCtx := pkgctx.NewSystemContext()
	createErr := proc.Storage().Create(sysCtx, pkgctx.NewSystemSecurityContext(), map[string]any{
		"id":            "BLI-100",
		"kind":          "backlog_item",
		"title":         "Initial Item",
		"status":        "planned",
		"priority_tier": "P1",
	})
	require.NoError(t, createErr)

	out3, err3 := executeRollbackCmd(t, cmdApply3, testRoot, "apply", pointID)
	require.NoError(t, err3)
	assert.Contains(t, out3, pointID)
}

func TestRollbackReconstructCmd_ValidationAndExecution(t *testing.T) {
	testRoot := setupRollbackTestProject(t)

	// 1. Missing required flags
	cmdRec1 := rollbackcmd.NewRollbackCmd()
	_, err1 := executeRollbackCmd(t, cmdRec1, testRoot, "reconstruct")
	require.Error(t, err1)

	// 2. Invalid timestamp format
	cmdRec2 := rollbackcmd.NewRollbackCmd()
	_, err2 := executeRollbackCmd(t, cmdRec2, testRoot, "reconstruct", "--scope-id", "test:1", "--timestamp", "invalid-time")
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "invalid --timestamp")

	// 3. No refs found for scope
	ts := time.Now().UTC().Format(time.RFC3339)
	cmdRec3 := rollbackcmd.NewRollbackCmd()
	_, err3 := executeRollbackCmd(t, cmdRec3, testRoot, "reconstruct", "--scope-type", "plan", "--scope-id", "NON-EXISTENT", "--timestamp", ts)
	require.Error(t, err3)
	assert.Contains(t, err3.Error(), "no refs for scope")
}

func TestRollbackRetainCmd_Execution(t *testing.T) {
	testRoot := setupRollbackTestProject(t)

	cmdRetain := rollbackcmd.NewRollbackCmd()
	out, err := executeRollbackCmd(t, cmdRetain, testRoot, "retain")
	require.NoError(t, err)
	assert.Contains(t, out, "retained")
}

func TestRollbackGetLoggerFromProfile(t *testing.T) {
	cmdDefault := rollbackcmd.NewRollbackCmd()
	loggerDefault := rollbackcmd.GetLoggerFromProfile(cmdDefault)
	assert.NotNil(t, loggerDefault)

	cmdAgent := rollbackcmd.NewRollbackCmd()
	cliCtx := cli.ContextForProjectRoot(".")
	cliCtx.Context.Profile = "ai-agent"
	cli.SetContext(cmdAgent, cliCtx)
	loggerAgent := rollbackcmd.GetLoggerFromProfile(cmdAgent)
	assert.NotNil(t, loggerAgent)
}
