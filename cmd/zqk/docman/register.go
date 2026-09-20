package docman

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/docman"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NewRegisterCmd creates a new register command
func NewRegisterCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Discover and register documentation files as doc_entry objects",
		"Discover all markdown files in the docs directory and register them as doc_entry objects.",
		"",
		"This command:",
		"  - Scans the docs/ directory for markdown files",
		"  - Extracts metadata (title, summary, group, category) from each file",
		"  - Creates doc_entry objects for files that don't already have entries",
		"  - Skips files that already have registered doc_entry objects",
	).
		AddExample("Register all documentation files", "%s docman register").
		AddExample("Dry run to see what would be registered", "%s docman register --dry-run").
		ExcludeCommonFlags()

	registerCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewDocmanRegisterCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use:  "register",
		RunE: runRegister,
	})

	helpBuilder.ApplyToCommand(registerCmd)

	// Add flags
	registerCmd.Flags().Bool("dry-run", false, "Show what would be registered without actually creating objects")
	registerCmd.Flags().Bool("update-existing", false, "Update existing doc_entries to include missing reference fields per spec")
	registerCmd.Flags().StringSlice("subtrees", nil, "Specific subtrees to scan and register (defaults to all docs/)")
	registerCmd.Flags().Bool("shipped-only", false, "Only scan and register shipped documentation subtrees (architecture, best-practices, onboarding)")

	// Add common flags
	cli.AddCommonFlags(registerCmd)

	return registerCmd
}

func runRegister(cmd *cobra.Command, args []string) error {
	// Get context from command
	cmdCtx := cmd.Context()
	if cmdCtx == nil {
		cmdCtx = pkgctx.NewSystemContext()
	}

	// Get profile from command flags or default
	profile := string(pkgctx.ProfileHuman)
	if f := cmd.Flags().Lookup("context"); f != nil {
		if val, err := cmd.Flags().GetString("context"); err == nil && val != emptyValue {
			profile = val
		}
	}

	logger := logging.GetLoggerFromProfile(profile)

	// Get project root
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found")
	}

	// Get flags
	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return errfmt.Newf("failed to get dry-run flag").Wrap(err)
	}
	updateExisting, err := cmd.Flags().GetBool("update-existing")
	if err != nil {
		return errfmt.Newf("failed to get update-existing flag").Wrap(err)
	}
	subtrees, err := cmd.Flags().GetStringSlice("subtrees")
	if err != nil {
		return errfmt.Newf("failed to get subtrees flag").Wrap(err)
	}
	shippedOnly, err := cmd.Flags().GetBool("shipped-only")
	if err != nil {
		return errfmt.Newf("failed to get shipped-only flag").Wrap(err)
	}
	if shippedOnly && len(subtrees) == 0 {
		subtrees = docman.ShippedInitDocSubtrees
	}

	// Initialize storage provider via StorageFactory for unified storage routing
	factory, err := storage.NewStorageFactory(cmdCtx, projectRoot)
	if err != nil {
		return errfmt.Newf("failed to initialize storage factory").Wrap(err)
	}
	storageProvider := factory.GetStorageForKind("doc_entry")

	// Create registry
	registry := docman.NewRegistry(storageProvider, projectRoot)

	// Update existing entries if requested
	if updateExisting {
		logging.Fluent(logger).Info("Updating existing doc_entries to include missing reference fields").Log()
		updated, err := registry.UpdateExistingEntries(cmdCtx, profile)
		if err != nil {
			return errfmt.Newf("failed to update existing entries").Wrap(err)
		}
		msg := fmt.Sprintf("Updated %d doc_entry objects with missing reference fields\n", updated)
		//nolint:errcheck // Output errors are non-critical
		_ = cli.WriteOutput(cmd, []byte(msg))
		if !dryRun {
			return nil // If only updating, return here
		}
	}

	// Register documentation
	logging.Fluent(logger).Info("Starting documentation registration").
		String("dry_run", fmt.Sprintf("%v", dryRun)).
		Log()

	created, skipped, err := registry.RegisterSubtrees(cmdCtx, profile, subtrees, dryRun)
	if err != nil {
		if !dryRun && created > 0 {
			msg := fmt.Sprintf("Created %d doc_entry objects before interruption\nSkipped %d files (already registered)\n", created, skipped)
			//nolint:errcheck // Output errors are non-critical
			_ = cli.WriteOutput(cmd, []byte(msg))
		}
		return errfmt.Newf("failed to register documentation (created %d, skipped %d)", created, skipped).Wrap(err)
	}

	// Output results
	if dryRun {
		msg := fmt.Sprintf("Would create %d doc_entry objects\nWould skip %d files (already registered)\n", created, skipped)
		//nolint:errcheck // Output errors are non-critical
		_ = cli.WriteOutput(cmd, []byte(msg))
	} else {
		msg := fmt.Sprintf("Created %d doc_entry objects\nSkipped %d files (already registered)\n", created, skipped)
		//nolint:errcheck // Output errors are non-critical
		_ = cli.WriteOutput(cmd, []byte(msg))
	}

	return nil
}
