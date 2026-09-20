package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewReportsTestCommandBuilder creates a new reports_test command
func NewReportsTestCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for test")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
