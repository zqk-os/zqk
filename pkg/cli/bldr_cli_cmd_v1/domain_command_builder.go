package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewDomainCommandBuilder creates a new domain command
func NewDomainCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for domain")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
