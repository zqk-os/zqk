package app

import (
	"path/filepath"
	"testing"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCommandSpecParity(t *testing.T) {
	ensureCommandsRegistered()
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	projectRoot := filepath.Clean(filepath.Join(cwd, "../../.."))
	specsDir := filepath.Join(projectRoot, ".zqk/cli/specs")
	coverage, err := clipkg.AnalyzeCommandSpecCoverage(rootCmd, specsDir)
	if err != nil {
		t.Fatalf("analyze command-spec coverage: %v", err)
	}
	// registerCommands keeps GroupID=admin commands on *.test binaries so unit tests can
	// exercise them; the shipped community binary strips them. Parity must match that surface.
	coverage.CommandsWithoutSpecs = excludeAdminGroupCommands(rootCmd, coverage.CommandsWithoutSpecs)
	baselinePath := filepath.Join(projectRoot, ".zqk/cli/command_spec_coverage_baseline.community.json")
	baseline, err := clipkg.LoadCommandSpecCoverageBaseline(baselinePath)
	if err != nil {
		t.Fatalf("load command-spec baseline: %v", err)
	}
	coverage = clipkg.ApplyCommandSpecCoverageBaseline(coverage, baseline, baselinePath)
	if !coverage.Valid {
		t.Fatalf(
			"command-spec parity failed: new_commands_without_specs=%v new_specs_without_commands=%v",
			coverage.NewCommandsWithoutSpecs,
			coverage.NewSpecsWithoutCommands,
		)
	}
}
