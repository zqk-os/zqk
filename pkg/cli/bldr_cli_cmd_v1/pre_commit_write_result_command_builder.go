package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewPreCommitWriteResultCommandBuilder creates a new pre_commit_write_result command
func NewPreCommitWriteResultCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("write-result")
	builder.WithShort("Write a category result file for pre-commit background checks")
	help := clipkg.DynamicHelpBuilder("Write a category result file for pre-commit background checks")
	help.WithDescriptionLines("Writes .zqk/pre-commit/<category>.json. Use from scripts or scheduler jobs after")
	help.WithDescriptionLines("running a check (e.g. linter). Then run pre-commit aggregate to merge.")
	help.AddExample("Write lint passed", "%s pre-commit write-result --category=lint --ok=true --summary=passed")
	help.AddExample("Write lint failed", "%s pre-commit write-result --category=lint --ok=false --summary=\"3 issues\"")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.AddStringFlag("project-root", "", "", "Project root (default: resolve from cwd)")
	builder.AddStringFlag("category", "", "", "Category name (lint, integrity, policy, docman)")
	builder.AddBoolFlag("ok", "", false, "Check passed")
	builder.AddStringFlag("summary", "", "", "Short summary (e.g. 3 lint issues)")
	builder.AddBoolFlag("blocking", "", true, "If true and !ok, commit is blocked")
	builder.AddStringArrayFlag("details", "", "Optional detail lines")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
