package object

import (
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
)

// runUpdateAll handles updating all objects of a kind
func runUpdateAll(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		_ = args // args not used - required by cobra.Command.RunE signature
		var err error
		_ = err

		// Get kind flag
		kind, err := cmd.Flags().GetString("kind")
		if err != nil || kind == emptyValue {
			return cli.Guard(cmd).Require(false, "--kind is required when using --all").Return()
		}

		if nk, ok := kindCanonicalFromPRERun(cmd); ok {
			kind = nk
		} else {
			kind, err = objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), kind)
			if err != nil {
				return cli.Guard(cmd).Err(err).Return()
			}
		}

		// Get flags
		addMissingFields, _ := cmd.Flags().GetBool("add-missing-fields") //nolint:errcheck
		dryRun, _ := cmd.Flags().GetBool("dry-run")                      //nolint:errcheck

		// Build updates map from --field flags
		// For --all, we'll handle append operations per-object in the loop
		// Pass nil here since we don't have a specific current object yet
		updates, err := buildUpdatesFromFieldFlags(cmd, proc, nil)
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		// Build filters from --filter flags
		filters, err := buildUpdateAllFilters(cmd, proc)
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		// List objects of this kind
		storageCtx := proc.StorageContext()
		listFilter := storage.ListFilter{
			Kind:    kind,
			Filters: filters,
		}
		result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, listFilter)
		if err != nil {
			return cli.Guard(cmd).Err(err).Wrapf("failed to list objects: %w").Return()
		}

		if len(result.Objects) == 0 {
			msg := fmt.Sprintf("No objects of kind %s found\n", kind)
			return cli.WriteOutput(cmd, []byte(msg))
		}

		logging.FluentEvent(proc.Logger()).Info("Updating all objects").
			Kind(kind).
			Int("count", len(result.Objects)).
			Log()

		// Determine cache invalidation strategy
		totalObjects := len(result.Objects)
		objectsToUpdate := countObjectsToUpdate(result, kind, updates, addMissingFields, proc)
		useBulkInvalidation := shouldUseBulkInvalidation(objectsToUpdate, totalObjects, dryRun)

		if useBulkInvalidation {
			performBulkCacheInvalidation(kind, proc)
		}

		// Track results
		successCount := 0
		errorCount := 0
		var errors []string

		// Update each object
		for _, obj := range result.Objects {
			objID, _ := obj[objects.FieldKeyID].(string)
			if objID == emptyValue {
				continue
			}

			// Build object-specific updates
			// For append operations, rebuild updates with current object
			objUpdates := buildObjectUpdates(obj, kind, updates, addMissingFields, proc)

			// Handle append operations (field+=value) for this specific object
			fields, _ := cmd.Flags().GetStringArray("field")
			hasAppend := false
			for _, fieldStr := range fields {
				if strings.Contains(fieldStr, "+=") {
					hasAppend = true
					break
				}
			}
			if hasAppend {
				// Rebuild field updates with current object for append operations
				fp := cli.NewFieldParser(proc.Logger())
				appendUpdates, err := fp.ParseFieldFlagsWithAppend(fields, obj)
				if err != nil {
					logging.FluentEvent(proc.Logger()).Error("Failed to parse append operations", err).
						ObjectID(objID).
						Log()
					errors = append(errors, fmt.Sprintf("%s: %v", objID, err))
					errorCount++
					continue
				}
				// Merge append updates into objUpdates
				maps.Copy(objUpdates, appendUpdates)
			}

			if len(objUpdates) == 0 {
				continue
			}

			if dryRun {
				logging.FluentEvent(proc.Logger()).Info("Would update object").
					ObjectID(objID).
					Int("field_count", len(objUpdates)).
					Log()
				successCount++
				continue
			}

			// Update object
			objKind, _ := obj[objects.FieldKeyKind].(string)
			opCtx := proc.OperationContext()
			if !useBulkInvalidation {
				opCtx = pkgctx.WithCacheUpdate(opCtx, objID, objKind, "")
			}
			if err := guardManualStatusUpdate(cmd, proc, objID, objKind, objUpdates); err != nil {
				return err
			}
			if err := guardManualRefFieldUpdates(cmd, proc, objID, objKind, objUpdates); err != nil {
				return err
			}
			if err := guardManualSystemProvenanceFields(cmd, proc, objID, objKind, objUpdates); err != nil {
				return err
			}
			var glassErr error
			opCtx, glassErr = withUpdateBreakGlass(cmd, opCtx, objKind)
			if glassErr != nil {
				return cli.Guard(cmd).Require(false, glassErr.Error()).Return()
			}
			err := proc.Storage().Update(opCtx, proc.SecurityContext(), objID, objUpdates)
			when.When(func() bool { return err != nil }).Then(func() {
				logging.FluentEvent(proc.Logger()).Error("Failed to update object", err).
					ObjectID(objID).
					Log()
				errors = append(errors, fmt.Sprintf("%s: %v", objID, err))
				errorCount++
			}).OrElse(func() {
				successCount++
			}).Run()
		}

		// Write-behind: update --all performs many Updates; flush once after a non-dry run so the next
		// zqk process sees CAS (same contract as object update, bulk create, bulk update, bulk delete).
		if !dryRun && successCount > 0 {
			flushCtx, cancelFlush := storage.DurabilityFlushContext()
			defer cancelFlush()
			t0 := time.Now()
			if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), []string{kind}); err != nil {
				logging.FluentEvent(proc.Logger()).Warn("Persist flush after object update --all timed out, but objects are updated").
					WithError(err).
					Kind(kind).
					Log()
				fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Update all completed, but index refresh is delayed."))
			}
			logSlowCLIObjectMutationFlush(proc.Logger(), "update_all", "(bulk)", []string{kind}, time.Since(t0), 0)
			proc.TriggerCacheFreshnessCheck("update_all", []string{kind})
		}

		// Output results
		output := formatUpdateAllResults(kind, successCount, errorCount, errors, dryRun)
		return cli.WriteOutput(cmd, output)
	})(cmd, args)
}
