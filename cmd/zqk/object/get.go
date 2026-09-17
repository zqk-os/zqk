package object

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/objectget"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// skipObjectGetFieldACL: CLI defaults to system/admin or planner/orchestrator; SpecLoader+SAC on every get
// dominated warm latency ([REDACTED-ID]). Non-admin/non-orchestrator callers still filter.
func skipObjectGetFieldACL(sec *pkgctx.SecurityContext) bool {
	if sec == nil {
		return false
	}
	if sec.AccountID == pkgctx.SystemAccountID {
		return true
	}
	if slices.Contains(sec.Roles, "admin") {
		return true
	}
	return slices.Contains(sec.Permissions, "access:*")
}

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
	// TRACK: BLI-1785909672838827000-9fca84f5 — resolved sidecar (not CAS)
	cmd.Flags().Bool("write-resolved-sidecar", false, "Write hydration overlay to .zqk/resolved/… sidecar (CAS get stays raw when combined with --resolved-sidecar-only)")
	cmd.Flags().Bool("resolved-sidecar-only", false, "After hydrating, write sidecar and emit raw CAS object (no resolved_* in stdout)")
	return cmd
}

func runGet(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		var rawIDs []string
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
			rawIDs = expandObjectIDArgs(cmd, args)
		}
		// get --file is an ID YAML array
		if cmd.Flags().Changed("file") && !cmd.Flags().Changed("ids") {
			idsFromFlags, err := clipkg.LoadIDsFromFlags(cmd, proc.Logger())
			if err == nil {
				rawIDs = append(rawIDs, clipkg.ExpandCommaSeparatedIDs(idsFromFlags...)...)
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

		writeSidecar, _ := cmd.Flags().GetBool("write-resolved-sidecar")
		sidecarOnly, _ := cmd.Flags().GetBool("resolved-sidecar-only")
		if sidecarOnly {
			writeSidecar = true
		}
		// Sidecar write needs an explicit hydrate mode; default lazy when omitted.
		if writeSidecar && hydration == objectget.HydrationUnspecified {
			hydration = objectget.HydrationLazy
		}

		logging.FluentEvent(proc.Logger()).Debug("Reading object").
			ObjectID(id).
			Log()

		obj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
		if err != nil {
			storage.LogObjectReadFailure(proc.Logger(), err, id)
			return cli.Guard(cmd).Err(err).Wrapf("failed to read object: %w").Return()
		}

		// BLI-642: field-level permissions — filter to fields the security context can read
		// (skip SpecLoader+SAC for system/admin CLI path — [REDACTED-ID]).
		if proc.ProjectRoot() != emptyValue && !skipObjectGetFieldACL(proc.SecurityContext()) {
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

		var rawClone map[string]any
		if writeSidecar {
			rawClone = make(map[string]any, len(obj))
			for k, v := range obj {
				rawClone[k] = v
			}
		}

		applyObjectGetOverlays(proc, obj, viewName, hydration)

		if writeSidecar {
			hydLabel := hydration.String()
			if hydLabel == "" {
				hydLabel = "view:" + viewName
			}
			path, werr := objectget.WriteResolvedSidecar(proc.ProjectRoot(), obj, hydLabel)
			if werr != nil {
				return cli.Guard(cmd).Err(werr).Return()
			}
			logging.FluentEvent(proc.Logger()).Info("Wrote resolved sidecar").
				ObjectID(id).
				String("path", path).
				Log()
			if sidecarOnly {
				obj = rawClone
			}
		}

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
