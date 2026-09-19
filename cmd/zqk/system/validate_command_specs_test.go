package system

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
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
	if err := fileutil.MkdirAll(specsDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create sample spec for mock command "alpha"
	alphaSpec := filepath.Join(specsDir, "alpha_command.yaml")
	specContent := `name: alpha
short: Alpha command
common_flags: true
`
	if err := fileutil.WriteFile(alphaSpec, []byte(specContent), 0644); err != nil {
		t.Fatal(err)
	}

	baselineFile := filepath.Join(dir, "baseline.json")
	if err := fileutil.WriteFile(baselineFile, []byte(`{"commands_without_specs":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	root := &cobra.Command{Use: "zqk"}
	alphaCmd := &cobra.Command{Use: "alpha", Short: "Alpha command"}
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

	out, err := exec.Command(binPath, "system", "validate-command-specs").CombinedOutput()
	if err != nil {
		t.Fatalf("zqk system validate-command-specs failed: %v\nOutput: %s", err, string(out))
	}
}
