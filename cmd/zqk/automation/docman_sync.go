package automation

import (
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
)

// NewDocmanSyncCmd creates a command to sync documentation registration
func NewDocmanSyncCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Sync documentation registration (discover and register markdown files)",
		"Sync documentation registration by discovering and registering markdown files as doc_entry objects.",
		"",
		"This command is designed for automation contexts (git hooks, CI/CD pipelines, scripts) to ensure",
		"documentation is automatically registered when new markdown files are added.",
		"",
		"The command:",
		"  - Scans the docs/ directory for markdown files",
		"  - Extracts metadata (title, summary, group, category) from each file",
		"  - Creates doc_entry objects for files that don't already have entries",
		"  - Skips files that already have registered doc_entry objects",
		"",
		"When --git-aware is enabled, the command only runs if markdown files are staged or changed.",
		"This makes it safe to use in git hooks without unnecessary overhead.",
	).
		AddExample("Sync all documentation (register any new files)", "%s automation docman-sync").
		AddExample("Only sync if markdown files are staged (for git hooks)", "%s automation docman-sync --git-aware").
		AddExample("Check if registration is needed (exit 1 if needed, 0 if up-to-date)", "%s automation docman-sync --check-only").
		AddExample("Dry run to see what would be registered", "%s automation docman-sync --dry-run").
		AddExample("Update existing entries with missing reference fields", "%s automation docman-sync --update-existing").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAutomationDocmanSyncCommandBuilder(

	// Apply help builder to command
	), &cobra.Command{
		Use:  "docman-sync",
		RunE: runDocmanSync,
	})

	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().Bool("check-only", false, "Only check if docs need registration (exit 1 if needed, 0 if up-to-date)")
	cmd.Flags().Bool("git-aware", false, "Only run if markdown files are staged/changed (for git hooks)")
	cmd.Flags().Bool("update-existing", false, "Update existing doc_entries to include missing reference fields per spec")
	cmd.Flags().Bool("dry-run", false, "Show what would be registered without actually creating objects")

	// Add common flags
	cli.AddCommonFlags(cmd)

	return cmd
}

func runDocmanSync(cmd *cobra.Command, args []string) error {
	// Get command context
	cmdCtx := getCommandContext(cmd)

	// Parse flags
	flags, err := parseDocmanSyncFlags(cmd)
	if err != nil {
		return err
	}

	logger := logging.GetLoggerFromProfile(flags.Profile)
	cliCmd := paths.CLICommandName
	if cliCmd == emptyValue {
		cliCmd = paths.CLICommandNameDefault
	}

	// Get project root
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not found - must be run from within a %s project", cliCmd)
	}

	// Check git-aware sync
	shouldContinue := checkGitAwareSync(cmd, projectRoot, flags.GitAware, logger)
	if !shouldContinue {
		return nil
	}

	// Initialize registry
	registry, err := initializeDocmanRegistry(projectRoot)
	if err != nil {
		return err
	}

	// Update existing entries if requested
	shouldReturn, err := updateExistingDocEntries(cmd, registry, cmdCtx, flags.Profile, flags.UpdateExisting, flags.DryRun, flags.CheckOnly, logger)
	if err != nil {
		return err
	}
	if shouldReturn {
		return nil
	}

	// Register documentation
	created, skipped, err := registerDocumentation(registry, cmdCtx, flags.Profile, flags.DryRun, flags.CheckOnly, logger)
	if err != nil {
		return err
	}

	// Handle check-only mode
	if flags.CheckOnly {
		return handleCheckOnlyMode(cmd, created)
	}

	// Output results
	outputRegistrationResults(cmd, created, skipped, flags.DryRun)

	return nil
}

// hasStagedMarkdownFiles checks if there are any staged markdown files in git
func hasStagedMarkdownFiles(projectRoot string) (bool, error) {
	// Check if we're in a git repository
	gitDirCmd := execwrap.Command("git", "-C", projectRoot, "rev-parse", "--git-dir")
	if err := gitDirCmd.Run(); err != nil {
		return false, errfmt.Newf("not a git repository").Wrap(err)
	}

	// Get list of staged files
	cmd := execwrap.Command("git", "-C", projectRoot, "diff", "--cached", "--name-only")
	output, err := cmd.Output()
	if err != nil {
		return false, errfmt.Newf("failed to get staged files").Wrap(err)
	}

	// Check if any staged files are markdown
	for line := range strings.SplitSeq(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == emptyValue {
			continue
		}
		// Check if file has .md extension
		if strings.HasSuffix(strings.ToLower(line), ".md") {
			return true, nil
		}
	}

	return false, nil
}
