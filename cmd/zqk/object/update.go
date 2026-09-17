package object

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/mcp"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/spf13/cobra"

	"github.com/lanceman/zqk/pkg/objects"
)

// NewUpdateCmd creates a new update command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewUpdateCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectUpdateCommandBuilder()
	cli.BindAsyncProgress(cmd, runUpdate)
	cmd.Aliases = []string{"edit"}
	cmd.Args = cobra.MinimumNArgs(0)
	ensureCmdAnnotations(cmd)
	cmd.Annotations[AnnotationKindValidate] = KindValidateUpdateAllKindFlag
	return cmd
}

//nolint:gocyclo
func runUpdate(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		// Check for --all flag
		updateAll, _ := cmd.Flags().GetBool("all") //nolint:errcheck // Flag parsing errors are non-critical
		if updateAll {
			return runUpdateAll(cmd, args)
		}

		// TRACK: BLI-CEF-CLI-MULTI-ID-UPDATE — positional comma lists must expand like delete.
		rawIDs := expandObjectIDArgs(cmd, args)

		if len(rawIDs) == 0 {
			return cli.Guard(cmd).Require(false, "object ID is required (or use --all --kind <kind> to update all objects of a kind)").Return()
		}

		if len(rawIDs) > 1 {
			// Natively route to BulkUpdate in pkg/storage when multiple targets are specified
			var bulkItems []storage.BulkUpdateItem
			var affectedKinds []string
			kindSet := make(map[string]bool)

			relaxed, _ := cmd.Flags().GetBool("relaxed")
			if relaxed {
				setCacheCheckerForBatchCreation(proc)
			}

			force, _ := cmd.Flags().GetBool("force")
			override, _ := cmd.Flags().GetBool("override")

			for _, arg := range rawIDs {
				id, err := proc.ResolveSemanticArgument(proc.OperationContext(), "", arg)
				if err != nil {
					return cli.Guard(cmd).Err(err).Wrapf("semantic routing failed").Return()
				}

				current, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
				if err != nil && (!force || err != storage.ErrObjectNotFound && !strings.Contains(err.Error(), "not found")) {
					logging.FluentEvent(proc.Logger()).Error("Failed to read object", err).ObjectID(id).Log()
					continue
				}
				if force && err != nil {
					current = nil
				}

				updates, err := buildUpdatesMap(cmd, proc, current)
				if err != nil || len(updates) == 0 {
					continue
				}

				if current != nil && proc.ProjectRoot() != emptyValue && !force && !override {
					loader := objects.GetGlobalSpecLoader()
					sac := mcp.NewSpecAccessControlFromObjectsLoader(loader)
					if sac != nil {
						filtered := make(map[string]any)
						for k, v := range updates {
							if sac.HasFieldAccess(k, current, proc.SecurityContext(), "write") {
								filtered[k] = v
							}
						}
						updates = filtered
					}
				}

				addOptimisticLocking(cmd, updates)
				kind := ""
				if current != nil {
					kind, _ = current[objects.FieldKeyKind].(string)
				} else if k, ok := updates[objects.FieldKeyKind].(string); ok {
					kind = k
				}
				if err := guardManualStatusUpdate(cmd, proc, id, kind, updates); err != nil {
					return err
				}
				if err := guardManualRefFieldUpdates(cmd, proc, id, kind, updates); err != nil {
					return err
				}
				bulkItems = append(bulkItems, storage.BulkUpdateItem{ID: id, Updates: updates})

				if kind != "" && !kindSet[kind] {
					kindSet[kind] = true
					affectedKinds = append(affectedKinds, kind)
				}
			}

			bulkCtx := proc.OperationContext()
			for _, k := range affectedKinds {
				var glassErr error
				bulkCtx, glassErr = withUpdateBreakGlass(cmd, bulkCtx, k)
				if glassErr != nil {
					return cli.Guard(cmd).Require(false, glassErr.Error()).Return()
				}
			}
			result, err := proc.Storage().BulkUpdate(bulkCtx, proc.SecurityContext(), bulkItems)
			if err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("bulk update failed").Return()
			}

			flushCtx, cancelFlush := storage.DurabilityFlushContext()
			defer cancelFlush()
			_ = storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), affectedKinds)
			proc.TriggerCacheFreshnessCheck("bulk_update", affectedKinds)

			format := string(proc.Format())
			outputBulkResult(cmd, result, format, "update")
			return nil
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

		logging.FluentEvent(proc.Logger()).Debug("Updating object").
			ObjectID(id).
			Log()

		// Get force flag early to handle missing objects
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			force = false
		}

		reconstituteUnreadable := false
		// Get current object (needed for append operations and field-level permissions)
		// We always need it, so read once upfront - buildUpdatesMap will only use it if append operations are present
		current, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
		if err != nil {
			// If object doesn't exist and --force is set, we'll create it from updates
			if (err == storage.ErrObjectNotFound || strings.Contains(err.Error(), "not found")) && force {
				// We'll handle this after building the updates map
				current = nil
			} else if storage.LiveCASBlobUnreadable(err) {
				// Committed conflict / hash mismatch: --file rewrite replaces the live blob.
				current = nil
				reconstituteUnreadable = true
			} else {
				logging.FluentEvent(proc.Logger()).Error("Failed to read object", err).
					ObjectID(id).
					Log()
				return cli.Guard(cmd).Err(err).Wrapf("failed to read object: %w").Return()
			}
		}

		// Build updates map (pass current object for append operations)
		updates, err := buildUpdatesMap(cmd, proc, current)
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		// BLI-642: field-level permissions — restrict updates to fields the security context can write
		override, _ := cmd.Flags().GetBool("override")
		requestedFieldCount := len(updates)
		deniedFields := make([]string, 0)
		if current != nil && proc.ProjectRoot() != emptyValue && !force && !override && !skipObjectGetFieldACL(proc.SecurityContext()) {
			// Use global spec loader to reuse cached specs (avoids expensive reload on every update)
			loader := objects.GetGlobalSpecLoader()
			sac := mcp.NewSpecAccessControlFromObjectsLoader(loader)
			if sac != nil {
				filtered := make(map[string]any)
				for k, v := range updates {
					if sac.HasFieldAccess(k, current, proc.SecurityContext(), "write") {
						filtered[k] = v
					} else {
						deniedFields = append(deniedFields, k)
					}
				}
				updates = filtered
			}
		}

		if len(updates) == 0 {
			// TRACK: BLI-REDACTED — ACL strip previously looked like "no updates provided"
			// and caused repeated TestAllKindsCRUD Update false-fails (e.g. role.description).
			if requestedFieldCount > 0 && len(deniedFields) > 0 {
				logging.FluentEvent(proc.Logger()).Warn("All requested field updates denied by field-level permissions").
					ObjectID(id).
					Int("denied_field_count", len(deniedFields)).
					Log()
				return cli.Guard(cmd).Require(false,
					fmt.Sprintf("all requested field updates denied by field-level permissions (denied: %s); use a writable field, elevate context, or --override",
						strings.Join(deniedFields, ", "))).Return()
			}
			logging.FluentEvent(proc.Logger()).Warn("No updates provided for object update").
				ObjectID(id).
				Log()
			return cli.Guard(cmd).Require(false, "no updates provided (use --file, --data, --field, --add-ref/--remove-ref, --unset-field, --auto-status, or pipe from stdin)").Return()
		}

		logging.FluentEvent(proc.Logger()).Debug("Prepared updates").
			ObjectID(id).
			Int("field_count", len(updates)).
			Log()

		// Add optimistic locking if provided
		addOptimisticLocking(cmd, updates)

		// Handle dry-run
		handled, err := handleUpdateDryRun(cmd, id, current, updates, proc)
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}
		if handled {
			return nil
		}

		// Get relaxed flag
		relaxed, err := cmd.Flags().GetBool("relaxed")
		if err != nil {
			relaxed = false
		}

		// Set cache checker if relaxed mode (enables batch update with non-blocking reference validation)
		if relaxed {
			setCacheCheckerForBatchCreation(proc)
		}

		// If object doesn't exist and --force is set, create it from updates
		if current == nil && force {
			// Determine kind from updates or infer from ID
			objKind, _ := updates[objects.FieldKeyKind].(string)
			when.When(func() bool { return objKind == emptyValue }).Then(func() {
				when.When(func() bool { return strings.HasPrefix(id, "BLI-") }).Then(func() { objKind = pplanKindBacklogItem }).
					OrElseWhen(func() bool { return strings.HasPrefix(id, "GOAL-") }).Then(func() { objKind = "goal" }).
					OrElse(func() { objKind = pplanKindBacklogItem }).Run()
			}).Run()

			// Build object from updates (add ID and kind if not present)
			mergedObj := make(map[string]any)
			mergedObj[objects.FieldKeyID] = id
			mergedObj[objects.FieldKeyKind] = objKind
			for k, v := range updates {
				mergedObj[k] = v
			}

			// Normalize values
			normalizedObj, err := NormalizeObjectValues(mergedObj, objKind)
			when.When(func() bool { return err != nil }).Then(func() {
				logging.FluentEvent(proc.Logger()).Warn("Failed to normalize object values").
					WithError(err).
					Log()
			}).OrElse(func() {
				mergedObj = normalizedObj
			}).Run()

			// Emit Context HUD if crossing namespaces
			clipkg.EmitContextHUD(proc.OperationContext(), proc.SecurityContext(), mergedObj, "creating (via force)", id)

			// Create object
			createCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), id, objKind, "")
			if createErr := proc.Storage().Create(createCtx, proc.SecurityContext(), mergedObj); createErr != nil {
				logging.FluentEvent(proc.Logger()).Error("Failed to create object with --force", createErr).
					ObjectID(id).
					Log()
				return cli.Guard(cmd).Err(createErr).Wrapf("failed to create object with --force: %w").Return()
			}
			logging.FluentEvent(proc.Logger()).Info("Created object with --force (object didn't exist)").
				ObjectID(id).
				Log()
		} else {
			if current == nil {
				current = make(map[string]any)
				for k, v := range updates {
					current[k] = v
				}
				current[objects.FieldKeyID] = id
			}
			// Normal update path
			// Get object kind for cache context
			objKind, _ := current[objects.FieldKeyKind].(string)

			// Normalize update values
			normalizedUpdates, err := NormalizeObjectValues(updates, objKind)
			when.When(func() bool { return err != nil }).Then(func() {
				logging.FluentEvent(proc.Logger()).Warn("Failed to normalize update values").
					WithError(err).
					Log()
			}).OrElse(func() {
				updates = normalizedUpdates
			}).Run()

			// Build cache context
			opCtx := buildUpdateCacheContext(proc, id, updates, objKind)
			var glassErr error
			opCtx, glassErr = withUpdateBreakGlass(cmd, opCtx, objKind)
			if glassErr != nil {
				return cli.Guard(cmd).Require(false, glassErr.Error()).Return()
			}

			// Emit Context HUD if crossing namespaces
			clipkg.EmitContextHUD(proc.OperationContext(), proc.SecurityContext(), current, "mutating", id)

			// --file rewrite of an unreadable live blob reconstitutes occupancy; status in
			// the replacement map is the recovered value, not a lifecycle hop.
			if !reconstituteUnreadable {
				if err := guardManualStatusUpdate(cmd, proc, id, objKind, updates); err != nil {
					return err
				}
				if err := guardManualRefFieldUpdates(cmd, proc, id, objKind, updates); err != nil {
					return err
				}
			}
			if err := proc.Storage().Update(opCtx, proc.SecurityContext(), id, updates); err != nil {
				logging.FluentEvent(proc.Logger()).Error("Failed to update object", err).
					ObjectID(id).
					Log()
				return cli.Guard(cmd).Err(err).Wrapf("failed to update object: %w").Return()
			}
		}

		var objKind string
		when.When(func() bool { return current != nil }).Then(func() {
			objKind, _ = current[objects.FieldKeyKind].(string)
		}).OrElse(func() {
			kind, ok := updates[objects.FieldKeyKind].(string)
			when.When(func() bool { return ok }).Then(func() { objKind = kind }).OrElse(func() { objKind = pplanKindBacklogItem }).Run()
		}).Run()

		// Write-behind: ensure next zqk process can read the object immediately after update/create-via-force.
		flushCtx, cancelFlush := storage.DurabilityFlushContext()
		defer cancelFlush()
		t0 := time.Now()
		if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), []string{objKind}); err != nil {
			logging.FluentEvent(proc.Logger()).Warn("Persist flush after object update timed out, but object is durable").
				WithError(err).
				ObjectID(id).
				String(objects.FieldKeyKind, objKind).
				Log()
			// Emit warning but don't fail, the object is safely on disk.
			if id != emptyValue {
				fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString(fmt.Sprintf("Warning: Object '%s' updated successfully, but index refresh is delayed.", id)))
			} else {
				fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Object updated successfully, but index refresh is delayed."))
			}
		}
		logSlowCLIObjectMutationFlush(proc.Logger(), "update", id, []string{objKind}, time.Since(t0), 0)

		// Trigger cache freshness check
		proc.TriggerCacheFreshnessCheck("update", []string{objKind})

		// Get the updated object data for output
		objData, readErr := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
		if readErr != nil {
			// Fallback to merged map if read fails (ranging a nil map is a no-op).
			objData = make(map[string]any)
			for k, v := range current {
				objData[k] = v
			}
			for k, v := range updates {
				objData[k] = v
			}
		}

		format := cli.GetFormat(cmd)
		if format == cli.FormatJSON || format == cli.FormatYAML || format == cli.FormatJSONRPC || format == cli.FormatStream {
			return cli.FormatOutput(cmd, objData)
		}

		// Format success message using shared utility
		msg := clipkg.FormatUpdateSuccessMessage(id, false, "object", proc.Logger())
		return cli.WriteOutput(cmd, []byte(msg))
	})(cmd, args)
}

func enforceActivePlanMembership(ctx context.Context, secCtx *pkgctx.SecurityContext, sp storage.ObjectStorageProvider, id string) error {
	// Child-owned membership: BLI.priority_plan_ref must point at an execution-facing plan
	// (active = shovel-ready, in_progress = scope locked). Parent backlog_item_refs is not required.
	// TRACK: BLI-REDACTED
	item, err := sp.Read(ctx, secCtx, id)
	if err != nil || item == nil {
		return fmt.Errorf("failed to read backlog item %s for plan membership check", id)
	}
	planID, _ := item[objects.FieldKeyPriorityPlanRef].(string)
	if planID == emptyValue {
		return fmt.Errorf("backlog item '%s' has no priority_plan_ref; cannot enter in_progress", id)
	}
	plan, err := sp.Read(ctx, secCtx, planID)
	if err != nil || plan == nil {
		return fmt.Errorf("priority_plan_ref %s not found for backlog item '%s'", planID, id)
	}
	planStatus, _ := plan[objects.FieldKeyStatus].(string)
	if validation.PlanStatusAllowsBacklogInProgress(planStatus) {
		return nil
	}
	return validation.ErrBacklogInProgressRequiresExecutionFacingPlan(id, planID, planStatus)
}

func enforcePriorityOrder(ctx context.Context, secCtx *pkgctx.SecurityContext, sp storage.ObjectStorageProvider, id string) error {
	filter := storage.ListFilter{
		Kind: objects.KindPriorityPlan,
		Filters: map[string]any{
			objects.FieldKeyStatus: objects.ObjectStatusActive,
		},
	}
	res, err := sp.List(ctx, secCtx, pkgctx.NewStorageContext(), filter)
	if err != nil || res == nil || len(res.Objects) == 0 {
		return nil
	}

	plan := res.Objects[0]
	if refs, ok := plan[objects.FieldKeyBacklogItemRefs].([]any); ok {
		var firstActiveItem string
		for _, ref := range refs {
			if strRef, ok := ref.(string); ok {
				item, err := sp.Read(ctx, secCtx, strRef)
				if err == nil && item != nil {
					status, _ := item[objects.FieldKeyStatus].(string)
					if status != "complete" && status != "archived" {
						firstActiveItem = strRef
						break
					}
				}
			}
		}

		if firstActiveItem != "" && firstActiveItem != id {
			return fmt.Errorf("priority order violation: item '%s' has higher priority and must be completed before '%s'. Use --force to override", firstActiveItem, id)
		}
	}
	return nil
}
