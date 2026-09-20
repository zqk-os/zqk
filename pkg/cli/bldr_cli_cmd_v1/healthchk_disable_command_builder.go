package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewHealthchkDisableCommandBuilder creates a new healthchk_disable command
func NewHealthchkDisableCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for disable")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
