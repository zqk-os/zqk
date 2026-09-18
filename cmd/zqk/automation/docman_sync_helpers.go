package automation

import (
	"context"
	"fmt"
	"os"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/docman"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// DocmanSyncFlags contains parsed docman-sync command flags
type DocmanSyncFlags struct {
	Profile        string
	CheckOnly      bool
	GitAware       bool
	UpdateExisting bool
	DryRun         bool
}

// parseDocmanSyncFlags parses all docman-sync command flags
func parseDocmanSyncFlags(cmd *cobra.Command) (*DocmanSyncFlags, error) {
	flags := &DocmanSyncFlags{}

	// Get profile from command flags or default
	profile, err := cmd.Flags().GetString("context")
	if err != nil {
		return nil, errfmt.Newf("failed to get context flag").Wrap(err)
	}
	if profile == emptyValue {
		profile = automationProfileAIAgent // Default profile for automation
	}
	flags.Profile = profile

	checkOnly, err := cmd.Flags().GetBool("check-only")
	if err != nil {
		return nil, errfmt.Newf("failed to get check-only flag").Wrap(err)
	}
	flags.CheckOnly = checkOnly

	gitAware, err := cmd.Flags().GetBool("git-aware")
	if err != nil {
		return nil, errfmt.Newf("failed to get git-aware flag").Wrap(err)
	}
	flags.GitAware = gitAware

	updateExisting, err := cmd.Flags().GetBool("update-existing")
	if err != nil {
		return nil, errfmt.Newf("failed to get update-existing flag").Wrap(err)
	}
	flags.UpdateExisting = updateExisting

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return nil, errfmt.Newf("failed to get dry-run flag").Wrap(err)
	}
	flags.DryRun = dryRun

	return flags, nil
}

// getCommandContext gets the command context or creates a background context
func getCommandContext(cmd *cobra.Command) context.Context {
	cmdCtx := cmd.Context()
	if cmdCtx == nil {
		cmdCtx = pkgctx.NewSystemContext()
	}
	return cmdCtx
}

// checkGitAwareSync checks if git-aware sync should proceed
func checkGitAwareSync(cmd *cobra.Command, projectRoot string, gitAware bool, logger logging.Logger) bool {
	if !gitAware {
		return true
	}

	hasMarkdown, err := hasStagedMarkdownFiles(projectRoot)
	if err != nil {
		// If git is not available or not a git repo, log warning but continue
		logging.Fluent(logger).Debug("Failed to check staged markdown files (not a git repo or git not available)").
			WithError(err).
			Log()
		// Continue execution - don't fail if git check fails
		return true
	}

	if !hasMarkdown {
		// No markdown files staged, exit silently (success)
		if !cli.IsQuiet(cmd) {
			msg := "No markdown files staged, skipping docman-sync\n"
			//nolint:errcheck // Output errors are non-critical
			_ = cli.WriteOutput(cmd, []byte(msg))
		}
		return false
	}

	return true
}

// initializeDocmanRegistry initializes the docman registry
func initializeDocmanRegistry(projectRoot string) (*docman.Registry, error) {
	// Initialize storage provider via StorageFactory for unified storage routing
	factory, err := storage.NewStorageFactory(context.Background(), projectRoot) // Background: request-or-shutdown derived
	if err != nil {
		return nil, errfmt.Newf("failed to initialize storage factory").Wrap(err)
	}
	storageProvider := factory.GetStorageForKind("doc_entry")

	// Create registry
	registry := docman.NewRegistry(storageProvider, projectRoot)

	return registry, nil
}

// updateExistingDocEntries updates existing doc_entries if requested
func updateExistingDocEntries(cmd *cobra.Command, registry *docman.Registry, cmdCtx context.Context, profile string, updateExisting, dryRun, checkOnly bool, logger logging.Logger) (bool, error) {
	if !updateExisting {
		return false, nil
	}

	logging.Fluent(logger).Info("Updating existing doc_entries to include missing reference fields").Log()
	updated, err := registry.UpdateExistingEntries(cmdCtx, profile)
	if err != nil {
		return false, errfmt.Newf("failed to update existing entries").Wrap(err)
	}

	if !cli.IsQuiet(cmd) {
		msg := fmt.Sprintf("Updated %d doc_entry objects with missing reference fields\n", updated)
		//nolint:errcheck // Output errors are non-critical
		_ = cli.WriteOutput(cmd, []byte(msg))
	}

	// If only updating, return here (unless check-only or dry-run)
	if !dryRun && !checkOnly {
		return true, nil
	}

	return false, nil
}

// registerDocumentation registers all documentation
func registerDocumentation(registry *docman.Registry, cmdCtx context.Context, profile string, dryRun, checkOnly bool, logger logging.Logger) (registeredCount, updatedCount int, err error) {
	logging.Fluent(logger).Info("Starting documentation registration").
		String("dry_run", fmt.Sprintf("%v", dryRun)).
		String("check_only", fmt.Sprintf("%v", checkOnly)).
		Log()

	created, skipped, err := registry.RegisterAll(cmdCtx, profile, dryRun || checkOnly)
	if err != nil {
		return 0, 0, errfmt.Newf("failed to register documentation").Wrap(err)
	}

	return created, skipped, nil
}

// handleCheckOnlyMode handles check-only mode
func handleCheckOnlyMode(cmd *cobra.Command, created int) error {
	if created > 0 {
		if !cli.IsQuiet(cmd) {
			msg := fmt.Sprintf("Documentation registration needed: %d files need registration\n", created)
			//nolint:errcheck // Output errors are non-critical
			_ = cli.WriteOutput(cmd, []byte(msg))
		}
		os.Exit(1)
	}

	if !cli.IsQuiet(cmd) {
		msg := "All documentation is registered\n"
		//nolint:errcheck // Output errors are non-critical
		_ = cli.WriteOutput(cmd, []byte(msg))
	}

	return nil
}

// outputRegistrationResults outputs the registration results
func outputRegistrationResults(cmd *cobra.Command, created, skipped int, dryRun bool) {
	if cli.IsQuiet(cmd) {
		return
	}

	var msg string
	if dryRun {
		msg = fmt.Sprintf("Would create %d doc_entry objects\nWould skip %d files (already registered)\n", created, skipped)
	} else {
		msg = fmt.Sprintf("Created %d doc_entry objects\nSkipped %d files (already registered)\n", created, skipped)
	}
	//nolint:errcheck // Output errors are non-critical
	_ = cli.WriteOutput(cmd, []byte(msg))
}
