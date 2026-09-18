package system

import (
	"path/filepath"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/spf13/cobra"
)

const canonicalCommandSpecsRelativePath = ".zqk/cli/specs"
const commandSpecCoverageBaselineRelativePath = ".zqk/cli/command_spec_coverage_baseline.json"

type commandSpecCoverageSummary struct {
	Valid                    bool   `json:"valid" yaml:"valid"`
	Parity                   bool   `json:"parity" yaml:"parity"`
	LoadedCommandCount       int    `json:"loaded_command_count" yaml:"loaded_command_count"`
	CommandSpecCount         int    `json:"command_spec_count" yaml:"command_spec_count"`
	CommandsWithoutSpecCount int    `json:"commands_without_spec_count" yaml:"commands_without_spec_count"`
	SpecsWithoutCommandCount int    `json:"specs_without_command_count" yaml:"specs_without_command_count"`
	NewDriftCount            int    `json:"new_drift_count" yaml:"new_drift_count"`
	ResolvedDriftCount       int    `json:"resolved_drift_count" yaml:"resolved_drift_count"`
	BaselinePath             string `json:"baseline_path" yaml:"baseline_path"`
}

// NewValidateCommandSpecsCmd creates the command-spec parity gate.
func NewValidateCommandSpecsCmd() *cobra.Command {
	command := bldr_cli_cmd_v1.NewSystemValidateCommandSpecsCommandBuilder()
	command.RunE = runValidateCommandSpecs
	return command
}

func runValidateCommandSpecs(command *cobra.Command, _ []string) error {
	specsDir, err := command.Flags().GetString("specs-dir")
	if err != nil {
		return errfmt.Newf("read specs-dir flag").Wrap(err)
	}
	baselinePath, err := command.Flags().GetString("baseline")
	if err != nil {
		return errfmt.Newf("read baseline flag").Wrap(err)
	}
	writeBaseline, err := command.Flags().GetBool("write-baseline")
	if err != nil {
		return errfmt.Newf("read write-baseline flag").Wrap(err)
	}

	ctx := cli.GetContext(command)
	projectRoot := emptyValue
	if ctx != nil {
		projectRoot = ctx.ProjectRoot
	}
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return errfmt.Errorf("not a ZQK project (no project root found)")
	}
	if specsDir == emptyValue {
		specsDir = filepath.Join(projectRoot, canonicalCommandSpecsRelativePath)
	}
	if baselinePath == emptyValue {
		baselinePath = filepath.Join(projectRoot, commandSpecCoverageBaselineRelativePath)
	}

	coverage, err := clipkg.AnalyzeCommandSpecCoverage(command.Root(), specsDir)
	if err != nil {
		return errfmt.Newf("analyze command-spec coverage").Wrap(err)
	}
	if writeBaseline {
		if err := clipkg.WriteCommandSpecCoverageBaseline(baselinePath, coverage); err != nil {
			return errfmt.Newf("write command-spec coverage baseline").Wrap(err)
		}
	}
	baseline, err := clipkg.LoadCommandSpecCoverageBaseline(baselinePath)
	if err != nil {
		return errfmt.Newf("load command-spec coverage baseline").Wrap(err)
	}
	coverage = clipkg.ApplyCommandSpecCoverageBaseline(coverage, baseline, baselinePath)
	output := any(commandSpecCoverageSummary{
		Valid:                    coverage.Valid,
		Parity:                   coverage.Parity,
		LoadedCommandCount:       coverage.LoadedCommandCount,
		CommandSpecCount:         coverage.CommandSpecCount,
		CommandsWithoutSpecCount: len(coverage.CommandsWithoutSpecs),
		SpecsWithoutCommandCount: len(coverage.SpecsWithoutCommands),
		NewDriftCount:            len(coverage.NewCommandsWithoutSpecs) + len(coverage.NewSpecsWithoutCommands),
		ResolvedDriftCount:       len(coverage.ResolvedCommands) + len(coverage.ResolvedSpecs),
		BaselinePath:             coverage.BaselinePath,
	})
	if cli.IsVerbose(command) {
		output = coverage
	}
	if err := cli.FormatOutput(command, output); err != nil {
		return err
	}
	if !coverage.Valid {
		return errfmt.Errorf(
			"command-spec parity failed: %d commands lack specs; %d specs lack commands",
			len(coverage.CommandsWithoutSpecs),
			len(coverage.SpecsWithoutCommands),
		)
	}
	return nil
}
