package mcp

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestInstallCmd_Metadata(t *testing.T) {
	cmd := NewInstallCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "install", cmd.Use)
}

func TestInstallCmd_RunInProject(t *testing.T) {
	tempRoot := t.TempDir()
	opts := testkit.TempProjectTeardown(tempRoot, nil)
	opts.SkipWaitWAL = false
	testkit.RegisterStandardTeardown(t, opts)
	t.Setenv(zqkenv.ProjectRoot().Name(), tempRoot)

	cmd := NewInstallCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)

	err := cmd.Execute()
	require.NoError(t, err)

	out := outBuf.String()
	require.Contains(t, out, "✓ ZQK MCP server configuration applied; restart IDE(s) for changes to take effect")

	ideConfig := filepath.Join(tempRoot, ".ide", "mcp.json")
	cursorConfig := filepath.Join(tempRoot, ".cursor", "mcp.json")
	require.True(t, fileutil.Exists(ideConfig))
	require.True(t, fileutil.Exists(cursorConfig))
}

func TestInstallCmd_OutsideProject(t *testing.T) {
	t.Setenv(zqkenv.ProjectRoot().Name(), "")

	cmd := NewInstallCmd()
	var outBuf bytes.Buffer
	cmd.SetOut(&outBuf)
	cmd.SetErr(&outBuf)

	emptyDir := t.TempDir()
	origWd, _ := fileutil.Getwd()
	defer func() { _ = fileutil.Chdir(origWd) }()
	_ = fileutil.Chdir(emptyDir)

	err := cmd.Execute()
	if err != nil {
		require.Contains(t, err.Error(), "must be run inside a project")
	}
}
