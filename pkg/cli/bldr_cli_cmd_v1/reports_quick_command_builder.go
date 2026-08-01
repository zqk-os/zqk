package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewReportsQuickCommandBuilder creates a new reports_quick command
func NewReportsQuickCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for quick")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
