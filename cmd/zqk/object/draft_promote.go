package object

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// Constants for resilient draft plane bulk promote batching.
// TRACK: PRI-DRAFT-PROMOTE-TIMEOUT-001 / BLI-1789618716318867000-dc1647b6
const (
	defaultDraftPromoteBatchSize = 10
	maxBoundedDraftErrors        = 50
)

type draftItemError struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// NewDraftPromoteCmd creates `object draft promote`.
// TRACK: BLI-1785827958281378000-c0366ecd
func NewDraftPromoteCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectDraftPromoteCommandBuilder()
	cli.BindAsyncProgress(cmd, runObjectDraftPromote)
	return cmd
}

func runObjectDraftPromote(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}
		all, err := cmd.Flags().GetBool("all")
		if err != nil {
			return err
		}
		kind, err := cmd.Flags().GetString("kind")
		if err != nil {
			return err
		}
		idPrefix, err := cmd.Flags().GetString("id-prefix")
		if err != nil {
			return err
		}
		status, err := cmd.Flags().GetString("status")
		if err != nil {
			return err
		}
		olderThanStr, err := cmd.Flags().GetString("older-than")
		if err != nil {
			return err
		}
		maxN, err := cmd.Flags().GetInt("max")
		if err != nil {
			return err
		}
		var olderThan time.Duration
		if olderThanStr != "" {
			olderThan, err = time.ParseDuration(olderThanStr)
			if err != nil {
				return fmt.Errorf("invalid --older-than %q: %w", olderThanStr, err)
			}
		}
		matchOpts := storage.ObjectDraftPlaneMatchOptions{
			Kind: kind, IDPrefix: idPrefix, Status: status,
			OlderThan: olderThan, All: all, Max: maxN,
		}
		if !dryRun {
			if gateErr := storage.RequireDraftPlaneApplyGate(matchOpts, "promote"); gateErr != nil {
				return gateErr
			}
		}
		matched, skipped, inv, matchErr := storage.MatchObjectDraftPlane(proc.ProjectRoot(), matchOpts)
		if matchErr != nil {
			return matchErr
		}

		out := map[string]any{
			"dry_run":               dryRun,
			"matched":               len(matched),
			objects.FieldKeySkipped: len(skipped),
			"draft_root":            storage.ObjectDraftPlaneRoot(proc.ProjectRoot()),
			"inventory_before":      inv,
			"skipped_items":         skipped,
		}

		ids := make([]string, 0, len(matched))
		candidates := make([]map[string]any, 0, len(matched))
		for _, m := range matched {
			ids = append(ids, m.ID)
			row := map[string]any{
				objects.FieldKeyID: m.ID, objects.FieldKeyKind: m.Kind, objects.FieldKeyStatus: m.Status, objects.FieldKeyPath: m.Path,
			}
			if dryRun {
				row["action"] = "would_promote"
			} else {
				row["action"] = "promote_queued"
			}
			candidates = append(candidates, row)
		}
		out["candidates"] = candidates

		if dryRun || len(ids) == 0 {
			return cli.FormatOutput(cmd, out)
		}

		batchSize := defaultDraftPromoteBatchSize
		if len(ids) <= batchSize {
			batchSize = len(ids)
		}

		var promotedIDs []string
		var failedItems []draftItemError
		candidateIndex := make(map[string]int, len(candidates))
		for i, c := range candidates {
			if cid, ok := c[objects.FieldKeyID].(string); ok {
				candidateIndex[cid] = i
			}
		}

		for i := 0; i < len(ids); i += batchSize {
			end := i + batchSize
			if end > len(ids) {
				end = len(ids)
			}
			batch := ids[i:end]

			// Attempt batch promote
			batchErr := promoteObjectIDs(cmd, proc, batch)
			if batchErr == nil {
				for _, bid := range batch {
					promotedIDs = append(promotedIDs, bid)
					if idx, ok := candidateIndex[bid]; ok {
						candidates[idx]["action"] = "promoted"
					}
				}
				continue
			}

			// If batch failed, fall back to one-item-at-a-time
			// so individual stuck items or transient daemon delays do not block healthy drafts.
			// TRACK: BLI-1789618716318867000-dc1647b6 (one-id promote remains documented fallback)
			for _, singleID := range batch {
				singleErr := promoteObjectIDs(cmd, proc, []string{singleID})
				if singleErr == nil {
					promotedIDs = append(promotedIDs, singleID)
					if idx, ok := candidateIndex[singleID]; ok {
						candidates[idx]["action"] = "promoted"
					}
				} else {
					if idx, ok := candidateIndex[singleID]; ok {
						candidates[idx]["action"] = "failed"
						candidates[idx]["error"] = singleErr.Error()
					}
					if len(failedItems) < maxBoundedDraftErrors {
						failedItems = append(failedItems, draftItemError{
							ID:    singleID,
							Error: singleErr.Error(),
						})
					}
				}
			}
		}

		out["promoted"] = len(promotedIDs)
		out["failed"] = len(failedItems)
		out["candidates"] = candidates
		if len(failedItems) > 0 {
			out["errors"] = failedItems
			var errList []string
			for _, f := range failedItems {
				errList = append(errList, fmt.Sprintf("%s: %s", f.ID, f.Error))
			}
			summaryErr := fmt.Errorf("draft promote completed with %d failure(s) out of %d item(s):\n%s", len(failedItems), len(ids), strings.Join(errList, "\n"))
			out["promote_error"] = summaryErr.Error()
			_ = cli.FormatOutput(cmd, out)
			return summaryErr
		}

		return cli.FormatOutput(cmd, out)
	})(cmd, args)
}
