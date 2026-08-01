package object

import (
	"path/filepath"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objectget"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// NewGetCmd creates a new get command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewGetCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectGetCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runGet)
	cmd.Aliases = []string{"show", "view"}
	cmd.Args = cobra.MinimumNArgs(0)
	_ = cmd.RegisterFlagCompletionFunc("fields", completeGetProjectFields)
	return cmd
}

func runGet(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var rawIDs []string
		if len(args) > 0 {
			// Backwards compatibility for single get with kind and ID
			if len(args) == 2 && !cmd.Flags().Changed("file") && !cmd.Flags().Changed("ids") {
				var id string
				if strings.Contains(args[1], "-") {
					id = args[1]
				} else if strings.Contains(args[0], "-") {
					id = args[0]
				} else {
					id = args[1]
				}
				rawIDs = append(rawIDs, id)
			} else {
				rawIDs = append(rawIDs, args...)
			}
		}

		if cmd.Flags().Changed("file") || cmd.Flags().Changed("ids") {
			idsFromFlags, err := clipkg.LoadIDsFromFlags(cmd, proc.Logger())
			if err == nil {
				rawIDs = append(rawIDs, idsFromFlags...)
			}
		}

		if len(rawIDs) == 0 {
			return cli.Guard(cmd).Require(false, "at least one object ID must be provided").Return()
		}

		if len(rawIDs) > 1 {
			return executeBulkGet(cmd, rawIDs, proc)
		}

		id := rawIDs[0]
		viewName, _ := cmd.Flags().GetString("view")
		if viewName == emptyValue {
			viewName = objectget.ViewDefault
		}

		linkHydrationRaw, _ := cmd.Flags().GetString("link-hydration")
		hydration, parseErr := objectget.ParseLinkHydration(linkHydrationRaw)
		if parseErr != nil {
			return cli.Guard(cmd).Err(parseErr).Return()
		}

		var err error
		_ = err

		logging.FluentEvent(proc.Logger()).Debug("Reading object").
			ObjectID(id).
			Log()

		// Get object
		obj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to read object", err).
				ObjectID(id).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to read object: %w").Return()
		}

		// ITEM-642: field-level permissions — filter to fields the security context can read
		if proc.ProjectRoot() != emptyValue {
			specsDir := filepath.Join(proc.ProjectRoot(), paths.ProcessInternalObjectSpecsDir)
			loader := objects.NewSpecLoader(specsDir)
			sac := mcp.NewSpecAccessControlFromObjectsLoader(loader)
			if sac != nil {
				obj = sac.FilterObjectFields(obj, proc.SecurityContext(), "read")
			}
		}

		logging.FluentEvent(proc.Logger()).Debug("Object read successfully").
			ObjectID(id).
			Log()

		applyObjectGetOverlays(proc, obj, viewName, hydration)

		// Apply view-based field projection to keep the payload focused.
		obj = applyObjectViewProjection(viewName, obj)

		projectFields, perr := clipkg.FieldsFromCmd(cmd)
		if perr != nil {
			return cli.Guard(cmd).Err(perr).Return()
		}
		if len(projectFields) > 0 {
			kind, _ := obj[objects.FieldKeyKind].(string)
			mask := objects.HybridMaskForList(kind, projectFields, "")
			obj = objects.ProjectMapHybrid(obj, mask)
		}

		// Output using format handlers for consistent formatting
		return cli.FormatOutput(cmd, obj)
	})(cmd, args)
}
