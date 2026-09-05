package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemPolicyInterruptsCommandBuilder creates a new system_policy_interrupts command
func NewSystemPolicyInterruptsCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for policy-interrupts")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
