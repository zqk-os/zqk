package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemSubmitTdeBashCommandBuilder creates a new system_submit_tde_bash command
func NewSystemSubmitTdeBashCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for submit-tde-bash")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
