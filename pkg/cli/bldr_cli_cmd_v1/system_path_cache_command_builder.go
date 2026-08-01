package bldr_cli_cmd_v1

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/spf13/cobra"
)

// NewSystemPathCacheCommandBuilder creates a new system_path_cache command
func NewSystemPathCacheCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("")
	builder.WithShort("Generated spec for path-cache")
	builder.WithCommonFlags(false, nil)
	cmd := builder.Build()
	return cmd
}
