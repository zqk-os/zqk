package system

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/diskusage"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/resourcehygiene"
)

// NewResourceHygieneCmd creates the resource-hygiene command.
// Spec: .zqk/cli/specs/system/resource_hygiene_command.yaml
func NewResourceHygieneCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resource-hygiene",
		Short: "Inspect and enforce I/O resource hygiene, file retention, stale lock reaping, and temp artifact cleanup",
		Long: `Inspects and enforces system-wide I/O resource hygiene across .zqk directories.
Measures open file descriptors against process limits, inspects storage volume
breakdown, sweeps stale lock files without active holders, reaps abandoned
temporary files (*.tmp, *.tmp-*, .tmp-*), and enforces retention policies on logs.`,
		RunE: runResourceHygiene,
	}

	cmd.Flags().Bool("dry-run", false, "Scan and report hygiene issues without modifying or deleting files")
	cmd.Flags().Bool("reap-locks", true, "Reap stale .lock files whose holding processes are dead")
	cmd.Flags().Bool("reap-temp", true, "Reap orphaned temporary files older than the threshold")
	cmd.Flags().Bool("enforce-retention", true, "Prune or truncate logs older than max-age or larger than max-size")
	cmd.Flags().String("lock-threshold", "15m", "Threshold duration for stale locks (e.g. 15m, 1h)")
	cmd.Flags().String("temp-threshold", "30m", "Threshold duration for orphaned temp files (e.g. 30m, 2h)")
	cmd.Flags().String("project-root", "", "Project root directory")

	cli.AddCommonFlags(cmd)

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

		fmt.Printf("Resource Hygiene Report (dry_run: %t)\n", report.DryRun)
		fmt.Printf("=========================================\n")
		fmt.Printf("Locks reaped:     %d\n", report.LocksReaped)
		fmt.Printf("Temp reaped:      %d\n", report.TempReaped)
		fmt.Printf("Logs pruned:      %d\n", report.LogsPruned)
		fmt.Printf("Bytes reclaimed:  %s (%d bytes)\n", diskusage.FormatBytes(report.BytesReclaimed), report.BytesReclaimed)
		if len(report.ReapedPaths) > 0 {
			fmt.Printf("\nReaped Artifacts:\n")
			for _, p := range report.ReapedPaths {
				fmt.Printf("  • %s\n", p)
			}
		}
		if telemetry != nil {
			fmt.Printf("\nCurrent I/O Telemetry:\n")
			fmt.Printf("  • Open FDs:          %d / %d\n", telemetry.OpenFileDescriptors, telemetry.MaxFileDescriptors)
			fmt.Printf("  • .zqk Storage:      %d files (%s)\n", telemetry.TotalZqkFiles, diskusage.FormatBytes(telemetry.TotalZqkBytes))
			fmt.Printf("  • Stale locks:       %d\n", telemetry.StaleLocksCount)
			fmt.Printf("  • Orphaned temps:    %d\n", telemetry.OrphanedTempCount)
		}

		return nil
	})(cmd, args)
}
