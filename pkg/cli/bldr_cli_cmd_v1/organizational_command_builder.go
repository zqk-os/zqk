package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewOrganizationalCommandBuilder creates a new organizational command
func NewOrganizationalCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for organizational")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
