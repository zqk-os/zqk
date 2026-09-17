// path_cache.go: CLI command to ensure path alias cache is built or refreshed for the project root.
// Command structure from spec: .zqk/cli/specs/system/path_cache_command.yaml (builder: bldr_cli_cmd_v1.NewSystemPathCacheCommandBuilder).
package system

import (
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/spf13/cobra"
)

// NewPathCacheCmd creates the path-cache check/refresh command from the command spec.
func NewPathCacheCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemPathCacheCommandBuilder(), &cobra.Command{Use: "path-cache"})
	cmd.Args = cobra.NoArgs
	cmd.RunE = runPathCache
	return cmd
}

func runPathCache(cmd *cobra.Command, _ []string) error {
	return RunPathCacheViaPipeline(cmd, nil)
}
