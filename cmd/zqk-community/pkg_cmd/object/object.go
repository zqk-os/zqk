package object

import (
	"fmt"

	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/spf13/cobra"
)

const emptyValue = ""

// NewObjectCmd creates a new object command group
func NewObjectCmd() *cobra.Command {
	objectCmdLong := fmt.Sprintf(`Object operations for managing all object kinds in the system.

This command group provides operations that work across all object kinds:
  Common operations: create, get, list, update, delete, count
  Relationship traversal: related, path, neighbors
  Specialized operations: bulk (batch operations)
  Kind-based operations: <kind> fields (explore fields for a specific kind)

Examples:
  # Common operations (work for all object kinds)
  %s object create backlog_item --file item.yaml
  %s object list backlog_item --filter status=exploring
  %s object get ITEM-626
  %s object update ITEM-626 --field status=validated
  %s object delete ITEM-626
  %s object count backlog_item
  %s object backlog_item fields

  # Specialized operations
  %s object bulk create backlog_item --file items.yaml
  %s object bulk update --file updates.yaml`, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName)
	objectCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObjectCommandBuilder(), &cobra.Command{
		Use:   "object",
		Short: "Object operations (CRUD, query, and management)",
		Long:  objectCmdLong,
	})
	// Scheduler guard + declarative kind validation (see kind_validate_prerun.go; leaf commands set AnnotationKindValidate).
	objectCmd.PersistentPreRunE = runObjectSchedulerGuard
	objectCmd.PersistentFlags().Bool("allow-degraded", false, "Allow scheduler-dependent commands to run when scheduler daemon is not running")

	// Common operations (work for all object kinds)
	objectCmd.AddCommand(NewCreateCmd())
	objectCmd.AddCommand(NewGetCmd())
	objectCmd.AddCommand(NewListCmd())
	objectCmd.AddCommand(NewUpdateCmd())
	objectCmd.AddCommand(NewPromoteCmd())
	objectCmd.AddCommand(NewDemoteCmd())
	objectCmd.AddCommand(NewDeleteCmd())
	objectCmd.AddCommand(NewMoveCmd())
	objectCmd.AddCommand(NewRenameCmd())
	objectCmd.AddCommand(NewCountCmd())
	objectCmd.AddCommand(NewTemplateCmd())
	objectCmd.AddCommand(NewObjectRootFieldsCmd())

	// Kind-based command group (allows "object <kind> fields")
	objectCmd.AddCommand(NewKindCmd())

	// Relationship traversal operations
	objectCmd.AddCommand(NewRelatedCmd())
	objectCmd.AddCommand(NewPathCmd())
	objectCmd.AddCommand(NewNeighborsCmd())

	// Specialized operations
	objectCmd.AddCommand(NewBulkCmd())
	objectCmd.AddCommand(NewExportCmd())
	objectCmd.AddCommand(NewImportCmd())
	objectCmd.AddCommand(NewPPlanCmd())
	objectCmd.AddCommand(NewSPlanCmd())
	objectCmd.AddCommand(NewWstransCmd())
	objectCmd.AddCommand(NewEvomanCmd())

	return objectCmd
}
