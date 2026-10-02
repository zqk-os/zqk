package spec

import (
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
	internal "github.com/zqk-os/zqk/pkg/zqkcli"
)

const emptyValue = ""

// NewSpecListCmd creates the "spec list" subcommand.
func NewSpecListCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSpecListCommandBuilder(), &cobra.Command{
		Use:   "list",
		Short: "List available object specifications",
		Long:  "List object_spec objects from storage (or bundled specs when using file backend).",
	})
	cmd.Aliases = []string{"ls"}
	cli.BindAsyncProgress(cmd, runSpecList)
	cli.AddCommonFlags(cmd)
	return cmd
}

func runSpecList(cmd *cobra.Command, _ []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		var err error
		_ = err

		storageCtx := proc.StorageContext()
		listFilter := storage.ListFilter{
			Kind:    objects.KindObjectSpec,
			Filters: map[string]any{},
			Limit:   0,
		}

		result, err := proc.Storage().List(proc.OperationContext(), proc.SecurityContext(), storageCtx, listFilter)
		if err != nil || len(result.Objects) == 0 {
			// File backend often has no storage for object_spec; fall back to scanning .zqk/specs/objects
			result, err = listSpecsFromFiles(proc.ProjectRoot())
			if err != nil {
				return cli.Guard(cmd).Err(err).Wrapf("failed to list object specs: %w").Return()
			}
		}

		data := map[string]any{
			"objects": result.Objects,
			"meta":    result.Meta,
		}
		if err := cli.FormatOutput(cmd, data); err != nil {
			return cli.Guard(cmd).Err(err).Wrapf("format output: %w").Return()
		}
		return nil
	})(cmd, nil)
}

// listSpecsFromFiles scans .zqk/specs/objects for YAML spec files (file-backend fallback).
func listSpecsFromFiles(projectRoot string) (*storage.QueryResult, error) {
	return internal.ScanObjectSpecsFromFiles(projectRoot)
}
