package system

import (
	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/storage"
)

func NewStreamGCCmd() *cobra.Command {
	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemStreamGcCommandBuilder(), &cobra.Command{
		Use:   "stream-gc",
		Short: "Run garbage collector to prune orphaned JSONL stream segment files",
		RunE: func(cmd *cobra.Command, args []string) error {
			results, errKinds := storage.GCOrphanedStreamSegmentsForAllKinds(".")

			totalFiles := 0
			totalBytes := int64(0)

			for _, res := range results {
				if res.FilesRemoved > 0 || res.Errors > 0 {
					cmd.Printf("Kind: %s, Files Removed: %d, Bytes Freed: %d, Errors: %d\n", res.Kind, res.FilesRemoved, res.BytesFreed, res.Errors)
					totalFiles += res.FilesRemoved
					totalBytes += res.BytesFreed
				}
			}

			if errKinds > 0 {
				cmd.Printf("Encountered per-file errors in %d kinds.\n", errKinds)
			}

			cmd.Printf("Successfully pruned %d orphaned stream files (freed %d bytes).\n", totalFiles, totalBytes)
			return nil
		},
	})
	return cmd
}
