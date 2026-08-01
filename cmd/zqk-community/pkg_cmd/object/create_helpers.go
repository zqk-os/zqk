package object

import (
	"fmt"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// RunCreateWithData runs the object create flow with pre-built objData (e.g. from quick commands).
// Does not load from --file/--data and does not cleanup any source file.
func RunCreateWithData(cmd *cobra.Command, kind string, objData map[string]any) error {
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return cli.Guard(cmd).Err(err).Wrapf("failed to create processor: %w").Return()
	}
	kind, err = objects.ResolveAndValidateKindForProject(proc.ProjectRoot(), kind)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	if objData[objects.FieldKeyKind] == nil || objData[objects.FieldKeyKind] == emptyValue {
		objData[objects.FieldKeyKind] = kind
	}
	normalizeObjectData(objData, kind, proc)
	if err := ensureKindMatches(objData, kind, proc); err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	handled, err := handleDryRun(cmd, objData, kind, proc)
	if err != nil {
		return cli.Guard(cmd).Err(err).Return()
	}
	if handled {
		return nil
	}
	relaxed, _ := cmd.Flags().GetBool("relaxed")
	if relaxed {
		setCacheCheckerForBatchCreation(proc)
	}
	objID, _ := objData[objects.FieldKeyID].(string)
	objKind, _ := objData[objects.FieldKeyKind].(string)
	force, _ := cmd.Flags().GetBool("force")
	opCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")
	if err := proc.Storage().Create(opCtx, proc.SecurityContext(), objData); err != nil {
		if (err == storage.ErrObjectExists || strings.Contains(err.Error(), "already exists")) && force && objID != emptyValue {
			updateCtx := pkgctx.WithCacheUpdate(proc.OperationContext(), objID, objKind, "")
			if updateErr := proc.Storage().Update(updateCtx, proc.SecurityContext(), objID, objData); updateErr != nil {
				return cli.Guard(cmd).Err(updateErr).Wrapf("failed to update with --force: %w").Return()
			}
			logging.FluentEvent(proc.Logger()).Info("Updated existing object with --force").
				ObjectID(objID).
				Log()
		} else {
			return cli.Guard(cmd).Err(err).Wrapf("failed to create object: %w").Return()
		}
	}

	// Update object ID and kind from data (in case they were generated/normalized by storage)
	if id, ok := objData[objects.FieldKeyID].(string); ok && id != emptyValue {
		objID = id
	}
	if k, ok := objData[objects.FieldKeyKind].(string); ok && k != emptyValue {
	}

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	t0 := time.Now()
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), []string{kind}); err != nil {
		logging.FluentEvent(proc.Logger()).Warn("Persist flush after object create timed out, but object is durable").
			WithError(err).
			Kind(kind).
			ObjectID(objID).
			Log()
		if objID != emptyValue {
			// Emit warning but don't fail
			fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString(fmt.Sprintf("Warning: Object '%s' created successfully, but index refresh is delayed.", objID)))
		} else {
			fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Object created successfully, but index refresh is delayed."))
		}
	}
	logSlowCLIObjectMutationFlush(proc.Logger(), "create", objID, []string{kind}, time.Since(t0), 0)
	proc.TriggerCacheFreshnessCheck("create", []string{kind})
	msg := formatCreateSuccessMessage(objData, kind, proc)
	return cli.WriteOutput(cmd, []byte(msg))
}

// loadObjectData loads object data from file, inline data, last-draft pointer, or stdin using internal/cli DataLoader.
func loadObjectData(cmd *cobra.Command, proc *cli.Processor, kind string) (objData map[string]any, filePath string, err error) {
	dl := cli.NewDataLoader(proc.Logger())
	hint := &cli.LastDraftHint{
		Scope: cli.LastDraftScopeObject,
		Kind:  kind,
	}
	return dl.LoadData(cmd, hint)
}

// normalizeObjectData normalizes object values to ensure correct types.
// Uses GetFieldsForKindIfLoaded only so we do not trigger LoadFields() on the create hot path (PRE_CHANGE_CHECKLIST §3).
func normalizeObjectData(objData map[string]any, kind string, proc *cli.Processor) {
	fieldRegistry := objects.GetGlobalFieldRegistry()
	kindFields, ok := fieldRegistry.GetFieldsForKindIfLoaded(kind)
	if !ok {
		return
	}
	if err := normalizeObjectValues(objData, kindFields); err != nil {
		logging.FluentEvent(proc.Logger()).Warn("Failed to normalize object values").
			WithError(err).
			Log()
	}
}

// handleKindSynonym handles special case for kind_synonym objects
func handleKindSynonym(objData map[string]any) {
	// For kind_synonym, the YAML's "kind" field is the target kind (spec field "kind")
	// Storage needs obj[kind] to be kind_synonym (object's own kind)
	// If target_kind is not set, use the "kind" field value as target_kind
	if _, hasTargetKind := objData[objects.FieldKeyTargetKind]; !hasTargetKind {
		if kindValue, ok := objData[objects.FieldKeyKind].(string); ok && kindValue != objects.KindSynonym {
			objData[objects.FieldKeyTargetKind] = kindValue
		}
	}
	objData[objects.FieldKeyKind] = objects.KindSynonym // Set object's own kind for storage
}

// ensureKindMatches ensures the object's kind matches the command's kind
func ensureKindMatches(objData map[string]any, kind string, proc *cli.Processor) error {
	if kind == objects.KindSynonym {
		handleKindSynonym(objData)
		return nil
	}

	return clipkg.EnsureKindMatches(objData, kind, proc.Logger())
}

// handleDryRun handles dry-run mode using internal/cli DryRunHandler and formats through output formatters.
func handleDryRun(cmd *cobra.Command, objData map[string]any, kind string, proc *cli.Processor) (bool, error) {
	dr := cli.NewDryRunHandler(proc.Logger())
	handled, result, err := dr.HandleCreateDryRunResult(cmd, objData, kind, "object")
	if err != nil {
		return handled, err
	}
	if handled && result != nil {
		return true, cli.FormatOutput(cmd, result)
	}
	return handled, nil
}

// cleanupSourceFile removes the source file after successful creation.
// Uses shared utility from pkg/cli
func cleanupSourceFile(cmd *cobra.Command, filePath string, proc *cli.Processor) {
	clipkg.CleanupSourceFile(cmd, filePath, proc.Logger())
}

// formatCreateSuccessMessage formats the success message for object creation
// Uses shared utility from pkg/cli
func formatCreateSuccessMessage(objData map[string]any, kind string, proc *cli.Processor) string {
	return clipkg.FormatCreateSuccessMessage(objData, kind, "Object", proc.Logger())
}
