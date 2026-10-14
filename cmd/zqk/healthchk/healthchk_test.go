package healthchk

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func createHealthchkTestCmd(t *testing.T, projectRoot string, storageProvider storage.ObjectStorageProvider, args ...string) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmd := NewHealthchkCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	baseCtx := pkgctx.NewSystemContext()
	baseCtx = pkgctx.WithCommandOutputWriter(baseCtx, &buf)
	baseCtx = pkgctx.WithSecurityContext(baseCtx, pkgctx.NewSystemSecurityContext())
	if storageProvider != nil {
		baseCtx = cli.WithStorageProvider(baseCtx, storageProvider)
	}
	cmd.SetContext(baseCtx)

	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, err := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, err)
	cctx.ProjectRoot = projectRoot
	cli.SetContext(cmd, cctx)
	cmd.SetArgs(args)

	return cmd, &buf
}

func TestHealthchkCmd_Structure(t *testing.T) {
	cmd := NewHealthchkCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "healthchk", cmd.Use)
	assert.Contains(t, cmd.Short, "Health check registry")

	subcommands := make(map[string]*cobra.Command)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = sub
	}

	assert.Contains(t, subcommands, "list")
	assert.Contains(t, subcommands, "update")
	assert.Contains(t, subcommands, "run")
	assert.Contains(t, subcommands, "bulk")
}

func TestHealthchkList(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	cmd, buf := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "list")
	err := cmd.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "object_volume")
	assert.Contains(t, out, "wal_backlog")
}

func TestHealthchkUpdate(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// 1. Disable a monitor
	cmdDisable, bufDisable := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "update", "object_volume", "--enabled=false")
	err := cmdDisable.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufDisable.String(), "Monitor object_volume enabled=false")

	// 2. Re-enable the monitor
	cmdEnable, bufEnable := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "update", "object_volume", "--enabled=true")
	err = cmdEnable.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufEnable.String(), "Monitor object_volume enabled=true")

	// 3. Error on non-existent monitor
	cmdInvalid, bufInvalid := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "update", "nonexistent_monitor_xyz")
	assert.NotNil(t, bufInvalid)
	err = cmdInvalid.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestHealthchkRun(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// 1. Run single monitor in table/text format
	cmdSingle, bufSingle := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "run", "object_volume")
	err := cmdSingle.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufSingle.String(), "status=")

	// 2. Run single monitor in JSON format
	cmdJSON, bufJSON := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "run", "object_volume", "--format", "json")
	err = cmdJSON.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufJSON.String(), `"status"`)

	// 3. Run all monitors
	cmdAll, bufAll := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "run")
	err = cmdAll.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufAll.String(), "status=")
}

func TestHealthchkBulkDisable(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, nil)

	// 1. Disable via --ids flag
	cmdIDs, bufIDs := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "bulk", "disable", "--ids", "object_volume,wal_backlog")
	err := cmdIDs.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufIDs.String(), "Disabled 2 monitor(s)")

	// 2. Disable via --ids-file with YAML list
	yamlFile := filepath.Join(proj.Root, "monitors.yaml")
	require.NoError(t, fileutil.WriteFile(yamlFile, []byte("- object_volume\n- stream_volume\n"), 0600))

	cmdYAML, bufYAML := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "bulk", "disable", "--ids-file", yamlFile)
	err = cmdYAML.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufYAML.String(), "Disabled 2 monitor(s)")

	// 3. Disable via --ids-file with plain list
	txtFile := filepath.Join(proj.Root, "monitors.txt")
	require.NoError(t, fileutil.WriteFile(txtFile, []byte("object_volume\nwal_backlog\n"), 0600))

	cmdTxt, bufTxt := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "bulk", "disable", "--ids-file", txtFile)
	err = cmdTxt.Execute()
	require.NoError(t, err)
	assert.Contains(t, bufTxt.String(), "Disabled 2 monitor(s)")

	// 4. Missing both flags returns error
	cmdNoArgs, bufNoArgs := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "bulk", "disable")
	assert.NotNil(t, bufNoArgs)
	err = cmdNoArgs.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "provide --ids or --ids-file")

	// 5. Invalid file path returns error
	cmdMissingFile, bufMissingFile := createHealthchkTestCmd(t, proj.Root, proj.FileStorage, "bulk", "disable", "--ids-file", "nonexistent.yaml")
	assert.NotNil(t, bufMissingFile)
	err = cmdMissingFile.Execute()
	require.Error(t, err)
}

func TestResolveHealthcheckProjectRoot_ErrorCases(t *testing.T) {
	t.Chdir(t.TempDir())
	cmd := NewHealthchkCmd()
	initCtx := pkgctx.NewCliInitializationContext(cli.ResolveProjectRoot, ".")
	cctx, getErr := cli.GetContextFromCommand(cmd, initCtx)
	require.NoError(t, getErr)
	cctx.ProjectRoot = ""
	cli.SetContext(cmd, cctx)

	root, err := resolveHealthcheckProjectRoot(cmd)
	assert.Empty(t, root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")

	boundRoot, reg, regErr := resolveBoundRegistry(cmd)
	assert.Empty(t, boundRoot)
	assert.Nil(t, reg)
	require.Error(t, regErr)
}
