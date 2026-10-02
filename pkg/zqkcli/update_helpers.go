package internal

import (
	"maps"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// readCurrentObject reads the current object to check if it's built-in
func readCurrentObject(proc *cli.Processor, id string) (obj map[string]any, found bool, err error) {
	current, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
	if err != nil {
		logErr := errfmt.Newf("failed to read object").Wrap(err)
		logging.FluentEvent(proc.Logger()).Error("Failed to read object", logErr).
			ObjectID(id).
			Log()
		return nil, false, logErr
	}

	isBuiltIn := storage.IsBuiltIn(current)
	if isBuiltIn {
		logging.FluentEvent(proc.Logger()).Info("Updating built-in object").
			ObjectID(id).
			Log()
	}

	return current, isBuiltIn, nil
}

// parseFieldUpdates parses field updates from --field flags using internal/cli FieldParser.
func parseFieldUpdates(cmd *cobra.Command, proc *cli.Processor) (map[string]any, error) {
	var flagsBag clipkg.FlagBag
	fields := flagsBag.StringArray(cmd, "field")
	if flagsBag.Err() != nil || len(fields) == 0 {
		return nil, flagsBag.Err()
	}
	fp := cli.NewFieldParser(proc.Logger())
	return fp.ParseFieldFlags(fields)
}

// readFileUpdates reads updates from a file using internal/cli DataLoader.
func readFileUpdates(cmd *cobra.Command, proc *cli.Processor) (map[string]any, error) {
	var flagsBag clipkg.FlagBag
	filePath := flagsBag.String(cmd, "file")
	if flagsBag.Err() != nil || filePath == emptyValue {
		return nil, flagsBag.Err()
	}
	return cli.NewDataLoader(proc.Logger()).LoadUpdatesFromFile(filePath)
}

// readDataUpdates reads updates from --data flag using internal/cli DataLoader.
func readDataUpdates(cmd *cobra.Command, proc *cli.Processor) (map[string]any, error) {
	var flagsBag clipkg.FlagBag
	dataStr := flagsBag.String(cmd, "data")
	if flagsBag.Err() != nil || dataStr == emptyValue {
		return nil, flagsBag.Err()
	}
	return cli.NewDataLoader(proc.Logger()).LoadUpdatesFromData(dataStr)
}

func applyInternalAutoStatusFlag(cmd *cobra.Command, currentObj map[string]any, updates map[string]any) error {
	return cli.ApplyAutoStatusFlag(cmd, currentObj, updates)
}

// buildAllUpdates builds all updates from field flags, file, data, or stdin using internal/cli.
func buildAllUpdates(cmd *cobra.Command, proc *cli.Processor, currentObj map[string]any) (map[string]any, error) {
	updates := make(map[string]any)

	fieldUpdates, err := parseFieldUpdates(cmd, proc)
	if err != nil {
		return nil, err
	}
	maps.Copy(updates, fieldUpdates)

	fileUpdates, err := readFileUpdates(cmd, proc)
	if err != nil {
		return nil, err
	}
	if fileUpdates != nil {
		maps.Copy(updates, fileUpdates)
		if err := applyInternalAutoStatusFlag(cmd, currentObj, updates); err != nil {
			return nil, err
		}
		return updates, nil
	}

	dataUpdates, err := readDataUpdates(cmd, proc)
	if err != nil {
		return nil, err
	}
	if dataUpdates != nil {
		maps.Copy(updates, dataUpdates)
		if err := applyInternalAutoStatusFlag(cmd, currentObj, updates); err != nil {
			return nil, err
		}
		return updates, nil
	}

	var flagsBag clipkg.FlagBag
	fields := flagsBag.StringArray(cmd, "field")
	autoStatus := flagsBag.Bool(cmd, "auto-status")
	if flagsBag.Err() == nil && len(fields) == 0 && !autoStatus {
		dl := cli.NewDataLoader(proc.Logger())
		stdinUpdates, _, err := dl.LoadFromStdin()
		if err != nil {
			return nil, err
		}
		maps.Copy(updates, stdinUpdates)
	}

	if err := applyInternalAutoStatusFlag(cmd, currentObj, updates); err != nil {
		return nil, err
	}

	if len(updates) == 0 {
		return nil, errfmt.Errorf("no updates provided (use --file, --data, --field, --auto-status, or pipe from stdin)")
	}
	return updates, nil
}

// executeUpdate executes the update operation
func executeUpdate(proc *cli.Processor, id string, updates map[string]any) error {
	// For built-in objects, we need to allow updating normally immutable fields
	// The storage layer handles this for internal commands
	if err := proc.Storage().Update(proc.OperationContext(), proc.SecurityContext(), id, updates); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to update object", err).
			ObjectID(id).
			Log()
		return errfmt.Newf("failed to update object").Wrap(err)
	}

	kinds := uniqueKindsFromObjectIDs([]string{id})
	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), kinds); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Persist flush after internal update failed", err).
			ObjectID(id).
			Log()
		return errfmt.Newf("internal update reported success but data is not yet readable (write-behind flush)").Wrap(err)
	}
	proc.TriggerCacheFreshnessCheck("internal_update", kinds)

	logging.FluentEvent(proc.Logger()).Info("Object updated successfully").
		ObjectID(id).
		Log()
	return nil
}

// outputUpdateSuccess outputs success message using shared utility
func outputUpdateSuccess(cmd *cobra.Command, id string, isBuiltIn bool, proc *cli.Processor) error {
	msg := clipkg.FormatUpdateSuccessMessage(id, isBuiltIn, "internal object", proc.Logger())
	return cli.WriteOutput(cmd, []byte(msg))
}
