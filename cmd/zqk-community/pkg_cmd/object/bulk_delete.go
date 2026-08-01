package object

import (
	"bytes"
	"errors"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/lanceman/zqk/cmd/zqk-community/pkg_cmd/system"
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	storagepkg "github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/spf13/cobra"
)

// NewBulkDeleteCmd creates a bulk delete command
// Generated from command spec - DO NOT EDIT MANUALLY (use spec file as source of truth)
func NewBulkDeleteCmd() *cobra.Command {
	// Use generated builder from spec
	cmd := bldr_cli_cmd_v1.NewObjectBulkDeleteCommandBuilder()
	// Add RunE implementation
	cli.BindAsyncProgress(cmd, runBulkDelete)

	return cmd
}

func runBulkDelete(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		ids, err := clipkg.LoadIDsFromFlags(cmd, proc.Logger())
		if err != nil {
			return cli.Guard(cmd).Err(err).Return()
		}
		return executeBulkDelete(cmd, ids, proc)
	})(cmd, args)
}

func executeBulkDelete(cmd *cobra.Command, ids []string, proc *cli.Processor) error {
	var err error
	_ = err

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

	if dryRun {
		logging.FluentEvent(proc.Logger()).Info("Dry-run mode: showing what would be deleted").
			Int("count", len(ids)).
			Log()
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "Would delete %d objects:\n", len(ids))
		for _, id := range ids {
			obj, err := proc.Storage().Read(proc.OperationContext(), proc.SecurityContext(), id)
			when.When(func() bool { return err == nil }).Then(func() {
				fmt.Fprintf(&buf, "  - %s", id)
				if kind, ok := obj[objects.FieldKeyKind].(string); ok {
					fmt.Fprintf(&buf, " (kind: %s)", kind)
				}
				if title, ok := obj[objects.FieldKeyTitle].(string); ok {
					fmt.Fprintf(&buf, " - %s", title)
				}
				buf.WriteString("\n")
			}).OrElse(func() {
				fmt.Fprintf(&buf, "  - %s (not found)\n", id)
			}).Run()
		}
		if unlinkRefs {
			buf.WriteString("  Unlink references from dependents before delete: true\n")
		}
		if cascade {
			buf.WriteString("  Cascade: true (would also delete dependents)\n")
		}
		return cli.WriteOutput(cmd, buf.Bytes())
	}

	// Mark context as CLI operation for authorization (required for delete)
	cliCtx := proc.WithCLIOperation()
	if unlinkRefs {
		cliCtx = storagepkg.WithUnlinkReferencesBeforeDelete(cliCtx)
	}

	// Set progress so heartbeat shows "Deleting N objects..." every 5s (no routine progress otherwise)
	if fn := pkgctx.GetValidationProgress(cmd.Context()); fn != nil {
		fn("bulk_delete", fmt.Sprintf("Deleting %d objects...", len(ids)))
	}

	result, err := proc.Storage().BulkDelete(cliCtx, proc.SecurityContext(), ids, cascade)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Bulk delete failed", err).Log()
		return cli.Guard(cmd).Err(err).Wrapf("bulk delete failed: %w").Return()
	}

	// Invalidate object ID cache for successfully deleted IDs so object-count-report stays accurate (OBJECT_COUNT_SELF_MAINTENANCE)
	successIDs := make([]string, 0, len(result.Results))
	for _, m := range result.Results {
		if id, ok := m[objects.FieldKeyID].(string); ok {
			successIDs = append(successIDs, id)
		}
	}
	if len(successIDs) > 0 {
		system.BulkInvalidateObjectIDCache(successIDs, proc.ProjectRoot())
	}

	// Trigger cache freshness check (async, non-blocking)
	affectedKinds := make(map[string]bool)
	for _, m := range result.Results {
		// Kind may be present for create/get; for delete we only have id
		if kind, ok := m[objects.FieldKeyKind].(string); ok {
			affectedKinds[kind] = true
		} else if id, ok := m[objects.FieldKeyID].(string); ok {
			// Fallback: infer kind from ID so we still flush the correct CAS index/cache
			if inferred := validation.GetIDValidator().InferKindFromID(id); inferred != "" {
				affectedKinds[inferred] = true
			}
		}
	}
	kindsList := make([]string, 0, len(affectedKinds))
	for kind := range affectedKinds {
		kindsList = append(kindsList, kind)
	}

	// Write-behind: BulkDelete returns after WAL+buffer enqueue; flush so the next
	// zqk process sees the CAS changes immediately.
	flushCtx, cancelFlush := storagepkg.DurabilityFlushContext()
	defer cancelFlush()
	t0 := time.Now()
	if err := storagepkg.EnsureCLIObjectMutationVisibleForProvider(flushCtx, proc.Storage(), proc.ProjectRoot(), kindsList); err != nil {
		logging.FluentEvent(proc.Logger()).Warn("Persist flush after bulk delete timed out, but objects are removed").
			WithError(err).
			Bool("cascade", cascade).
			Log()
		// Emit warning but don't fail, the objects are safely deleted.
		fmt.Fprintln(cmd.ErrOrStderr(), color.YellowString("Warning: Bulk delete completed, but index refresh is delayed."))
	}
	ensureDur := time.Since(t0)
	t1 := time.Now()
	// Best-effort extra safety: flush listing indexes so no stale mappings survive
	// across write-behind content-addressed update batching.
	if err := storagepkg.FlushAllListingIndexesForProjectRoot(proc.ProjectRoot()); err != nil {
		logging.FluentEvent(proc.Logger()).Error("FlushAllListingIndexesForProjectRoot after bulk delete failed", err).
			Bool("cascade", cascade).
			Log()
		return cli.Guard(cmd).Err(err).Wrapf("failed to flush listing indexes after bulk delete: %w").Return()
	}
	logSlowCLIObjectMutationFlush(proc.Logger(), "bulk_delete", "(bulk)", kindsList, ensureDur, time.Since(t1))

	proc.TriggerCacheFreshnessCheck("bulk_delete", kindsList)

	projectFields, perr := clipkg.FieldsFromCmd(cmd)
	if perr != nil {
		return cli.Guard(cmd).Err(perr).Return()
	}
	applyHybridProjectionToBulkResult(result, projectFields)

	// Output results (format respects context precedence: system -> user -> project -> command)
	format := string(proc.Format())
	outputBulkResult(cmd, result, format, "delete")

	return nil
}
