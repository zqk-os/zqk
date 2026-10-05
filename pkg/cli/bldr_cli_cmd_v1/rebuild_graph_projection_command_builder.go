package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewRebuildGraphProjectionCommandBuilder creates a new rebuild_graph_projection command
func NewRebuildGraphProjectionCommandBuilder() *cobra.Command {
	builder := clipkg.NewCommandBuilder("rebuild-graph-projection")
	builder.WithShort("Rebuild MemGraph projection from file SSOT")
	help := clipkg.DynamicHelpBuilder("Rebuild MemGraph projection from file SSOT")
	help.WithDescriptionLines("When STORAGE_MODE=file+projection, upserts all file-backed objects into the")
	help.WithDescriptionLines("graph projection and deletes projection orphans not present in file SSOT.")
	help.WithDescriptionLines("Also ensures spec-derived id indexes. Use --dry-run to count without writes.")
	help.AddExample("Rebuild projection from file durability store", "%s system rebuild-graph-projection")
	help.AddExample("Preview counts only", "%s system rebuild-graph-projection --dry-run")
	help.ExcludeFlag("columns")
	help.ExcludeFlag("ignore-scheduler-down")
	builder.WithHelpBuilder(help)
	builder.WithArgs(cobra.NoArgs)
	builder.AddBoolFlag("dry-run", "", false, "Count rebuilds and orphans without writing to the projection")
	builder.WithCommonFlagsExcluding(cli.AddCommonFlagsExcluding, []string{"columns", "ignore-scheduler-down"})
	cmd := builder.Build()
	return cmd
}
