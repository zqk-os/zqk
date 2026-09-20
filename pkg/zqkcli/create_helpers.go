package internal

import (
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/objects"
)

// readObjectData reads object data from file, data flag, last-draft pointer, or stdin using internal/cli DataLoader.
func readObjectData(cmd *cobra.Command, proc *cli.Processor, kind string) (map[string]any, string, error) {
	dl := cli.NewDataLoader(proc.Logger())
	hint := &cli.LastDraftHint{
		Scope: cli.LastDraftScopeInternal,
		Kind:  kind,
	}
	return dl.LoadData(cmd, hint)
}

// validateAndPrepareObject validates and prepares object for creation
func validateAndPrepareObject(objData map[string]any, kind string, proc *cli.Processor) error {
	// Ensure kind matches using shared utility
	if err := clipkg.EnsureKindMatches(objData, kind, proc.Logger()); err != nil {
		return err
	}

	// Ensure source_type is set to internal for internal objects
	if objData[objects.FieldKeySourceType] == nil {
		objData[objects.FieldKeySourceType] = internalSourceInternal
	}

	return nil
}

// handleDryRunCreate handles dry-run mode for create using internal/cli DryRunHandler.
func handleDryRunCreate(cmd *cobra.Command, proc *cli.Processor, objData map[string]any, kind string) error {
	dr := cli.NewDryRunHandler(proc.Logger())
	handled, result, err := dr.HandleCreateDryRunResult(cmd, objData, kind, "internal object")
	if err != nil {
		return err
	}
	if handled && result != nil {
		return cli.FormatOutput(cmd, result)
	}
	return nil
}

// createInternalObject creates the internal object
func createInternalObject(proc *cli.Processor, objData map[string]any, kind string) (string, error) {
	// Create object
	if err := proc.Storage().Create(proc.OperationContext(), proc.SecurityContext(), objData); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to create object", err).
			Kind(kind).
			Log()
		return "", errfmt.Newf("failed to create object").Wrap(err)
	}

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), []string{kind}); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Persist flush after internal create failed", err).
			Kind(kind).
			Log()
		return "", errfmt.Newf("internal create reported success but data is not yet readable (write-behind flush)").Wrap(err)
	}
	proc.TriggerCacheFreshnessCheck("internal_create", []string{kind})

	id, _ := objData[objects.FieldKeyID].(string)
	logging.FluentEvent(proc.Logger()).Info("Internal object created successfully").
		ObjectID(id).
		Kind(kind).
		Log()

	return id, nil
}
