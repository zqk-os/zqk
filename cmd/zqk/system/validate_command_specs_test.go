package system

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

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
	dir := t.TempDir()
	specsDir := filepath.Join(dir, "specs")
	if err := fileutil.MkdirAll(specsDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	// Create sample spec for mock command "alpha"
	alphaSpec := filepath.Join(specsDir, "alpha_command.yaml")
	specContent := `name: alpha
short: Alpha command
common_flags: true
`
	if err := fileutil.WriteFile(alphaSpec, []byte(specContent), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	baselineFile := filepath.Join(dir, "baseline.json")
	if err := fileutil.WriteFile(baselineFile, []byte(`{"commands_without_specs":[]}`), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	root := clipkg.NewCommandBuilder("zqk").Build()
	alphaCmd := clipkg.NewCommandBuilder("alpha").WithShort("Alpha command").Build()
	validateCmd := NewValidateCommandSpecsCmd()

	root.AddCommand(alphaCmd)
	root.AddCommand(validateCmd)

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
	cmd.SetArgs([]string{
		"--specs-dir", filepath.Join(dir, "nonexistent"),
	})

	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for nonexistent specs dir, got nil")
	}
}

func TestRunValidateCommandSpecs_BinaryIntegration(t *testing.T) {
	root := cli.ResolveProjectRoot(".")
	if root == "" {
		t.Skip("not running inside a valid project root")
	}

	binPath := filepath.Join(root, "bin", "zqk")
	if _, err := fileutil.Stat(binPath); err != nil {
		t.Skip("bin/zqk not found, skipping full binary execution test")
	}

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
		"DOC-CLI-COMMAND-TAXONOMY-STANDARDS-001",
		"PER-INFORMATION-ARCHITECT",
		"PER-TECHNICAL-DOCUMENTARIAN",
		"REQ-CLI-TAXONOMY-HARMONIZATION-001",
		"100% Declarative Spec Coverage Mandatory",
	}

	for _, token := range requiredTokens {
		if !strings.Contains(content, token) {
			t.Errorf("standards doc missing required governance token: %q", token)
		}
	}
}

func TestCommandSpecPolicingAudit_ZeroNewDrift(t *testing.T) {
	root := cli.ResolveProjectRoot(".")
	if root == "" {
		t.Skip("not running inside a valid project root")
	}

	binPath := filepath.Join(root, "bin", "zqk")
	if _, err := fileutil.Stat(binPath); err != nil {
		t.Skip("bin/zqk not found, skipping binary audit test")
	}

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
	root := cli.ResolveProjectRoot(".")
	if root == "" {
		t.Skip("not running inside a valid project root")
	}

	binPath := filepath.Join(root, "bin", "zqk")
	if _, err := fileutil.Stat(binPath); err != nil {
		t.Skip("bin/zqk not found, skipping integration test")
	}

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
