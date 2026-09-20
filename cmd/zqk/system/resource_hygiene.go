package system

import (
	"context"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/diskusage"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
)

// NewResourceHygieneCmd creates the resource-hygiene command.
func NewResourceHygieneCmd() *cobra.Command {
	cmd := bldr_cli_cmd_v1.NewSystemResourceHygieneCommandBuilder()
	cmd.RunE = runResourceHygiene
	return cmd
}

func runResourceHygiene(cmd *cobra.Command, args []string) error {
	return cli.WithProcessor(func(cmd *cobra.Command, _ []string, proc *cli.Processor) error {
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		reapLocks, _ := cmd.Flags().GetBool("reap-locks")
		reapTemp, _ := cmd.Flags().GetBool("reap-temp")
		enforceRetention, _ := cmd.Flags().GetBool("enforce-retention")
		lockThresholdStr, _ := cmd.Flags().GetString("lock-threshold")
		tempThresholdStr, _ := cmd.Flags().GetString("temp-threshold")

		lockThreshold := 15 * time.Minute
		if lockThresholdStr != "" {
			if d, err := time.ParseDuration(lockThresholdStr); err == nil {
				lockThreshold = d
			}
		}
		tempThreshold := 30 * time.Minute
		if tempThresholdStr != "" {
			if d, err := time.ParseDuration(tempThresholdStr); err == nil {
				tempThreshold = d
			}
		}

		root, _ := cmd.Flags().GetString("project-root")
		if root == "" {
			root = proc.ProjectRoot()
		}
		if root == "" {
			return errfmt.Errorf("project root required")
		}

		opts := resourcehygiene.HygieneOptions{
			ReapLocks:        reapLocks,
			ReapTemp:         reapTemp,
			EnforceRetention: enforceRetention,
			DryRun:           dryRun,
			LockThreshold:    lockThreshold,
			TempThreshold:    tempThreshold,
			LogMaxAge:        14 * 24 * time.Hour,
			LogMaxSize:       10 * 1024 * 1024,
		}

		report, err := resourcehygiene.ExecuteHygiene(root, opts)
		if err != nil {
			return errfmt.Newf("execute resource hygiene").Wrap(err)
		}

		telemetry, _ := resourcehygiene.InspectIOResources(context.Background(), root)

		isJSON, _ := cmd.Flags().GetString("format")
		if isJSON == "json" {
			return cli.FormatOutput(cmd, map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusSuccess,
				"report":               report,
				"telemetry":            telemetry,
			})
		}

		cmd.Printf("Resource Hygiene Report (dry_run: %t)\n", report.DryRun)
		cmd.Printf("=========================================\n")
		cmd.Printf("Locks reaped:     %d\n", report.LocksReaped)
		cmd.Printf("Temp reaped:      %d\n", report.TempReaped)
		cmd.Printf("Logs pruned:      %d\n", report.LogsPruned)
		cmd.Printf("Bytes reclaimed:  %s (%d bytes)\n", diskusage.FormatBytes(report.BytesReclaimed), report.BytesReclaimed)
		if len(report.ReapedPaths) > 0 {
			cmd.Printf("\nReaped Artifacts:\n")
			for _, p := range report.ReapedPaths {
				cmd.Printf("  • %s\n", p)
			}
		}
		if telemetry != nil {
			cmd.Printf("\nCurrent I/O Telemetry:\n")
			cmd.Printf("  • Open FDs:          %d / %d\n", telemetry.OpenFileDescriptors, telemetry.MaxFileDescriptors)
			cmd.Printf("  • .zqk Storage:      %d files (%s)\n", telemetry.TotalZqkFiles, diskusage.FormatBytes(telemetry.TotalZqkBytes))
			cmd.Printf("  • Stale locks:       %d\n", telemetry.StaleLocksCount)
			cmd.Printf("  • Orphaned temps:    %d\n", telemetry.OrphanedTempCount)
		}

		return nil
	})(cmd, args)
}
