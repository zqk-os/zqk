package organizational

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	orgdomain "github.com/zqk-os/zqk/pkg/domain/organizational"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewPropagateCmd creates the propagate command from the generated builder.
func NewPropagateCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewOrganizationalPropagateCommandBuilder()
	cli.BindAsyncProgress(cmd, runPropagate)
	return cmd
}

func runPropagate(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		changeID, _ := cmd.Flags().GetString("change")
		confirm, _ := cmd.Flags().GetBool("confirm")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if changeID == emptyValue {
			return errfmt.Errorf("--change is required")
		}

		ctx, secCtx, store := proc.StorageTuple()
		domainLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

		changeObj, err := store.Read(ctx, secCtx, changeID)
		if err != nil {
			logging.Fluent(domainLogger).Error("Failed to read organizational change", err).
				String("change_id", changeID).
				Log()
			return errfmt.Errorf("failed to read organizational change %s: %w", changeID, err)
		}

		kind, _ := changeObj[objects.FieldKeyKind].(string)
		if kind != objects.KindOrganizationalChange {
			return errfmt.Errorf("object %s is not an %s (got %s)", changeID, objects.KindOrganizationalChange, kind)
		}

		impactAnalysisID := getOrCreateImpactAnalysis(ctx, store, secCtx, changeObj, changeID, domainLogger)
		if impactAnalysisID == emptyValue {
			return errfmt.Errorf("no impact analysis for change %s; run 'organizational analyze-impact --change %s' first", changeID, changeID)
		}

		impactObj, err := store.Read(ctx, secCtx, impactAnalysisID)
		if err != nil {
			logging.Fluent(domainLogger).Error("Failed to read impact analysis", err).
				String("impact_id", impactAnalysisID).
				Log()
			return errfmt.Errorf("failed to read impact analysis %s: %w", impactAnalysisID, err)
		}

		affectedObjects, _ := impactObj[objects.FieldKeyAffectedObjects].(map[string]any)
		if len(affectedObjects) == 0 {
			msg := fmt.Sprintf("No affected objects for change %s.\n", changeID)
			return cli.WriteOutput(cmd, []byte(msg))
		}

		var total int
		var updated int
		summary := "Propagating organizational changes...\n"

		for objType, list := range affectedObjects {
			ids, _ := list.([]any)
			if len(ids) == 0 {
				continue
			}
			count := len(ids)
			total += count
			summary += fmt.Sprintf("  - %s: %d objects\n", objType, count)

			if confirm && !dryRun {
				for _, idAny := range ids {
					objID, ok := idAny.(string)
					if !ok || objID == emptyValue {
						continue
					}
					updates := map[string]any{"last_propagated_change_ref": changeID}
					if err := store.Update(ctx, secCtx, objID, updates); err != nil {
						logging.Fluent(domainLogger).Warn("Failed to update object for propagation").
							ObjectID(objID).
							WithError(err).
							Log()
						continue
					}
					updated++
				}
			}
		}

		if confirm && !dryRun {
			summary += fmt.Sprintf("✓ %d objects updated with last_propagated_change_ref\n", updated)
			summary += "✓ Change propagation complete\n"
		} else if dryRun {
			summary += fmt.Sprintf("Dry run: %d objects would be updated (use --confirm to apply)\n", total)
		} else {
			summary += fmt.Sprintf("Use --confirm to update %d affected objects, or --dry-run to preview.\n", total)
		}

		return cli.WriteOutput(cmd, []byte(summary))
	})(cmd, args)
}

// getOrCreateImpactAnalysis returns the latest impact_analysis ID for the change, or runs AnalyzeChange if none.
func getOrCreateImpactAnalysis(ctx context.Context, store storage.ObjectStorageProvider, secCtx *pkgctx.SecurityContext, changeObj map[string]any, changeID string, logger logging.Logger) string {
	refs, _ := changeObj[objects.FieldKeyImpactAnalysisRefs].([]any)
	if len(refs) > 0 {
		if last, ok := refs[len(refs)-1].(string); ok && last != emptyValue {
			return last
		}
	}
	domainLogger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	analyzer := orgdomain.NewImpactAnalyzer(storage.NewOrganizationalStorageAdapter(store), domainLogger, secCtx)
	impactID, err := analyzer.AnalyzeChange(ctx, changeID)
	if err != nil {
		logging.Fluent(logger).Warn("Failed to create impact analysis for propagate").
			WithError(err).
			String("change_id", changeID).
			Log()
		return ""
	}
	return impactID
}
