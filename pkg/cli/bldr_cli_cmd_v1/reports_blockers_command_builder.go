package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewReportsBlockersCommandBuilder creates a new reports_blockers command
func NewReportsBlockersCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for blockers")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
