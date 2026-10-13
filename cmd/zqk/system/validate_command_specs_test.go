package system

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupMockSpecsHierarchy(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	alphaSpec := filepath.Join(specsDir, "alpha_command.yaml")
	specContent := "name: alpha\nshort: Alpha command\ncommon_flags: true\n"
	if err := fileutil.WriteFile(alphaSpec, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	return dir, specsDir
}

func resolveBinaryForTest(t *testing.T) (string, string) {
	t.Helper()
	root := cli.ResolveProjectRoot(".")
	if root == "" {
		t.Skip("not running inside a valid project root")
	}
	binPath := filepath.Join(root, "bin", "zqk")
	if _, err := fileutil.Stat(binPath); err != nil {
		t.Skip("bin/zqk not found, skipping binary execution test")
	}
	return root, binPath
}

func setupRootWithValidateCmd() (*cobra.Command, *cobra.Command) {
	root := clipkg.NewCommandBuilder("zqk").Build()
	validateCmd := NewValidateCommandSpecsCmd()
	root.AddCommand(validateCmd)
	return root, validateCmd
}

func setupRootWithAlphaAndValidateCmd() (*cobra.Command, *cobra.Command) {
	root, validateCmd := setupRootWithValidateCmd()
	alphaCmd := clipkg.NewCommandBuilder("alpha").WithShort("Alpha command").Build()
	root.AddCommand(alphaCmd)
	return root, validateCmd
}

func TestNewValidateCommandSpecsCmd(t *testing.T) {
	cmd := NewValidateCommandSpecsCmd()
	if cmd.Use != "validate-command-specs" {
		t.Fatalf("expected Use=validate-command-specs, got %s", cmd.Use)
	}

	if cmd.Flag("specs-dir") == nil {
		t.Fatal("missing --specs-dir flag")
	}
	if cmd.Flag("baseline") == nil {
		t.Fatal("missing --baseline flag")
	}
	if cmd.Flag("write-baseline") == nil {
		t.Fatal("missing --write-baseline flag")
	}
}

func TestRunValidateCommandSpecs_MockHierarchy(t *testing.T) {
	dir, specsDir := setupMockSpecsHierarchy(t)

	baselineFile := filepath.Join(dir, "baseline.json")
	if err := fileutil.WriteFile(baselineFile, []byte(`{"commands_without_specs":[]}`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	_, validateCmd := setupRootWithAlphaAndValidateCmd()
	validateCmd.SetArgs([]string{
		"--specs-dir", specsDir,
		"--baseline", baselineFile,
	})

	if err := validateCmd.Execute(); err != nil {
		t.Fatalf("validateCmd failed on mock hierarchy: %v", err)
	}
}

func TestRunValidateCommandSpecs_InvalidSpecsDir(t *testing.T) {
	dir := t.TempDir()
	cmd := NewValidateCommandSpecsCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetArgs([]string{
		"--specs-dir", filepath.Join(dir, "nonexistent"),
	})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for nonexistent specs dir, got nil")
	}
}

func TestRunValidateCommandSpecs_BinaryIntegration(t *testing.T) {
	_, binPath := resolveBinaryForTest(t)

	out, err := testkit.ManagedCommand(t, t.Context(), binPath, "system", "validate-command-specs").CombinedOutput()
	if err != nil {
		t.Fatalf("zqk system validate-command-specs failed: %v\nOutput: %s", err, string(out))
	}
}

func TestCLITaxonomyGovernance_StandardsAndPersonas(t *testing.T) {
	root := cli.ResolveProjectRoot(".")
	if root == "" {
		t.Skip("not running inside a valid project root")
	}

	standardsDoc := filepath.Join(root, "docs", "architecture", "CLI_COMMAND_TAXONOMY_STANDARDS.md")
	data, err := fileutil.ReadFile(standardsDoc)
	if err != nil {
		t.Fatalf("failed to read CLI taxonomy standards doc: %v", err)
	}

	content := string(data)
	if len(content) < 500 {
		t.Fatalf("CLI taxonomy standards doc too short (%d bytes)", len(content))
	}

	requiredTokens := []string{
		"CLI Command Taxonomy Standards",
		"Rule 1: Strict Flat Hierarchy (depth <= 2)",
		"PER-COMMUNITY-PLATFORM-ENGINEER",
		"PER-COMMUNITY-APPLICATION-DEVELOPER",
		"PER-COMMUNITY-OPERATOR",
		"PER-COMMUNITY-DATA-SCIENTIST",
		"PER-COMMUNITY-SECURITY-AUDITOR",
	}

	for _, token := range requiredTokens {
		if !strings.Contains(content, token) {
			t.Errorf("standards doc missing required governance token: %q", token)
		}
	}
}

func TestCommandSpecPolicingAudit_ZeroNewDrift(t *testing.T) {
	_, binPath := resolveBinaryForTest(t)

	out, err := testkit.ManagedCommand(t, t.Context(), binPath, "system", "validate-command-specs", "--format", "json").CombinedOutput()
	if err != nil {
		t.Fatalf("zqk system validate-command-specs failed: %v\nOutput: %s", err, string(out))
	}

	var summary commandSpecCoverageSummary
	if err := json.Unmarshal(out, &summary); err != nil {
		t.Fatalf("failed to parse validate-command-specs output: %v\nOutput: %s", err, string(out))
	}

	if !summary.Valid {
		t.Fatalf("expected valid command spec coverage, got invalid: %+v", summary)
	}
	if summary.NewDriftCount != 0 {
		t.Fatalf("expected 0 new drift, got %d", summary.NewDriftCount)
	}
}

func TestRollupCLITaxonomyOverhaul_Integration(t *testing.T) {
	_, binPath := resolveBinaryForTest(t)

	testCases := [][]string{
		{"job", "--help"},
		{"service", "--help"},
		{"convergence", "--help"},
		{"agent", "swarm", "--help"},
		{"agent", "feed", "--help"},
		{"object", "spec", "--help"},
	}

	for _, tc := range testCases {
		out, err := testkit.ManagedCommand(t, t.Context(), binPath, tc...).CombinedOutput()
		if err != nil {
			t.Fatalf("command failed: zqk %v: %v\nOutput: %s", tc, err, string(out))
		}
		if len(out) == 0 {
			t.Errorf("command returned empty output: zqk %v", tc)
		}
	}
}

func TestRunValidateCommandSpecs_CoverageElevation(t *testing.T) {
	dir, specsDir := setupMockSpecsHierarchy(t)
	baselineFile := filepath.Join(dir, "baseline.json")

	// 1. Test --write-baseline
	_, validateCmd := setupRootWithAlphaAndValidateCmd()
	validateCmd.SetArgs([]string{
		"--specs-dir", specsDir,
		"--baseline", baselineFile,
		"--write-baseline",
		"-v",
		"-f", "json",
	})
	if err := validateCmd.Execute(); err != nil {
		t.Fatalf("write-baseline execution failed: %v", err)
	}

	// 2. Test invalid baseline format
	corruptBaseline := filepath.Join(dir, "corrupt.json")
	if err := fileutil.WriteFile(corruptBaseline, []byte("{invalid-json"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	root2, valCmd2 := setupRootWithValidateCmd()
	assert.NotNil(t, valCmd2)
	root2.SetArgs([]string{
		"validate-command-specs",
		"--specs-dir", specsDir,
		"--baseline", corruptBaseline,
	})
	if err := root2.Execute(); err == nil {
		t.Fatal("expected error on corrupt baseline, got nil")
	}

	// 3. Test parity failure when command has no spec
	betaCmd := clipkg.NewCommandBuilder("beta").WithShort("Beta command without spec").Build()
	root3, valCmd3 := setupRootWithAlphaAndValidateCmd()
	assert.NotNil(t, valCmd3)
	root3.AddCommand(betaCmd)
	root3.SetArgs([]string{
		"validate-command-specs",
		"--specs-dir", specsDir,
		"--baseline", baselineFile,
	})
	if err := root3.Execute(); err == nil {
		t.Fatal("expected error on missing spec for command, got nil")
	}

	// 4. Test defaults on existing project root
	rootProj := cli.ResolveProjectRoot(".")
	if rootProj != "" {
		root4, validateCmd4 := setupRootWithValidateCmd()
		root4.SetArgs([]string{"validate-command-specs"})
		cli.SetContext(validateCmd4, cli.ContextForProjectRoot(rootProj))
		execErr := root4.Execute()
		assert.NotNil(t, execErr)
	}

	// 5. Test write-baseline error on invalid path
	root5, valCmd5 := setupRootWithValidateCmd()
	assert.NotNil(t, valCmd5)
	root5.SetArgs([]string{
		"validate-command-specs",
		"--specs-dir", specsDir,
		"--baseline", "/invalid/nonexistent/dir/baseline.json",
		"--write-baseline",
	})
	if err := root5.Execute(); err == nil {
		t.Fatal("expected error when writing baseline to invalid path, got nil")
	}
}
