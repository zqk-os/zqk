package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewHealthchkUpdateCommandBuilder creates a new healthchk_update command
func NewHealthchkUpdateCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for update")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
