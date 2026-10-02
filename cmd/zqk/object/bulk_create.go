package object

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	objkeys "github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/when"
)

// NewBulkCreateCmd creates a bulk create command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewBulkCreateCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectBulkCreateCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runBulkCreate)

	// Mark file flag as required (not handled by codegen yet)
	//nolint:errcheck // Flag requirement check - error would be caught at runtime
	_ = cmd.MarkFlagRequired("file")

	configureKindPositionalValidation(cmd, false)

	return cmd
}

func runBulkCreate(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		kind, err := resolvePositional0Kind(cmd, proc, args, "")
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}

		filePath, data, err := readRequiredFileFlag(cmd)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to read file", err).
				File(filePath).
				Log()
			return err
		}

		// Parse YAML array
		var objects []map[string]any
		if err := yaml.Unmarshal(data, &objects); err != nil {
			logging.FluentEvent(proc.Logger()).Error("Failed to parse YAML", err).Log()
			return cli.Guard(cmd).Err(err).Wrapf("failed to parse YAML: %w").Return()
		}

		// Ensure all objects have the correct kind and normalize values
		for i, obj := range objects {
			if obj[objkeys.FieldKeyKind] == nil {
				obj[objkeys.FieldKeyKind] = kind
			} else if obj[objkeys.FieldKeyKind] != kind {
				return cli.Guard(cmd).Err(errfmt.Errorf("object at index %d has kind %v, expected %s", i, obj[objkeys.FieldKeyKind], kind)).Return()
			}
			// Normalize object values to ensure correct types (especially for number fields)
			// This handles cases where YAML parsing converts whole-number floats to ints
			normalizedObj, err := NormalizeObjectValues(obj, kind)
			when.When(func() bool { return err != nil }).Then(func() {
				logging.FluentEvent(proc.Logger()).Warn("Failed to normalize object values").
					WithError(err).
					Int("index", i).
					Log()
			}).OrElse(func() {
				objects[i] = normalizedObj
			}).Run()
		}

		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			dryRun = false
		}
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			force = false
		}

		// Auto-detect relaxed mode: bulk operations are always batch operations
		// Set cache checker automatically to enable non-blocking reference validation
		// The --relaxed flag is still available for explicit control but not required
		setCacheCheckerForBatchCreation(proc)

		if dryRun {
			logging.FluentEvent(proc.Logger()).Info("Dry-run mode: showing what would be created").
				Int("count", len(objects)).
				Log()
			var buf bytes.Buffer
			fmt.Fprintf(&buf, "Would create %d objects:\n", len(objects))
			for i, obj := range objects {
				fmt.Fprintf(&buf, "\nObject %d:\n", i+1)
				output, err := yaml.Marshal(obj)
				if err != nil {
					fmt.Fprintf(&buf, "Error marshaling object: %v\n", err)
					continue
				}
				buf.Write(output)
			}
			return cli.WriteOutput(cmd, buf.Bytes())
		}

		// If --force is set, check for existing objects and update them instead of creating
		if force {
			// Separate objects into new and existing
			newObjects := make([]map[string]any, 0)
			existingObjects := make([]map[string]any, 0)

			for _, obj := range objects {
				objID, _ := obj[objkeys.FieldKeyID].(string)
				when.When(func() bool { return objID != emptyValue }).Then(func() {
					_, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), objID)
					when.When(func() bool { return err == nil }).Then(func() {
						existingObjects = append(existingObjects, obj)
					}).OrElse(func() {
						newObjects = append(newObjects, obj)
					}).Run()
				}).OrElse(func() {
					newObjects = append(newObjects, obj)
				}).Run()
			}

			// Update existing objects
			for _, obj := range existingObjects {
				objID, _ := obj[objkeys.FieldKeyID].(string)
				objKind, _ := obj[objkeys.FieldKeyKind].(string)
				updateCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")
				if updateErr := proc.Storage().Update(updateCtx, proc.SecurityContext(), objID, obj); updateErr != nil {
					logging.FluentEvent(proc.Logger()).Warn("Failed to update existing object with --force").
						WithError(updateErr).
						String("object_id", objID).
						Log()
				}
			}

			// Only create new objects
			objects = newObjects
		}

		result, err := proc.Storage().BulkCreate(proc.OperationContext(), proc.SecurityContext(), objects)
		if err != nil {
			logging.FluentEvent(proc.Logger()).Error("Bulk create failed", err).Log()
			return cli.Guard(cmd).Err(err).Wrapf("bulk create failed: %w").Return()
		}

		// If --force was set, we already handled existing objects above
		// This section is for handling any remaining errors (shouldn't happen with force)
		if force && len(result.Errors) > 0 {
			logging.FluentEvent(proc.Logger()).Info("Retrying failed objects with --force (update instead of create)").
				Int("failed_count", len(result.Errors)).
				Log()

			for _, bulkErr := range result.Errors {
				// Check if error is due to object already existing
				var errMsg string
				when.When(func() bool { return bulkErr.Error != nil }).Then(func() {
					errMsg = bulkErr.Error.Error()
				}).OrElse(func() {
					errMsg = bulkErr.Message
				}).Run()

				if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "duplicate") {
					// Get the object that failed
					if bulkErr.Index >= 0 && bulkErr.Index < len(objects) {
						obj := objects[bulkErr.Index]
						objID, _ := obj[objkeys.FieldKeyID].(string)
						if objID != emptyValue {
							// Try to update instead
							objKind, _ := obj[objkeys.FieldKeyKind].(string)
							updateCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")
							if updateErr := proc.Storage().Update(updateCtx, proc.SecurityContext(), objID, obj); updateErr == nil {
								// Update succeeded - adjust counts
								result.SuccessCount++
								result.FailureCount--
								// Remove from errors (we'll rebuild the errors list)
								logging.FluentEvent(proc.Logger()).Info("Updated existing object with --force").
									String("object_id", objID).
									Int("index", bulkErr.Index).
									Log()
							}
						}
					}
				}
			}

			// Rebuild errors list (remove successfully updated objects)
			newErrors := make([]storagepkg.BulkOperationError, 0)
			for _, bulkErr := range result.Errors {
				var errMsg string
				when.When(func() bool { return bulkErr.Error != nil }).Then(func() {
					errMsg = bulkErr.Error.Error()
				}).OrElse(func() {
					errMsg = bulkErr.Message
				}).Run()

				// Keep error if it wasn't an "already exists" error, or if update failed
				if !strings.Contains(errMsg, "already exists") && !strings.Contains(errMsg, "duplicate") {
					newErrors = append(newErrors, bulkErr)
				} else {
					// Check if we successfully updated this object
					if bulkErr.Index >= 0 && bulkErr.Index < len(objects) {
						obj := objects[bulkErr.Index]
						objID, _ := obj[objkeys.FieldKeyID].(string)
						if objID != emptyValue {
							// Verify update succeeded by checking if object exists
							_, readErr := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), objID)
							if readErr == nil {
								// Object exists - update succeeded, don't include in errors
								continue
							}
						}
					}
					// Update failed or couldn't verify - keep error
					newErrors = append(newErrors, bulkErr)
				}
			}
			result.Errors = newErrors
		}

		// Write-behind: BulkCreate returns after WAL+buffer enqueue; flush so the next
		// zqk process (e.g. `object get`) sees the CAS changes immediately.
		flushCtx, cancelFlush := storagepkg.DurabilityFlushContext()
		defer cancelFlush()
		t0 := time.Now()
		if err := storagepkg.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), []string{kind}); err != nil {
			logging.FluentEvent(proc.Logger()).Warn("Persist flush after bulk create timed out, but objects are durable").
				WithError(err).
				Kind(kind).
				Log()
			// Emit warning but don't fail, the objects are safely on disk.
			fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Bulk create completed, but index refresh is delayed."))
		}
		logSlowCLIObjectMutationFlush(proc.Logger(), "bulk_create", "(bulk)", []string{kind}, time.Since(t0), 0)

		// Trigger cache freshness check (async, non-blocking)
		// Collect affected kinds from created objects
		affectedKinds := make(map[string]bool)
		for _, obj := range objects {
			if kind, ok := obj[objkeys.FieldKeyKind].(string); ok {
				affectedKinds[kind] = true
			}
		}
		kindsList := make([]string, 0, len(affectedKinds))
		for kind := range affectedKinds {
			kindsList = append(kindsList, kind)
		}
		proc.TriggerCacheFreshnessCheck("bulk_create", kindsList)

		if err := projectAndOutputBulkResult(cmd, proc, result, "create"); err != nil {
			return err
		}

		if len(result.Errors) > 0 {
			return cli.Guard(cmd).Err(errfmt.Errorf("bulk create failed with %d errors", len(result.Errors))).Return()
		}

		return nil
	})(cmd, args)
}
