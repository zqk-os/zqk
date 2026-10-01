package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewPreCommitAggregateCommandBuilder creates a new pre_commit_aggregate command
func NewPreCommitAggregateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("aggregate")
	builder.WithShort("Merge category results into .zqk/pre-commit/results.json")
	help := clipkg.DynamicHelpBuilder("Merge category results into .zqk/pre-commit/results.json")
	help.WithDescriptionLines("Reads category files in .zqk/pre-commit/*.json (excluding results.json), sets block if any blocking category has !ok,")
	help.WithDescriptionLines("writes .zqk/pre-commit/results.json. Run after background check jobs or scripts.")
	help.AddExample("Merge category results into single file", "%s pre-commit aggregate")
	help.AddExample("Specify project root", "%s pre-commit aggregate --project-root /path/to/repo")
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
