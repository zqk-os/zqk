package object

import (
	"errors"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/validation"
)

// NewDeleteCmd creates a new delete command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewDeleteCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectDeleteCommandBuilder()
	cli.BindAsyncProgress(cmd, runDelete)
	cmd.Aliases = []string{"rm", "remove"}
	cmd.Args = cobra.MinimumNArgs(0)
	return cmd
}

func runDelete(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		rawIDs := expandObjectIDArgs(cmd, args)
		// delete --file is an ID YAML array (unlike update --file = payload).
		// Prefer expandObjectIDArgs for --ids; load --file only when ids flag is unset
		// so LoadIDsFromFlags does not re-append the same --ids list.
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
			var resolvedIDs []string
			for _, arg := range rawIDs {
				id, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", arg)
				if err != nil {
					return cli.Guard(cmd).Err(err).Wrapf("semantic routing failed").Return()
				}
				resolvedIDs = append(resolvedIDs, id)
			}
			return executeBulkDelete(cmd, resolvedIDs, proc)
		}

		idArg := rawIDs[0]

		var err error
		_ = err

		// Phase 17: Semantic CLI Routing
		// Resolve natural language intents (e.g. "My Backlog Item") to exact CAS object IDs
		id, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", idArg)
		if err != nil {
			return cli.Guard(cmd).Err(err).Wrapf("semantic routing failed").Return()
		}

		cascade, err := cmd.Flags().GetBool("cascade")
		if err != nil {
			cascade = false
		}
		unlinkRefs, err := cmd.Flags().GetBool("unlink-references")
		if err != nil {
			unlinkRefs = false
		}
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			dryRun = false
		}
		if unlinkRefs && cascade {
			return cli.Guard(cmd).Err(errors.New("--unlink-references cannot be combined with --cascade")).Return()
		}
		// TEST EDIT: BLI-1786390312940998000-1f101465 — fail-closed delete (no silent GhostRef fan-in).
		if !unlinkRefs && !cascade {
			return cli.Guard(cmd).Err(errors.New("delete refused: pass --unlink-references (strip inbound refs) or --cascade; refusing to leave GhostRefs")).Return()
		}

		logging.FluentEvent(proc.Logger()).Debug("Deleting object").
			ObjectID(id).
			Bool("cascade", cascade).
			Bool("unlink_references", unlinkRefs).
			Bool("dry_run", dryRun).
			Log()

		// Check for dry-run using shared utility
		if dryRun {
			// Read object to show what would be deleted
			obj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
			if err != nil {
				logging.FluentEvent(proc.Logger()).Error("Failed to read object for dry-run", err).
					ObjectID(id).
					Log()
				return cli.Guard(cmd).Err(err).Wrapf("failed to read object: %w").Return()
			}

			handled, result, err := clipkg.HandleDeleteDryRun(cmd, id, obj, cascade, proc.Logger(), "object")
			if err != nil {
				return cli.Guard(cmd).Err(err).Return()
			}
			if handled && result != nil {
				// Format through output formatters to respect --format flag
				return cli.FormatOutput(cmd, result)
			}
		}

		// Mark context as CLI operation for authorization (required for delete).
		// Skip write-behind so delete is durable before return (avoids WAL races with create hammers).
		cliCtx := storage.WithSkipWriteBehind(proc.WithCLIOperation())
		if unlinkRefs {
			cliCtx = storage.WithUnlinkReferencesBeforeDelete(cliCtx)
		}
		var reasonErr error
		cliCtx, reasonErr = withCoreDeleteReasonFromFlags(cmd, cliCtx)
		if reasonErr != nil {
			return reasonErr
		}

		// Get kind from the object before deletion (for cache freshness trigger)
		var affectedKinds []string
		var preDeleteObj map[string]any
		if obj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id); err == nil {
			preDeleteObj = obj
			if kind, ok := obj[objects.FieldKeyKind].(string); ok {
				affectedKinds = []string{kind}
			}
		}
		// Fallback: infer kind from ID so we still flush the correct CAS index/cache
		// even when the pre-delete read can't resolve the object (e.g., stale index).
		if len(affectedKinds) == 0 {
			if inferred := validation.GetIDValidator().InferKindFromID(id); inferred != emptyValue {
				affectedKinds = []string{inferred}
			}
		}
		if unlinkRefs || cascade {
			if idx := objects.TryLoadSpecIndexForProjectRoot(proc.ProjectRoot()); idx != nil {
				affectedKinds = nil
				for k := range idx.Kinds {
					affectedKinds = append(affectedKinds, k)
				}
			}
		}

		// Emit Context HUD if crossing namespaces
		if preDeleteObj != nil {
			clipkg.EmitContextHUD(proc.OperationContext(), proc.SecurityContext(), preDeleteObj, "deleting", id)
		}

		// Delete object
		if err := proc.Storage().Delete(cliCtx, proc.SecurityContext(), id, cascade); err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to delete object", err).
				ObjectID(id).
				Bool("cascade", cascade).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to delete object: %w").Return()
		}

		// Write-behind: Delete returns after WAL+buffer enqueue; flush so the next
		// zqk process (e.g. `object get`) sees the CAS changes immediately.
		flushCtx, cancelFlush := storage.DurabilityFlushContext()
		defer cancelFlush()
		t0 := time.Now()
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), affectedKinds); err != nil {
			logging.FluentEvent(proc.Logger()).Warn("Persist flush after delete timed out, but object is removed").
				WithError(err).
				ObjectID(id).
				Bool("cascade", cascade).
				Log()
			// Emit warning but don't fail, the object is safely deleted.
			if id != emptyValue {
				fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString(fmt.Sprintf("Warning: Object '%s' deleted successfully, but index refresh is delayed.", id)))
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Object deleted successfully, but index refresh is delayed."))
			}
		}
		ensureDur := time.Since(t0)
		t1 := time.Now()
		// Best-effort extra safety: flush listing indexes so no stale mappings survive
		// across write-behind content-addressed update batching.
		if err := storage.FlushAllListingIndexesForProjectRoot(proc.ProjectRoot()); err != nil {
			logging.FluentEvent(proc.Logger()).Error("FlushAllListingIndexesForProjectRoot after delete failed", err).
				ObjectID(id).
				Bool("cascade", cascade).
				Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to flush listing indexes after delete: %w").Return()
		}
		logSlowCLIObjectMutationFlush(proc.Logger(), "delete", id, affectedKinds, ensureDur, time.Since(t1))

		// Trigger cache freshness check (async, non-blocking)
		proc.TriggerCacheFreshnessCheck("delete", affectedKinds)

		// Format success message using shared utility
		msg := clipkg.FormatDeleteSuccessMessage(id, false, cascade, "object", proc.Logger())
		return cli.WriteOutput(cmd, []byte(msg))
	})(cmd, args)
}
