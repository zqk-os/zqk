package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
)

// NewSystemFileLockCommandBuilder creates a new system_file_lock command
func NewSystemFileLockCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for file-lock")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
