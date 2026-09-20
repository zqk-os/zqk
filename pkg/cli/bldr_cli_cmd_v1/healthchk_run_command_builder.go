package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewHealthchkRunCommandBuilder creates a new healthchk_run command
func NewHealthchkRunCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for run")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
