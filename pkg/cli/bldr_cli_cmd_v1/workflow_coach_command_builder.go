package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewWorkflowCoachCommandBuilder creates a new workflow_coach command
func NewWorkflowCoachCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for coach")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
