package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewPreCommitClearCommandBuilder creates a new pre_commit_clear command
func NewPreCommitClearCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("clear")
	builder.WithShort("Clear pre-commit results after resolving blockers")
	help := clipkg.DynamicHelpBuilder("Clear pre-commit results after resolving blockers")
	help.WithDescriptionLines("Removes all category and results files under .zqk/pre-commit/ and writes a clean")
	help.WithDescriptionLines("results.json with block=false so the hook allows commits. Run after fixing issues;")
	help.WithDescriptionLines("the next background script run will repopulate categories.")
	help.AddExample("Clear results after resolving blockers", "%s pre-commit clear")
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
