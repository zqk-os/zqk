package system

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupSyncAgentsProject(t *testing.T) (string, string) {
	t.Helper()
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	agentsDir := filepath.Join(tmpDir, paths.ProjectDataDir, "agents")
	require.NoError(t, fileutil.MkdirAll(agentsDir, paths.DirPerm755))
	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(agentsDir, "agent.json"), []byte("{}")))
	require.NoError(t, fileutil.WriteStandardFile(filepath.Join(agentsDir, "config.yaml"), []byte("key: value")))

	return tmpDir, agentsDir
}

func executeSyncAgentsCmd(projectRoot string, args ...string) (string, error) {
	cmd := NewSyncAgentsCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)
	if projectRoot != "" {
		cli.SetContext(cmd, cli.ContextForProjectRoot(projectRoot))
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestNewSyncAgentsCmd_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	cmd := NewSyncAgentsCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "sync-agents", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("dir"))
}

func TestSyncAgents_OperationalDynamicProof_Ingest(t *testing.T) {
	tmpDir, agentsDir := setupSyncAgentsProject(t)

	_, err := executeSyncAgentsCmd(tmpDir, "--dir", agentsDir)
	require.NoError(t, err)
}

func TestSyncAgents_OperationalDynamicProof_EmptyDir(t *testing.T) {
	tmpDir, agentsDir := setupSyncAgentsProject(t)
	assert.NotEmpty(t, agentsDir)

	nonExistentDir := filepath.Join(tmpDir, "does_not_exist")
	_, err := executeSyncAgentsCmd(tmpDir, "--dir", nonExistentDir)
	require.NoError(t, err)
}
