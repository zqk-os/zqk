package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemRecoverCasCommandBuilder creates a new system_recover_cas command
func NewSystemRecoverCasCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for recover-cas")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
