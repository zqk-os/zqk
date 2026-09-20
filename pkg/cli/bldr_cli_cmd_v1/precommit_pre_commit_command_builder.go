package bldr_cli_cmd_v1

import (
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewPrecommitPreCommitCommandBuilder creates a new precommit_pre_commit command
func NewPrecommitPreCommitCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for pre-commit")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
