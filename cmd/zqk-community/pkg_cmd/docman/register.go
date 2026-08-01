package docman

import (
	"fmt"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/docman"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/spf13/cobra"
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
	profile, err := cmd.Flags().GetString("context")
	if err != nil {
		return errfmt.Newf("failed to get context flag").Wrap(err)
	}
	if profile == emptyValue {
		profile = string(pkgctx.ProfileHuman) // Default profile
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

	// Initialize storage provider
	storageProvider, err := storage.NewFileObjectStorage(projectRoot)
	if err != nil {
		return errfmt.Newf("failed to initialize storage provider").Wrap(err)
	}

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

	// Register all documentation
	logging.Fluent(logger).Info("Starting documentation registration").
		String("dry_run", fmt.Sprintf("%v", dryRun)).
		Log()

	created, skipped, err := registry.RegisterAll(cmdCtx, profile, dryRun)
	if err != nil {
		return errfmt.Newf("failed to register documentation").Wrap(err)
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
