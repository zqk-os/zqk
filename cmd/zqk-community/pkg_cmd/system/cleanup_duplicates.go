package system

import (
	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/spf13/cobra"
)

// NewCleanupDuplicatesCmd creates a command to clean up duplicate traditional files
func NewCleanupDuplicatesCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Clean up duplicate traditional files that have hash-based equivalents",
		"Scans hash-based files, extracts their object IDs, and removes traditional files (e.g., ITEM-*.yaml)",
		"that have the same object ID as a hash-based file.",
		"",
		"This is useful for cleaning up after CAS migration where traditional files weren't automatically removed.",
		"",
		"If a kind is provided, only processes that kind. Otherwise processes all kinds.",
	).
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSystemCleanupDuplicatesCommandBuilder(), &cobra.Command{
		Use:  "cleanup-duplicates [kind]",
		Args: cobra.MaximumNArgs(1),
		RunE: runCleanupDuplicates,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().Bool("dry-run", false, "Show what would be deleted without actually deleting")
	cmd.Flags().Bool("verbose", false, "Show detailed output")
	cmd.Flags().Bool("hash-duplicates", false, "Also handle duplicate hash-based files for the same object ID (quarantine by default)")
	cmd.Flags().Bool("delete-hash-duplicates", false, "Delete duplicate hash-based files instead of quarantining them (use with care)")
	cmd.Flags().String("quarantine-dir", "", "Directory to move quarantined duplicate hash files into (default: .zqk/system-health/quarantine/hash-duplicates)")

	cli.EnsureCmdAnnotations(cmd)
	cmd.Annotations[cli.AnnotationKeySystemKindValidate] = cli.KindValidatePositional0Opt

	return cmd
}

func runCleanupDuplicates(cmd *cobra.Command, args []string) error {
	ctx, err := initializeCleanupContext(cmd, args)
	if err != nil {
		return err
	}

	totalDeleted, totalSkipped, errors := cleanupKindsInParallel(ctx)

	if ctx.DryRun {
		logging.Fluent(ctx.Logger).Info("Dry run complete; use without --dry-run to actually delete files").Log()
	} else {
		logging.Fluent(ctx.Logger).Info("Cleanup complete").
			Deleted(totalDeleted).
			Errors(totalSkipped).
			Log()
		if len(errors) > 0 && ctx.Verbose {
			logging.Fluent(ctx.Logger).Warn("Some errors occurred during cleanup").ErrorCount(len(errors)).Log()
		}
	}

	return nil
}
