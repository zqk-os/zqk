package automation

import (
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/execwrap"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// NewLintBypassAuditCmd creates a command to create lint bypass audit events
func NewLintBypassAuditCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Create an audit event for lint check bypasses",
		"Create an audit event when lint checks are bypassed with --no-verify.",
		"",
		"This command is designed to be called from git hooks and wrapper scripts to track",
		"when lint checks are bypassed, helping identify when lint warnings increase and",
		"who is not taking time to resolve lint checks.",
		"",
		"The command creates an audit event in "+filepath.Join(paths.ProcessAuditDir, "YYYY-MM")+"/ with:",
		"  - Git user information (name and email)",
		"  - Commit message",
		"  - List of staged files",
		"  - Count of Go files affected",
		"",
		"The command automatically detects:",
		"  - Git user name from 'git config user.name'",
		"  - Git email from 'git config user.email'",
		"  - Commit message from HEAD or COMMIT_EDITMSG",
		"  - Staged files from 'git diff --cached --name-only'",
	).
		AddExample("Fully automatic (all context auto-detected from git)", "%s automation lint-bypass-audit").
		AddExample("Override specific values while auto-detecting others", "%s automation lint-bypass-audit --commit-message \"Custom message\"").
		AddExample("Explicit values (disable auto-detection)", "%s automation lint-bypass-audit --auto-detect=false --git-user \"John Doe\" --git-email \"john@example.com\" --commit-message \"fix: update code\" --files \"pkg/storage/file.go\"").
		ExcludeCommonFlags()

	cmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAutomationLintBypassAuditCommandBuilder(), &cobra.Command{
		Use:  "lint-bypass-audit",
		RunE: runLintBypassAudit,
	})

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cmd.Flags().String("git-user", "", "Git user name (auto-detected from git config if not provided)")
	cmd.Flags().String("git-email", "", "Git user email (auto-detected from git config if not provided)")
	cmd.Flags().String("commit-message", "", "Commit message (auto-detected from HEAD or staged changes if not provided)")
	cmd.Flags().StringArray("files", []string{}, "List of staged/changed files (auto-detected from git diff --cached if not provided)")
	cmd.Flags().Bool("auto-detect", true, "Automatically detect context from git (default: true)")

	return cmd
}

func runLintBypassAudit(cmd *cobra.Command, args []string) error {
	// Get project root
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == emptyValue {
		cliCmd := paths.CLICommandName
		if cliCmd == emptyValue {
			cliCmd = paths.CLICommandNameDefault
		}
		return errfmt.Errorf("project root not found - must be run from within a %s project", cliCmd)
	}

	// Parse flags
	flags, err := parseLintBypassAuditFlags(cmd)
	if err != nil {
		return err
	}

	// Auto-detect context if enabled
	autoDetectContext(flags)

	// Validate flags
	if err := validateLintBypassAuditFlags(flags); err != nil {
		return err
	}

	// Create audit event via coordinator
	ctx := pkgctx.NewSystemContext()
	if err := createLintBypassAuditEvent(ctx, projectRoot, flags); err != nil {
		return err
	}

	// Output success message
	outputAuditSuccess(cmd, projectRoot)

	return nil
}

// detectGitConfig runs `git config <key>` and returns the value
func detectGitConfig(key string) (string, error) {
	cmd := execwrap.Command("git", "config", key)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

// detectCommitMessage tries to get commit message from various sources
func detectCommitMessage() (string, error) {
	// First, try to get from HEAD (if commit already exists)
	cmd := execwrap.Command("git", "log", "-1", "--pretty=%B", "HEAD")
	output, err := cmd.Output()
	if err == nil && len(output) > 0 {
		msg := strings.TrimSpace(string(output))
		if msg != emptyValue {
			return msg, nil
		}
	}

	// If that fails, try to get from prepare-commit-msg hook context
	// (COMMIT_EDITMSG file contains the message being committed)
	gitDirCmd := execwrap.Command("git", "rev-parse", "--git-dir")
	gitDirOutput, err := gitDirCmd.Output()
	if err == nil {
		gitDir := strings.TrimSpace(string(gitDirOutput))
		commitEditMsg := filepath.Join(gitDir, "COMMIT_EDITMSG")
		if data, err := fileutil.ReadFile(commitEditMsg); err == nil {
			msg := strings.TrimSpace(string(data))
			if msg != emptyValue {
				return msg, nil
			}
		}
	}

	// Try to get from MERGE_MSG (during merge commits)
	if gitDirOutput != nil {
		gitDir := strings.TrimSpace(string(gitDirOutput))
		mergeMsg := filepath.Join(gitDir, "MERGE_MSG")
		if data, err := fileutil.ReadFile(mergeMsg); err == nil {
			msg := strings.TrimSpace(string(data))
			if msg != emptyValue {
				return msg, nil
			}
		}
	}

	return "", errfmt.Errorf("could not detect commit message")
}

// detectStagedFiles gets list of staged files from git
func detectStagedFiles() ([]string, error) {
	cmd := execwrap.Command("git", "diff", "--cached", "--name-only")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	if len(output) == 0 {
		return []string{}, nil
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var files []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != emptyValue {
			files = append(files, line)
		}
	}

	return files, nil
}
