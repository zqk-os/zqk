package utility

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewFixRegistrationCmd(t *testing.T) {
	cmd := NewFixRegistrationCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "fix-registration [id...]", cmd.Use)

	assert.NotNil(t, cmd.Flags().Lookup("kind"))
	assert.NotNil(t, cmd.Flags().Lookup("all"))
	assert.NotNil(t, cmd.Flags().Lookup("fix"))
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, cmd.Flags().Lookup("quiet"))

	// Test args validator
	assert.Error(t, cmd.Args(cmd, []string{}))
	assert.NoError(t, cmd.Args(cmd, []string{"BLI-001"}))
}

func TestRunFixRegistration_Flows(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	cmd := NewFixRegistrationCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(cmd.Context(), &buf)
	cmd.SetContext(ctx)

	// Case 1: No registration issues found
	buf.Reset()
	err := runFixRegistration(cmd, []string{"NONEXISTENT-001"}, "", false, false, false, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No registration issues found")

	// Case 2: Create an object with mismatching ID prefix:
	// A decision object with prefix "CUSTOMDEC-001"
	decDir := datacell.CellCASPrimaryDir(proj.Root, "decisions")
	require.NoError(t, fileutil.MkdirAll(decDir, paths.DirPerm755))
	decFile := filepath.Join(decDir, "CUSTOMDEC-001.yaml")
	decData := []byte("id: CUSTOMDEC-001\nkind: decision\ntitle: Custom Decision\n")
	require.NoError(t, fileutil.WriteFile(decFile, decData, paths.FilePerm644))

	// Scan by ID
	buf.Reset()
	err = runFixRegistration(cmd, []string{"CUSTOMDEC-001"}, "", false, false, false, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Found 1 registration issue(s)")
	assert.Contains(t, buf.String(), "CUSTOMDEC-001")

	// Scan by kind
	buf.Reset()
	err = runFixRegistration(cmd, []string{}, "decision", false, false, false, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Found 1 registration issue(s)")

	// Scan with --all
	buf.Reset()
	err = runFixRegistration(cmd, []string{}, "", true, false, false, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Found 1 registration issue(s)")

	// Create id_prefixes config file for --fix testing
	configDir := filepath.Join(proj.Root, paths.ProcessInternalConfigsDir)
	require.NoError(t, fileutil.MkdirAll(configDir, paths.DirPerm755))
	configPath := filepath.Join(configDir, paths.IdPrefixesConfigFile)
	require.NoError(t, fileutil.WriteFile(configPath, []byte("prefixes:\n  decision:\n    - DEC-\n"), paths.FilePerm644))

	// Scan with --fix and --dry-run
	cfgDir := filepath.Join(proj.Root, ".zqk", "specs", "configs")
	require.NoError(t, fileutil.MkdirAll(cfgDir, paths.DirPerm755))
	cfgFile := filepath.Join(cfgDir, "id_prefixes_config.yaml")
	require.NoError(t, fileutil.WriteFile(cfgFile, []byte("prefixes: {}\n"), paths.FilePerm644))

	buf.Reset()
	err = runFixRegistration(cmd, []string{"CUSTOMDEC-001"}, "", false, true, true, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Dry run: would add 1 prefix(es)")

	// Scan with --fix (modifies config)
	buf.Reset()
	err = runFixRegistration(cmd, []string{"CUSTOMDEC-001"}, "", false, true, false, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Added 1 prefix(es) to id_prefixes config")

	// Scan with --quiet and --fix
	buf.Reset()
	err = runFixRegistration(cmd, []string{"CUSTOMDEC-001"}, "", false, true, false, true)
	require.NoError(t, err)
}

func TestNotifyScanning(t *testing.T) {
	cmd := NewFixRegistrationCmd()
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)

	// Quiet = true does not output
	notifyScanning(cmd, true)
	assert.Empty(t, buf.String())

	// Quiet = false outputs message
	notifyScanning(cmd, false)
	assert.Contains(t, buf.String(), "Scanning objects for registration issues...")
}
