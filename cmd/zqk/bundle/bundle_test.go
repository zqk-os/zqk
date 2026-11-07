package bundle

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	cliapp "github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestBundleCmd_Hierarchy(t *testing.T) {
	cmd := NewBundleCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "bundle", cmd.Use)

	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}
	require.True(t, subcommands["list"], "list subcommand missing")
	require.True(t, subcommands["info"], "info subcommand missing")
	require.True(t, subcommands["add"], "add subcommand missing")
	require.True(t, subcommands["remove"], "remove subcommand missing")

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())
	require.Contains(t, buf.String(), "bundle")
}

func TestBundleCLI_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()

	manifestContent := `schema_version: "1"
name: test-bundle
language: go
version: 1.0.0
description: Test Go Bundle
standards:
  - id: GO-STD-001
    title: Error Handling
    description: Must not use panic
    severity: error
`
	manifestPath := filepath.Join(tmpDir, "bundle.yaml")
	require.NoError(t, fileutil.WriteFile(manifestPath, []byte(manifestContent), paths.FilePerm644))

	// 1. Initial list (empty)
	{
		cmd := newListCmd()
		cliapp.SetContext(cmd, cliapp.ContextForProjectRoot(tmpDir))
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		require.NoError(t, cmd.Execute())
		require.Contains(t, buf.String(), "No language bundles currently installed")
	}

	// 2. Add bundle
	{
		cmd := newInstallCmd()
		cliapp.SetContext(cmd, cliapp.ContextForProjectRoot(tmpDir))
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"--file", manifestPath})
		require.NoError(t, cmd.Execute())
		require.Contains(t, buf.String(), "installed successfully")
	}

	// 3. List installed bundle (table)
	{
		cmd := newListCmd()
		cliapp.SetContext(cmd, cliapp.ContextForProjectRoot(tmpDir))
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		require.NoError(t, cmd.Execute())
		require.Contains(t, buf.String(), "test-bundle")
		require.Contains(t, buf.String(), "1.0.0")
	}

	// 4. List installed bundle (json)
	{
		cmd := newListCmd()
		cliapp.SetContext(cmd, cliapp.ContextForProjectRoot(tmpDir))
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"--format", "json"})
		require.NoError(t, cmd.Execute())
		require.Contains(t, buf.String(), `"name": "test-bundle"`)
	}

	// 5. Info bundle
	{
		cmd := newInfoCmd()
		cliapp.SetContext(cmd, cliapp.ContextForProjectRoot(tmpDir))
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"test-bundle"})
		require.NoError(t, cmd.Execute())
		require.Contains(t, buf.String(), `"test-bundle"`)
		require.Contains(t, buf.String(), "GO-STD-001")
	}

	// 6. Remove bundle
	{
		cmd := newRemoveCmd()
		cliapp.SetContext(cmd, cliapp.ContextForProjectRoot(tmpDir))
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetArgs([]string{"test-bundle"})
		require.NoError(t, cmd.Execute())
		require.Contains(t, buf.String(), "uninstalled")
	}

	// 7. Verify list is empty again
	{
		cmd := newListCmd()
		cliapp.SetContext(cmd, cliapp.ContextForProjectRoot(tmpDir))
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		require.NoError(t, cmd.Execute())
		require.Contains(t, buf.String(), "No language bundles currently installed")
	}
}
