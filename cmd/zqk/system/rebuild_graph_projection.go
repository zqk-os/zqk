// rebuild_graph_projection.go: rebuild MemGraph projection from file SSOT.
package system

import (
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// NewRebuildGraphProjectionCmd wires command builder from spec.
func NewRebuildGraphProjectionCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewRebuildGraphProjectionCommandBuilder()
	cmd.RunE = runRebuildGraphProjection
	return cmd
}

func runRebuildGraphProjection(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		dryRun, err := cmd.Flags().GetBool("dry-run")
		if err != nil {
			return err
		}

		fp := storage.UnwrapToFileFirstProjection(proc.Storage())
		if fp == nil {
			return errfmt.Errorf("storage is not file+projection mode; set STORAGE_MODE=file+projection and ensure graph is enabled")
		}

		ctx := proc.OperationContext()
		res, err := fp.RebuildProjectionFromSSOTDetailed(ctx, proc.SecurityContext(), dryRun)
		if err != nil {
			return errfmt.Newf("rebuild graph projection").Wrap(err)
		}
		if res == nil {
			res = &storage.RebuildProjectionResult{}
		}

		payload := map[string]any{
			"dry_run":              dryRun,
			"rebuilt_count":        res.RebuiltCount,
			"orphan_deleted":       res.OrphanDeleted,
			"error_count":          len(res.Errors),
			"errors":               res.Errors,
			objects.FieldKeyStatus: "ok",
		}
		if len(res.Errors) > 0 {
			payload[objects.FieldKeyStatus] = "completed_with_errors"
		}
		return cli.FormatOutput(cmd, payload)
	})(cmd, nil)
}
