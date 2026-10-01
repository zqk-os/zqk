package system

import (
	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/cliapp"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/logging"
)

// cleanupResultKindEchoLimit is the point past which echoing the scanned kinds obscures the counts.
const cleanupResultKindEchoLimit = 12

// NewCleanupDuplicatesCmd creates a command to clean up duplicate traditional files
func NewCleanupDuplicatesCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Clean up duplicate traditional files that have hash-based equivalents",
		"Scans hash-based files, extracts their object IDs, and removes traditional files (e.g., BLI-*.yaml)",
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

// CleanupDuplicatesResult is the structured outcome of a cleanup-duplicates run.
//
// This command is what `system check` tells an operator to run against a Tier-1 duplicate-blob
// finding, and it previously reported only through logger Info lines, which the default context
// discards. So the remediation for a blocking finding printed nothing at all -- not even "Dry run
// complete" -- leaving no way to distinguish a successful quarantine from a silent no-op against the
// wrong directory. Counts go through FormatOutput per POL-CODE-015 so the operator sees an answer.
type CleanupDuplicatesResult struct {
	DryRun         bool     `json:"dry_run"`
	KindsScanned   int      `json:"kinds_scanned"`
	Kinds          []string `json:"kinds,omitempty"`
	HashDuplicates bool     `json:"hash_duplicates"`
	QuarantineDir  string   `json:"quarantine_dir,omitempty"`
	Removed        int      `json:"removed"`
	Skipped        int      `json:"skipped"`
	Errors         []string `json:"errors,omitempty"`
}

func runCleanupDuplicates(cmd *cobra.Command, args []string) error {
	ctx, err := initializeCleanupContext(cmd, args)
	if err != nil {
		return err
	}

	totalDeleted, totalSkipped, errs := cleanupKindsInParallel(ctx)

	if ctx.DryRun {
		logging.Fluent(ctx.Logger).Info("Dry run complete; use without --dry-run to actually delete files").Log()
	} else {
		logging.Fluent(ctx.Logger).Info("Cleanup complete").
			Deleted(totalDeleted).
			Errors(totalSkipped).
			Log()
		if len(errs) > 0 && ctx.Verbose {
			logging.Fluent(ctx.Logger).Warn("Some errors occurred during cleanup").ErrorCount(len(errs)).Log()
		}
	}

	res := CleanupDuplicatesResult{
		DryRun:         ctx.DryRun,
		KindsScanned:   len(ctx.Kinds),
		HashDuplicates: ctx.HashDuplicates,
		Removed:        totalDeleted,
		Skipped:        totalSkipped,
	}
	// Only echo the kind list when the caller scoped the run; the all-kinds form would print the
	// entire ontology and bury the counts that matter.
	if len(ctx.Kinds) <= cleanupResultKindEchoLimit {
		res.Kinds = ctx.Kinds
	}
	if ctx.HashDuplicates && !ctx.DeleteHashDuplicates {
		res.QuarantineDir = ctx.QuarantineDir
	}
	for _, e := range errs {
		res.Errors = append(res.Errors, e.Error())
	}

	return cli.FormatOutput(cmd, res)
}
