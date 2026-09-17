package object

import (
	"fmt"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewDraftPromoteCmd creates `object draft promote`.
// TRACK: BLI-REDACTED
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

		// Apply: reuse object promote one-hop rules (stuck diags printed to stdout).
		promoteErr := promoteObjectIDs(cmd, proc, ids)
		if promoteErr != nil {
			out["promote_error"] = promoteErr.Error()
			_ = cli.FormatOutput(cmd, out)
			return promoteErr
		}
		out["promoted"] = len(ids)
		for i := range candidates {
			candidates[i]["action"] = "promote_attempted"
		}
		out["candidates"] = candidates
		return cli.FormatOutput(cmd, out)
	})(cmd, args)
}
