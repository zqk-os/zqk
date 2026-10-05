package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewPreCommitLintReportCommandBuilder creates a new pre_commit_lint_report command
func NewPreCommitLintReportCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("lint-report")
	builder.WithShort("Show last lint output for creating backlog items")
	help := clipkg.DynamicHelpBuilder("Show last lint output for creating backlog items")
	help.WithDescriptionLines("Prints the contents of .zqk/pre-commit/lint-output.txt from the last run of")
	help.WithDescriptionLines("scripts/pre-commit-lint.sh. Use this to see which issues to fix and create")
	help.WithDescriptionLines("backlog items. If the file is missing, run ./scripts/pre-commit-lint.sh first.")
	help.AddExample("View lint output after running pre-commit-lint.sh", "%s pre-commit lint-report")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("project-root", "", "", "Project root (default: resolve from cwd)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
