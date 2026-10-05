package bldr_cli_cmd_v1

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
)

// NewObjectDeleteCommandBuilder creates a new object_delete command
func NewObjectDeleteCommandBuilder() *cobra.Command {
	builder := clipkg.NewCRUDCommandBuilder("delete", "delete <id> [id...]")
	builder.WithDryRunFlag()
	builder.WithCascadeFlag()
	builder.WithUnlinkReferencesFlag()
	builder.WithCommonFlagsDefault(cli.AddCommonFlags)
	builder.WithShort("Delete one or more objects by ID")
	help := clipkg.DynamicHelpBuilder("Delete one or more objects by ID")
	help.WithDescriptionLines("Delete one or more objects by ID.")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("IDs may be positional args, comma-separated, --ids, and/or --file (YAML array).")
	help.WithDescriptionLines("Multiple IDs use the same storage BulkDelete path (graph auto-batches).")
	help.WithDescriptionLines("")
	help.WithDescriptionLines("By default, deletion will fail if the object has dependents (objects that reference it).")
	help.WithDescriptionLines("Use --unlink-references to remove this ID from dependents' reference fields (CASCADE_NULLIFY-style), then delete.")
	help.WithDescriptionLines("Use --cascade to delete the object and all its dependents recursively.")
	help.WithDescriptionLines("Do not combine --unlink-references with --cascade.")
	help.WithDescriptionLines("Core kernel kinds (workstream, criteria, goals, plans, …) refuse hard delete unless --reason-code is set; prefer archive + aggregate/compress with lineage.")
	help.AddExample("Delete an object (fails if it has dependents)", "%s delete BLI-626")
	help.AddExample("Delete many from a file", "%s delete --file ids.yaml")
	help.AddExample("Delete multiple IDs", "%s delete BLI-001 BLI-002 --ids BLI-003")
	help.AddExample("Drop references from dependents (nullify refs), then delete", "%s delete CRIT-002 --unlink-references")
	help.AddExample("Delete with cascade (deletes object and all dependents)", "%s delete BLI-626 --cascade")
	help.AddExample("Dry-run to see what would be deleted", "%s delete BLI-626 --cascade --dry-run")
	help.AddExample("Break-glass hard delete of a core kind (requires justification)", "%s delete WS-010 --reason-code \"duplicate fixture WS after migration audit 2026-08-03\"")
	help.ExcludeFlag("format")
	help.ExcludeFlag("output")
	help.ExcludeFlag("verbose")
	help.ExcludeFlag("quiet")
	help.ExcludeFlag("timeout")
	help.ExcludeFlag("columns")
	builder.WithHelpBuilder(help)
	cmd := builder.Build()
	// Native multi-ID surface (bulk delete is deprecated): same flags as object bulk delete.
	cmd.Flags().String("ids", "", "Comma-separated list of object IDs")
	cmd.Flags().String("file", "", "Path to YAML file containing an array of IDs")
	cmd.Flags().String("reason-code", "", "Required for hard-delete of core kernel kinds (min 30 chars); prefer archive+aggregate instead")
	return cmd
}
