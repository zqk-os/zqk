package internal

import (
	"fmt"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewInternalDeleteCmd creates a delete command for internal/built-in objects
func NewInternalDeleteCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Delete an internal or built-in object (admin only)",
		"Delete an internal or built-in object with admin privileges.",
		"",
		"WARNING: Deleting built-in objects may break system functionality.",
		"Use with extreme caution.",
		"",
		"By default, deletion will fail if the object has dependents (objects that reference it).",
		"Use --unlink-references to remove this ID from dependents' reference fields, then delete.",
		"Use --cascade to delete the object and all its dependents recursively.",
		"Do not combine --unlink-references with --cascade.",
	).
		AddExample("Delete a built-in object (dangerous!)", "%s internal delete COMP-TYPE-001").
		AddExample("Delete with cascade (deletes object and all dependents)", "%s internal delete COMP-TYPE-001 --cascade").
		AddExample("Dry-run to see what would be deleted", "%s internal delete COMP-TYPE-001 --cascade --dry-run").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "delete <id> [--unlink-references] [--cascade]",
		Args: cobra.ExactArgs(1),
	}

	cli.BindAsyncProgress(cmd, runInternalDelete)
	cmd.Flags().Bool("cascade", false, "Delete object and all objects that reference it")
	cmd.Flags().Bool("unlink-references", false, "Strip this ID from dependents' reference fields, then delete (does not delete dependent objects)")
	cmd.Flags().Bool("dry-run", false, "Show what would be deleted without actually deleting it")

	return cli.FinalizeCommand(cmd, helpBuilder)
}

func runInternalDelete(cmd *cobra.Command, args []string) error {
	id, proc, err := initInternalProcessorWithID(cmd, args)
	if err != nil {
		return err
	}

	// Read object to check if it's built-in
	obj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to read object", err).
			ObjectID(id).
			Log()
		return errfmt.Newf("failed to read object").Wrap(err)
	}

	isBuiltIn := storage.IsBuiltIn(obj)
	if isBuiltIn {
		logging.FluentEvent(proc.Logger()).Warn("Deleting built-in object").
			ObjectID(id).
			Log()
		// Output warning through logging framework
		warning := fmt.Sprintf("WARNING: Deleting built-in object %s may break system functionality!\n", id)
		if err := cli.WriteOutput(cmd, []byte(warning)); err != nil {
			return err
		}
	}

	cascade, err := cmd.Flags().GetBool("cascade")
	if err != nil {
		cascade = false
	}
	unlinkRefs, err := cmd.Flags().GetBool("unlink-references")
	if err != nil {
		unlinkRefs = false
	}
	if unlinkRefs && cascade {
		return errfmt.Errorf("--unlink-references cannot be combined with --cascade")
	}
	// TRACK: fail-closed delete (parity with cmd/zqk/object).
	if !unlinkRefs && !cascade {
		return errfmt.Errorf("delete refused: pass --unlink-references or --cascade; refusing to leave GhostRefs")
	}

	// Check for dry-run using shared utility
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		dryRun = false
	}
	if dryRun {
		handled, result, err := clipkg.HandleDeleteDryRun(cmd, id, obj, cascade, proc.Logger(), "internal object")
		if err != nil {
			return err
		}
		if handled && result != nil {
			// Format through output formatters to respect --format flag
			return cli.FormatOutput(cmd, result)
		}
	}

	// Use processor's WithCLIOperation for authorization
	cliCtx := proc.WithCLIOperation()
	if unlinkRefs {
		cliCtx = storage.WithUnlinkReferencesBeforeDelete(cliCtx)
	}

	if err := proc.Storage().Delete(cliCtx, proc.SecurityContext(), id, cascade); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to delete object", err).
			ObjectID(id).
			Log()
		return errfmt.Newf("failed to delete object").Wrap(err)
	}

	affectedKinds := make([]string, 0, 1)
	if k, ok := obj[objects.FieldKeyKind].(string); ok && k != emptyValue {
		affectedKinds = append(affectedKinds, k)
	}
	if len(affectedKinds) == 0 {
		affectedKinds = uniqueKindsFromObjectIDs([]string{id})
	}

	flushCtx, cancelFlush := storage.DurabilityFlushContext()
	defer cancelFlush()
	if err := storage.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), affectedKinds); err != nil {
		logging.FluentEvent(proc.Logger()).Error("Persist flush after internal delete failed", err).
			ObjectID(id).
			Log()
		return errfmt.Newf("internal delete reported success but data is not yet readable (write-behind flush)").Wrap(err)
	}
	if err := storage.FlushAllListingIndexesForProjectRoot(proc.ProjectRoot()); err != nil {
		logging.FluentEvent(proc.Logger()).Error("FlushAllListingIndexesForProjectRoot after internal delete failed", err).
			ObjectID(id).
			Log()
		return errfmt.Newf("failed to flush listing indexes after internal delete").Wrap(err)
	}
	proc.TriggerCacheFreshnessCheck("internal_delete", affectedKinds)

	// Format success message using shared utility
	msg := clipkg.FormatDeleteSuccessMessage(id, isBuiltIn, cascade, "internal object", proc.Logger())
	return cli.WriteOutput(cmd, []byte(msg))
}
