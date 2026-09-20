package validate

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
)

// NewValidateCmd creates the validate command group.
func NewValidateCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewValidateObjectCommandBuilder(), &cobra.Command{
		Use:   "validate",
		Short: "Validation and verification commands",
		Long:  "Unified validation surface area for verifying artifacts, workflows, agents, and matrices.",
	})
	cmd.AddCommand(NewVerifyCompletionCmd())
	cmd.AddCommand(NewValidateAgentCmd())
	cmd.AddCommand(NewStewardCmd())
	cmd.AddCommand(NewBuildPromptCmd())
	cmd.AddCommand(NewCheckCmd())
	cmd.AddCommand(NewTreeCmd())
	cmd.AddCommand(NewMatrixValidateCmd())
	cmd.AddCommand(NewValidateObjectCmd())
	return cmd
}
