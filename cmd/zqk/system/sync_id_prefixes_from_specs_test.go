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
	"github.com/zqk-os/zqk/pkg/validation"
)

func setupSyncPrefixesProject(t *testing.T) (string, string, string) {
	t.Helper()
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	specsDir := filepath.Join(tmpDir, "specs")
	configDir := filepath.Join(tmpDir, paths.ProcessInternalConfigsDir)
	require.NoError(t, fileutil.MkdirAll(specsDir, paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(configDir, paths.DirPerm755))

	configFile := filepath.Join(configDir, paths.IdPrefixesConfigFile)
	initialConfig := `kind_to_prefixes:
  goal:
    - GOAL
kind_to_synonyms: {}
`
	require.NoError(t, fileutil.WriteFile(configFile, []byte(initialConfig), paths.FilePerm644))

	// Write a sample spec with a new prefix and synonym
	specContent := `kind: sample_kind
id_prefixes:
  - SMP
id_synonyms:
  - SAMPLE
`
	require.NoError(t, fileutil.WriteFile(filepath.Join(specsDir, "sample_kind.yaml"), []byte(specContent), paths.FilePerm644))

	return tmpDir, specsDir, configFile
}

func executeSyncPrefixesCmd(projectRoot string, args ...string) (string, error) {
	cmd := NewSyncIDPrefixesFromSpecsCmd()
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

func TestNewSyncIDPrefixesFromSpecsCmd_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	cmd := NewSyncIDPrefixesFromSpecsCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "sync-id-prefixes-from-specs", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("specs-dir"))
	assert.NotNil(t, cmd.Flags().Lookup("config-file"))
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, cmd.Flags().Lookup("apply"))
}

func TestSyncIDPrefixesFromSpecs_NegativeAdversarialBoundary_InvalidConfig(t *testing.T) {
	tmpDir, specsDir, configPath := setupSyncPrefixesProject(t)
	assert.NotEmpty(t, specsDir)
	assert.NotEmpty(t, configPath)

	badConfigFile := filepath.Join(tmpDir, "bad_config.yaml")
	require.NoError(t, fileutil.WriteFile(badConfigFile, []byte("invalid: 'unclosed"), paths.FilePerm644))

	_, err := executeSyncPrefixesCmd(tmpDir, "--config-file", badConfigFile)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load ID prefixes config")
}

func TestSyncIDPrefixesFromSpecs_OperationalDynamicProof_DryRunReport(t *testing.T) {
	tmpDir, specsDir, configFile := setupSyncPrefixesProject(t)

	// Format: Table (default)
	out, err := executeSyncPrefixesCmd(tmpDir, "--specs-dir", specsDir, "--config-file", configFile, "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "sync-id-prefixes-from-specs")
	assert.Contains(t, out, "dry_run: true")
	assert.Contains(t, out, "prefix_updates: 1")
	assert.Contains(t, out, "synonym_updates: 1")
	assert.Contains(t, out, "Changes:")

	// Format: JSON
	jsonOut, err := executeSyncPrefixesCmd(tmpDir, "--specs-dir", specsDir, "--config-file", configFile, "--dry-run", "--format", "json")
	require.NoError(t, err)
	assert.Contains(t, jsonOut, `"dry_run": true`)
	assert.Contains(t, jsonOut, `"prefix_updates": 1`)
	assert.Contains(t, jsonOut, `"synonym_updates": 1`)
}

func TestSyncIDPrefixesFromSpecs_OperationalDynamicProof_Apply(t *testing.T) {
	tmpDir, specsDir, configFile := setupSyncPrefixesProject(t)

	out, err := executeSyncPrefixesCmd(tmpDir, "--specs-dir", specsDir, "--config-file", configFile, "--apply", "--dry-run=false")
	require.NoError(t, err)
	assert.Contains(t, out, "dry_run: false")

	// Verify configFile was updated
	cfg, err := validation.LoadIDPrefixesConfig(configFile)
	require.NoError(t, err)
	assert.Equal(t, []string{"SMP"}, cfg.KindToPrefixes["sample_kind"])
	assert.Equal(t, []string{"SAMPLE"}, cfg.KindToSynonyms["sample_kind"])

	// Running a second time should detect no changes
	outSecond, err := executeSyncPrefixesCmd(tmpDir, "--specs-dir", specsDir, "--config-file", configFile, "--apply", "--dry-run=false")
	require.NoError(t, err)
	assert.Contains(t, outSecond, "No changes detected.")
}
