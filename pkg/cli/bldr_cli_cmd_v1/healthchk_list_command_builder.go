package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewHealthchkListCommandBuilder creates a new healthchk_list command
func NewHealthchkListCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for list")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
