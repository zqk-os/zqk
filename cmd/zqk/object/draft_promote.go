package object

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// Constants for resilient draft plane bulk promote batching.
const (
	defaultDraftPromoteBatchSize = 10
	maxBoundedDraftErrors        = 50
)

type draftItemError struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

// NewDraftPromoteCmd creates `object draft promote`.
func NewDraftPromoteCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewObjectDraftPromoteCommandBuilder()
	cli.BindAsyncProgress(cmd, runObjectDraftPromote)
	return cmd
}

func runObjectDraftPromote(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		flags, err := parseDraftPlaneCLIFlags(cmd)
		if err != nil {
			return err
		}
		dryRun := flags.dryRun
		if !flags.dryRun {
			if gateErr := storage.RequireDraftPlaneApplyGate(flags.matchOpts, "promote"); gateErr != nil {
				return gateErr
			}
		}
		matched, skipped, inv, matchErr := storage.MatchObjectDraftPlane(proc.ProjectRoot(), flags.matchOpts)
		if matchErr != nil {
			return matchErr
		}

		out := map[string]any{
			"dry_run":               flags.dryRun,
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

		kindByID := make(map[string]string, len(matched))
		pathByID := make(map[string]string, len(matched))
		for _, m := range matched {
			kindByID[m.ID] = m.Kind
			pathByID[m.ID] = m.Path
		}

		resolveKind := func(id string) string {
			if k, ok := kindByID[id]; ok && k != "" {
				return k
			}
			return validation.GetIDValidator().InferKindFromID(id)
		}

		pruneDraftFile := func(id string) {
			kind := resolveKind(id)
			if kind != "" {
				_ = storage.DeleteObjectDraftFile(proc.ProjectRoot(), kind, id)
			}
			if p, ok := pathByID[id]; ok && p != "" {
				if _, err := fileutil.Stat(p); err == nil {
					_ = fileutil.Remove(p)
					shardDir := filepath.Dir(p)
					_ = fileutil.Remove(shardDir)
					_ = fileutil.Remove(filepath.Dir(shardDir))
				}
			}
		}

		isPromoted := func(id string) bool {
			kind := resolveKind(id)
			if kind != "" && storage.CasBackedObjectID(proc.ProjectRoot(), kind, id) {
				return true
			}
			return false
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
					pruneDraftFile(bid)
				}
				continue
			}

			// If batch failed, inspect each item in the batch:
			// items that successfully materialized into CAS during the batch promote
			// must have their draft files pruned and marked as promoted immediately without re-promoting.
			// Only items that did not promote should be retried individually.
			for _, singleID := range batch {
				if isPromoted(singleID) {
					promotedIDs = append(promotedIDs, singleID)
					if idx, ok := candidateIndex[singleID]; ok {
						candidates[idx]["action"] = "promoted"
					}
					pruneDraftFile(singleID)
					continue
				}

				singleErr := promoteObjectIDs(cmd, proc, []string{singleID})
				if singleErr == nil || isPromoted(singleID) {
					promotedIDs = append(promotedIDs, singleID)
					if idx, ok := candidateIndex[singleID]; ok {
						candidates[idx]["action"] = "promoted"
					}
					pruneDraftFile(singleID)
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
