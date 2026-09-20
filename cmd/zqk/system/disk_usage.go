package system

import (
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/diskusage"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	diskUsageOutputFieldStatus = "status"
	diskUsageOutputStatusOK    = "ok"
)

// NewDiskUsageCmd reports directory sizes via pure-Go walk (cross-platform du-style).
// Spec: .zqk/cli/specs/system/disk_usage_command.yaml
// TRACK: docs/architecture/FILESYSTEM_DATA_LAYOUT.md — housekeeping insight before sprawl cleanup.
func NewDiskUsageCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemDiskUsageCommandBuilder()
	cmd.RunE = runDiskUsage
	cmd.Aliases = append(cmd.Aliases, "du")
	return cmd
}

func runDiskUsage(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, args []string, proc *cli.Processor) error {
		depth, err := cmd.Flags().GetInt("depth")
		if err != nil {
			return err
		}
		minStr, err := cmd.Flags().GetString("min")
		if err != nil {
			return err
		}
		top, err := cmd.Flags().GetInt("top")
		if err != nil {
			return err
		}
		includeFiles, err := cmd.Flags().GetBool("include-files")
		if err != nil {
			return err
		}

		minBytes, err := diskusage.ParseSize(minStr)
		if err != nil {
			return errfmt.Newf("invalid --min").Wrap(err)
		}
		if depth < 0 {
			return errfmt.Errorf("--depth must be >= 0")
		}
		if top < 0 {
			return errfmt.Errorf("--top must be >= 0")
		}

		root := proc.ProjectRoot()
		if len(args) == 1 && args[0] != "" {
			p := args[0]
			if !filepath.IsAbs(p) {
				p = filepath.Join(root, p)
			}
			root = p
		}

		res, err := diskusage.Scan(root, diskusage.Options{
			MaxDepth:     depth,
			MinBytes:     minBytes,
			Limit:        top,
			IncludeFiles: includeFiles,
		})
		if err != nil {
			return errfmt.Newf("disk-usage scan").Wrap(err)
		}

		payload := map[string]any{
			"root":                     res.Root,
			"max_depth":                res.MaxDepth,
			"min_bytes":                res.MinBytes,
			"min_human":                res.MinHuman,
			objects.FieldKeyEntries:    res.Entries,
			"skipped_errors":           res.SkippedErrors,
			"dirs_visited":             res.DirsVisited,
			"files_visited":            res.FilesVisited,
			"total_bytes_root":         res.TotalBytesRoot,
			"total_human_root":         diskusage.FormatBytes(res.TotalBytesRoot),
			diskUsageOutputFieldStatus: diskUsageOutputStatusOK,
		}
		return cli.FormatOutput(cmd, payload)
	})(cmd, args)
}
