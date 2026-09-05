package bldr_cli_cmd_v1

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewPreCommitStatusCommandBuilder creates a new pre_commit_status command
func NewPreCommitStatusCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("status")
	builder.WithShort("Show pre-commit results status and required actions")
	help := clipkg.DynamicHelpBuilder("Show pre-commit results status and required actions")
	help.WithDescriptionLines("Reads .zqk/pre-commit/results.json and prints whether the hook would block,")
	help.WithDescriptionLines("when results were last updated, and what to do if setup is missing or checks failed.")
	help.AddExample("Show status and required actions", "%s pre-commit status")
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
