package system

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/agentrules"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupRulesTestProject(t *testing.T) string {
	t.Helper()
	t.Parallel()
	root := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, root, nil)
	return root
}

func executeValidateAgentRulesCmd(projectRoot string, args ...string) (string, error) {
	cmd := NewValidateAgentRulesCmd()
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

func seedAgentRuleFile(t *testing.T, dir, filename string, content []byte) {
	t.Helper()
	require.NoError(t, fileutil.EnsureDir(dir))
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(dir, filename), content))
}

func seedManifestFile(t *testing.T, root, content string) {
	t.Helper()
	cfgDir := filepath.Dir(agentrules.ManifestPath(root))
	require.NoError(t, fileutil.EnsureDir(cfgDir))
	require.NoError(t, fileutil.WriteSecureFile(filepath.Join(cfgDir, agentrules.ManifestFileName), []byte(content)))
}

func TestNewValidateAgentRulesCmd_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	cmd := NewValidateAgentRulesCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "validate-agent-rules", cmd.Use)
	assert.Contains(t, cmd.Aliases, "validate-ide-rules")
	assert.NotEmpty(t, cmd.Short)
	assert.NotNil(t, cmd.Flags().Lookup("write-manifest"))
	assert.NotNil(t, cmd.Flags().Lookup("project-root"))
	assert.NotNil(t, cmd.Flags().Lookup("rules-dir"))
}

func TestValidateAgentRules_OperationalProof_ValidMatch(t *testing.T) {
	root := setupRulesTestProject(t)
	rulesDir := filepath.Join(root, ".ide", "rules")
	seedAgentRuleFile(t, rulesDir, "sample.mdc", []byte("---\nalwaysApply: true\n---\n"))
	seedManifestFile(t, root, "version: 1\nrules:\n  - sample.mdc\n")

	out, err := executeValidateAgentRulesCmd(root, "--project-root", root)
	require.NoError(t, err)
	assert.NotNil(t, out)
}

func TestValidateAgentRules_OperationalProof_WriteManifest(t *testing.T) {
	root := setupRulesTestProject(t)
	rulesDir := filepath.Join(root, ".ide", "rules")
	seedAgentRuleFile(t, rulesDir, "rule_alpha.mdc", []byte("content alpha"))
	seedAgentRuleFile(t, rulesDir, "rule_beta.mdc", []byte("content beta"))

	out, err := executeValidateAgentRulesCmd(root, "--write-manifest", "--project-root", root)
	require.NoError(t, err)
	assert.NotNil(t, out)

	// Validate passes after writing manifest
	vOut, vErr := executeValidateAgentRulesCmd(root, "--project-root", root)
	require.NoError(t, vErr)
	assert.NotNil(t, vOut)
}

func TestValidateAgentRules_OperationalProof_CustomRulesDir(t *testing.T) {
	root := setupRulesTestProject(t)
	customDir := filepath.Join(root, ".agent", "rules")
	seedAgentRuleFile(t, customDir, "custom.mdc", []byte("custom content"))
	seedManifestFile(t, root, "version: 1\nrules:\n  - custom.mdc\n")

	out, err := executeValidateAgentRulesCmd(root, "--rules-dir", ".agent/rules", "--project-root", root)
	require.NoError(t, err)
	assert.NotNil(t, out)
}

func TestValidateAgentRules_NegativeBoundary_Mismatch(t *testing.T) {
	root := setupRulesTestProject(t)
	rulesDir := filepath.Join(root, ".ide", "rules")
	seedAgentRuleFile(t, rulesDir, "present.mdc", []byte("present"))
	seedManifestFile(t, root, "version: 1\nrules:\n  - present.mdc\n  - missing_from_disk.mdc\n")

	out, err := executeValidateAgentRulesCmd(root, "--project-root", root)
	require.Error(t, err)
	assert.NotNil(t, out)
	assert.Contains(t, err.Error(), "agent rules validation failed")
}
