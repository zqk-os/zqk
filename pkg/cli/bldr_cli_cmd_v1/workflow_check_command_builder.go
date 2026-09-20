package bldr_cli_cmd_v1

import (
	"fmt"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1/check"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

const (
	checkShortDesc       = "Check current workflow state and configuration"
	checkDescription     = "Verify the current workflow is ready for execution by checking:\n  - Valid session context\n  - Active priority plan alignment\n  - Backlog item status and routing\n  - Policy compliance state"
	checkExampleUsage    = "%s workflow check"
	checkExampleLabel    = "Run workflow check"
	checkErrorWrapSuffix = "workflow check failed: %w"
)

// NewWorkflowCheckCommandBuilder creates a new workflow_check command
// TRACK: BLI-1788841705527001000-d18ec5a6 / REQ-CEF-ARCH-001
func NewWorkflowCheckCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("check")
	builder.WithShort(checkShortDesc)
	help := clipkg.DynamicHelpBuilder(checkShortDesc)
	help.WithDescriptionLines(checkDescription)
	help.AddExample(checkExampleLabel, checkExampleUsage)
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.ExactArgs(0))
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()

	// Wire run handler to execute the check workflow
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		result, err := check.NewWorkflowChecker().Run()
		if err != nil {
			return fmt.Errorf(checkErrorWrapSuffix, err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), result.Summary())
		return nil
	}

	return cmd
}
