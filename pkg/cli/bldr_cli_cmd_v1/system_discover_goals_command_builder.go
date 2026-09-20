package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemDiscoverGoalsCommandBuilder creates a new system_discover_goals command
func NewSystemDiscoverGoalsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for discover-goals")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
