package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewReportsEddCommandBuilder creates a new reports_edd command
func NewReportsEddCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for edd")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
