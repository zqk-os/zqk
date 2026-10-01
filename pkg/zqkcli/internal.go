package internal

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
)

const emptyValue = ""

// NewInternalCmd creates a new internal command with subcommands
func NewInternalCmd() *cobra.Command {
	internalCmdLong := fmt.Sprintf(`Manage internal and built-in objects with privileged access.

Deprecated: use '%s object … --internal' (license-gated elevated mode; zqk-admin allowed as transitional carrier).
See docs/strategy/open-core/OBJECT_PLANE_INTERNAL_FLAG.md.

This command provides admin-only access to:
  - Built-in objects (e.g., COMP-TYPE-* component types) - normally immutable
  - Internal objects (objects with visibility: internal)
  - System objects in _internal directories

Built-in objects are shareable/reusable but immutable unless accessed through
this command with admin privileges.

Command organization:
  Common operations: list, get, create, update, delete, bulk (work for all internal object kinds)
  Kind-based operations: <kind> fields (explore fields for a specific internal kind)
  Specialized operations: (future: lifecycle management, spec management, etc.)

Examples:
  # Prefer object elevated mode
  %s object list --internal
  %s object create object_spec --internal --file spec.yaml
  # Legacy (deprecated)
  %s internal list component --built-in
	%s internal get COMP-TYPE-001`, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName, paths.CLICommandName)
	internalCmd := clipkg.NewCommandBuilder("internal").
		WithShort("Deprecated: use object … --internal").
		WithLong(internalCmdLong).
		Build()
	internalCmd.Deprecated = fmt.Sprintf("use '%s object … --internal' instead", paths.CLICommandName)
	internalCmd.PersistentPreRunE = runInternalPersistentPreRunE
	internalCmd.PersistentFlags().Bool("allow-degraded", false, "Allow scheduler-dependent commands to run when scheduler daemon is not running")

	// Common operations (work for all internal object kinds)
	internalCmd.AddCommand(NewInternalListCmd())
	internalCmd.AddCommand(NewInternalGetCmd())
	internalCmd.AddCommand(NewInternalCreateCmd())
	internalCmd.AddCommand(NewInternalUpdateCmd())
	internalCmd.AddCommand(NewInternalDeleteCmd())
	internalCmd.AddCommand(NewInternalCountCmd())
	internalCmd.AddCommand(NewInternalBulkCmd())
	internalCmd.AddCommand(NewInternalProcessCmd())
	internalCmd.AddCommand(NewInternalRootFieldsCmd())

	// Kind-based command group (allows "internal <kind> fields")
	internalCmd.AddCommand(NewInternalKindCmd())
	RegisterDynamicInternalKindCommands(internalCmd)

	return internalCmd
}

func runInternalPersistentPreRunE(cmd *cobra.Command, args []string) error {
	if err := requireAdminRole(cmd, args); err != nil {
		return err
	}
	if err := validateAnnotatedInternalKind(cmd, args); err != nil {
		return err
	}
	return runInternalSchedulerGuard(cmd, args)
}

// requireAdminRole checks that the user has admin role
func requireAdminRole(cmd *cobra.Command, _ []string) error {
	logger := logging.GetLoggerFromContext(cmd.Context())

	// Get context to check security
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		logging.FluentEvent(logger).Error("Failed to get CLI context", errfmt.Errorf("context is nil")).Log()
		return errfmt.Errorf("failed to get context")
	}

	// For now, we'll use system security context which has admin role
	// In the future, this should check the actual user's account and roles
	// For CLI, we can allow it but log a warning if not system/admin
	logging.FluentEvent(logger).Debug("Internal command requires admin privileges").
		String("acc"+"ount", internalProfileSystem).
		String("note", "CLI operations use system context").
		Log()

	return nil
}

// getAdminSecurityContext returns a security context with admin privileges
//
