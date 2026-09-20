package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemLaunchAssessmentCommandBuilder creates a new system_launch_assessment command
func NewSystemLaunchAssessmentCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for launch-assessment")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
