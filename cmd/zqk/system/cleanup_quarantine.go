package system

import (
	"path/filepath"
	"time"

	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/spf13/cobra"
)

// NewCleanupQuarantineCmd creates a command to remove old files from the quarantine folder.
// Follows cleanup-duplicates pattern: dry-run, verbose, explicit defaults.
func NewCleanupQuarantineCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Remove old files from the quarantine folder",
		"Deletes quarantined files older than --older-than. Use --dry-run to see what would be removed.",
		"",
		"See "+filepath.Join(paths.ProcessDir, "system-health", "QUARANTINE_AND_ANALYSIS.md")+" for what goes in quarantine.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemCleanupQuarantineCommandBuilder(), &cobra.Command{
		Use:   "cleanup-quarantine",
		Short: "Remove old files from the quarantine folder",
		RunE:  runCleanupQuarantine,
	})
	helpBuilder.ApplyToCommand(cmd)
	cmd.Flags().Bool("dry-run", false, "Show what would be removed without deleting")
	cmd.Flags().Bool("verbose", false, "Show each file removed or would be removed")
	cmd.Flags().Duration("older-than", 30*24*time.Hour, "Remove files older than this (default: 720h = 30 days)")
	return cmd
}

func runCleanupQuarantine(cmd *cobra.Command, _ []string) error {
	ctx, err := initializeCleanupQuarantineContext(cmd)
	if err != nil {
		return err
	}
	removed, errs := RunCleanupQuarantine(ctx)
	for _, e := range errs {
		logging.Fluent(ctx.Logger).Warn("Cleanup error").WithError(e).Log()
	}
	if ctx.DryRun {
		logging.Fluent(ctx.Logger).Info("Dry run: would remove quarantined files").
			Count(removed).
			String("older_than", ctx.OlderThan.String()).
			Log()
	} else if removed > 0 {
		logging.Fluent(ctx.Logger).Info("Removed old quarantined files").
			Count(removed).
			String("older_than", ctx.OlderThan.String()).
			Log()
	}
	if len(errs) > 0 {
		return errfmt.Errorf("cleanup-quarantine: %d file(s) removed, %d error(s)", removed, len(errs))
	}
	return nil
}
