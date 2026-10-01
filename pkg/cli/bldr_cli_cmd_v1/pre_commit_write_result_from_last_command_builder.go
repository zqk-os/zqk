package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewPreCommitWriteResultFromLastCommandBuilder creates a new pre_commit_write_result_from_last command
func NewPreCommitWriteResultFromLastCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("write-result-from-last")
	builder.WithShort("Read staging file and write category result then aggregate (for pre-commit callback)")
	help := clipkg.DynamicHelpBuilder("Read staging file and write category result then aggregate (for pre-commit callback)")
	help.WithDescriptionLines("Reads .zqk/pre-commit/.last-<category>.json written by the check script (lint, policy, integrity),")
	help.WithDescriptionLines("writes the category file, then runs aggregate. Used by scheduler job callback when the job was")
	help.WithDescriptionLines("triggered with --pre-commit so the hook sees up-to-date results.")
	help.AddExample("Write lint category from last run and aggregate (callback)", "%s pre-commit write-result-from-last --category=lint")
	help.AddExample("Write policy category from last run and aggregate", "%s pre-commit write-result-from-last --category=policy")
	help.AddExample("Write integrity category from last run and aggregate", "%s pre-commit write-result-from-last --category=integrity")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("category", "", "", "Category (lint, policy, integrity)")
	builder.AddStringFlag("project-root", "", "", "Project root (default: resolve from cwd)")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
