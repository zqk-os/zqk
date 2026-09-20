package object

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/cmd/zqk/spec"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
)

const emptyValue = ""

// Help / discovery partitions for `zqk object -h` (verbs vs shortcuts vs schema kinds).
const (
	objectHelpGroupVerbs     = "verbs"
	objectHelpGroupGraph     = "graph"
	objectHelpGroupBatch     = "batch"
	objectHelpGroupShortcuts = "shortcuts"
	objectHelpGroupKinds     = "kinds"
)

// NewObjectCmd creates a new object command group
func NewObjectCmd() *cobra.Command {
	objectCmdLong := fmt.Sprintf(`Object operations for managing all object kinds in the system.

Help is partitioned so scrapers and operators do not confuse verbs with kinds:
  Core verbs: create, get, list, update, delete, count, …
  Shortcut groups (not schema kinds): splan, pplan, wstrans, evoman, draft
  Schema kinds: registered object_specs (also: %s object fields --list-kinds --format json)

Examples:
  # Common operations (work for all object kinds)
  %s object create backlog_item --file item.yaml
  %s object list backlog_item --filter status=exploring
  %s object get BLI-626
  %s object update BLI-626 --field status=validated
  %s object delete BLI-626
  %s object count backlog_item
  %s object backlog_item fields

  # Enumerate countable/schema kinds (machine-readable)
  %s object fields --list-kinds --format json

  # Specialized operations
  %s object bulk create backlog_item --file items.yaml
  %s object bulk update --file updates.yaml
  # Elevated access (Enterprise license or zqk-admin): include internal kinds
  %s object list --internal
  %s object create object_spec --internal --file spec.yaml`, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName)
	objectCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewObjectCommandBuilder(), &cobra.Command{
		Use:   "object",
		Short: "Object operations (CRUD, query, and management)",
		Long:  objectCmdLong,
	})
	objectCmd.AddGroup(
		&cobra.Group{ID: objectHelpGroupVerbs, Title: "Core Verbs:"},
		&cobra.Group{ID: objectHelpGroupGraph, Title: "Relationship Traversal:"},
		&cobra.Group{ID: objectHelpGroupBatch, Title: "Batch / Import-Export:"},
		&cobra.Group{ID: objectHelpGroupShortcuts, Title: "Shortcut Groups (not schema kinds):"},
		&cobra.Group{ID: objectHelpGroupKinds, Title: "Schema Kinds (use list/count <kind>; prefer fields --list-kinds for scrapes):"},
	)
	// Scheduler guard + declarative kind validation (see kind_validate_prerun.go; leaf commands set AnnotationKindValidate).
	objectCmd.PersistentPreRunE = runObjectSchedulerGuard
	objectCmd.PersistentFlags().Bool("allow-degraded", false, "Allow scheduler-dependent commands to run when scheduler daemon is not running")
	// Elevated access mode for built-in and internal kinds. Not visibility:internal filter.
	objectCmd.PersistentFlags().Bool(FlagElevatedInternal, false, "Elevated access mode for built-in and internal kinds (requires Enterprise license or zqk-admin)")

	add := func(cmd *cobra.Command, group string) {
		cmd.GroupID = group
		objectCmd.AddCommand(cmd)
	}

	// Core verbs
	add(NewCreateCmd(), objectHelpGroupVerbs)
	add(NewGetCmd(), objectHelpGroupVerbs)
	add(NewListCmd(), objectHelpGroupVerbs)
	add(NewUpdateCmd(), objectHelpGroupVerbs)
	add(NewPromoteCmd(), objectHelpGroupVerbs)
	add(NewDemoteCmd(), objectHelpGroupVerbs)
	add(NewParkCmd(), objectHelpGroupVerbs)
	add(NewDeleteCmd(), objectHelpGroupVerbs)
	add(NewMoveCmd(), objectHelpGroupVerbs)
	add(NewRenameCmd(), objectHelpGroupVerbs)
	add(NewCountCmd(), objectHelpGroupVerbs)
	add(NewTemplateCmd(), objectHelpGroupVerbs)
	add(NewObjectRootFieldsCmd(), objectHelpGroupVerbs)

	// Kind placeholder (visible since real kinds are hidden to reduce scrape surface)
	kindPlaceholder := NewKindCmd()
	kindPlaceholder.Hidden = false
	kindPlaceholder.GroupID = objectHelpGroupKinds
	objectCmd.AddCommand(kindPlaceholder)

	// Relationship traversal
	add(NewRefCmd(), objectHelpGroupGraph)
	add(NewRelatedCmd(), objectHelpGroupGraph)
	add(NewPathCmd(), objectHelpGroupGraph)
	add(NewNeighborsCmd(), objectHelpGroupGraph)

	// Batch / import-export
	add(NewBulkCmd(), objectHelpGroupBatch)
	add(NewExportCmd(), objectHelpGroupBatch)
	add(NewImportCmd(), objectHelpGroupBatch)
	add(NewDaemonCmd(), objectHelpGroupBatch)

	// Shortcut groups (not schema kinds for list/count)
	add(NewDraftCmd(), objectHelpGroupShortcuts)
	add(NewPPlanCmd(), objectHelpGroupShortcuts)
	add(NewSPlanCmd(), objectHelpGroupShortcuts)
	add(NewWstransCmd(), objectHelpGroupShortcuts)
	add(NewEvomanCmd(), objectHelpGroupShortcuts)
	add(spec.NewSpecCmd(), objectHelpGroupShortcuts)

	return objectCmd
}
