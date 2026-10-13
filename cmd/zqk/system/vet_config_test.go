package system

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	clicontext "github.com/zqk-os/zqk/pkg/cliapp/context"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupVetConfigTestEnv(t *testing.T) string {
	t.Helper()
	tempRoot := t.TempDir()
	dotZqk := filepath.Join(tempRoot, paths.ProjectDataDir)
	mkdirErr := fileutil.MkdirAll(dotZqk, paths.DirPerm755)
	require.NoError(t, mkdirErr)

	t.Setenv("ZQK_PROJECT_ROOT", tempRoot)
	return tempRoot
}

func assertVetConfigIssues(t *testing.T) {
	t.Helper()
	cmd := NewVetConfigCmd()
	err := runVetConfig(cmd, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "found 1 configuration issues")
}

func TestNewVetConfigCmd_StaticFloorInvariant(t *testing.T) {
	cmd := NewVetConfigCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "vet-config", cmd.Use)
	assert.NotEmpty(t, cmd.Short)
}

func TestRunVetConfig_OperationalProof_CleanProject(t *testing.T) {
	cmd := NewVetConfigCmd()
	err := runVetConfig(cmd, nil)
	assert.NoError(t, err)
}

func TestRunVetConfig_NegativeBoundary_MissingBrandSettings(t *testing.T) {
	setupVetConfigTestEnv(t)
	assertVetConfigIssues(t)
}

func TestRunVetConfig_NegativeBoundary_InvalidBrandSettings(t *testing.T) {
	tempRoot := setupVetConfigTestEnv(t)

	settingsPath := clicontext.BrandSettingsPath(tempRoot)
	mkdirConfErr := fileutil.EnsureDir(filepath.Dir(settingsPath))
	require.NoError(t, mkdirConfErr)
	writeErr := fileutil.WriteStandardFile(settingsPath, []byte("paths:\n  invalid_yaml: [unclosed"))
	require.NoError(t, writeErr)

	assertVetConfigIssues(t)
}
