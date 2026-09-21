package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewPrintIDEPasteApplescriptCommandBuilder creates a new print_ide_paste_applescript command
func NewPrintIDEPasteApplescriptCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("print-ide-paste-applescript")
	builder.WithShort("Print AppleScript for agent chat paste (same as --paste-ide; honors ZQK_CURSOR_PASTE_PREFIX_STEPS)")
	help := clipkg.DynamicHelpBuilder("Print AppleScript for agent chat paste (same as --paste-ide; honors ZQK_CURSOR_PASTE_PREFIX_STEPS)")
	help.WithDescriptionLines(paths.RewriteCanonicalCLIInvocations("Prints the AppleScript used by `zqk scheduler convergence measure --format agent-prompt --paste-ide`."))
	help.WithDescriptionLines("Respects `ZQK_CURSOR_PASTE_PREFIX_STEPS`; default tokens: cmd_shift_e, escape, option_cmd_e, option_cmd_e.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines(paths.RewriteCanonicalCLIInvocations("For shell: `zqk scheduler print-ide-paste-applescript | osascript -`"))
	help.AddExample("Pipe generated script to osascript", "%s scheduler print-ide-paste-applescript | osascript -")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"format", "output", "verbose", "quiet", "timeout", "columns"})
	cmd := builder.Build()
	cli.RequireSession(cmd, false)
	cli.RequireSchedulerCheck(cmd, false)
	return cmd
}
