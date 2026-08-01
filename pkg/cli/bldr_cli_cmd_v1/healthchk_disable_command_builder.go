package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewHealthchkDisableCommandBuilder creates a new healthchk_disable command
func NewHealthchkDisableCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for disable")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
