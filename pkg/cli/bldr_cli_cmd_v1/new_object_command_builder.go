package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewNewObjectCommandBuilder creates a new new_object command.
// Spec codegen: add .zqk/cli/specs/new/ when the new/ tree is migrated off hand-maintained builders.
func NewNewObjectCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("object")
	builder.WithShort("Mint an object onto the draft plane (kind + title)")
	help := clipkg.DynamicHelpBuilder("Mint an object onto the draft plane (kind + title)")
	help.WithDescriptionLines("Persists kind+title onto .zqk/object_drafts/ as {id}.yaml (no CAS until a successful promote).")
	help.WithDescriptionLines("Default: stay on the draft plane (title-only is rarely promote-ready). Pass --promote to enqueue background promote.")
	help.WithDescriptionLines("Requirement, goal, and milestone mints auto-run workflow gen-trace-pipeline unless --skip-trace-pipeline.")
	help.WithDescriptionLines(paths.RewriteCanonicalCLIInvocations("For printable YAML scaffolds (edit offline), use: zqk object template <kind>"))
	help.AddExample("Mint a backlog item onto the draft plane", "%s new object backlog_item --title \"Explore draft plane\"")
	help.AddExample("Mint a requirement and scaffold CRIT/TST/BLI", "%s new object requirement --title \"Description required at CAS\"")
	help.AddExample("Mint and enqueue background promote", "%s new object agent_task --title \"Ready spike\" --promote")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(1))
	builder.AddStringFlag("title", "t", "", "Required: object title (persists onto draft plane)")
	builder.AddBoolFlag("promote", "", false, "Enqueue background object promote after mint (opt-in; often sticks on title-only)")
	builder.AddBoolFlag("skip-trace-pipeline", "", false, "Do not auto-run workflow gen-trace-pipeline after minting requirement, goal, or milestone")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	return cmd
}
