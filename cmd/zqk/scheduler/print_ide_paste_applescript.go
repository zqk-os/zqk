package scheduler

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewPrintIDEPasteApplescriptCmd wires RunE for the spec-generated builder;
// YAML: .zqk/cli/specs/scheduler/print_ide_paste_applescript_command.yaml (run_e: runPrintIDEPasteApplescript).
//
// Prints the AppleScript used by `zqk scheduler convergence measure --format agent-prompt --paste-ide`.
// Respects ZQK_CURSOR_PASTE_PREFIX_STEPS; default tokens: cmd_shift_e,escape,option_cmd_e,option_cmd_e.
// For shell: zqk scheduler print-ide-paste-applescript | osascript -
func NewPrintIDEPasteApplescriptCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSchedulerPrintIDEPasteApplescriptCommandBuilder()
	cmd.RunE = runPrintIDEPasteApplescript
	// Legacy name still used by wake-cursor-tpm.sh / mesh scripts. Without this alias,
	// cobra prints scheduler help to stdout (exit 0) and `| osascript -` fails with -2740.
	cmd.Aliases = append(cmd.Aliases, "print-cursor-paste-applescript")
	return cmd
}

func runPrintIDEPasteApplescript(cmd *cobra.Command, _ []string) error {
	s, err := buildAppleScriptIDEPaste()
	if err != nil {
		return err
	}
	return cli.WriteOutput(cmd, []byte(s))
}
