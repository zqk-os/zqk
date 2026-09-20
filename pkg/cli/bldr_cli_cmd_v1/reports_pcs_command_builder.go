package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewReportsPcsCommandBuilder creates a new reports_pcs command
func NewReportsPcsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for pcs")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
