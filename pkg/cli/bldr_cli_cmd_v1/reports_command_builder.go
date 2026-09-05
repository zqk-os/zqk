package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewReportsCommandBuilder creates a new reports command
func NewReportsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for reports")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
