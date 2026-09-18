package automation

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/zqk-os/zqk/internal/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// LintBypassAuditFlags contains parsed lint-bypass-audit command flags
type LintBypassAuditFlags struct {
	GitUser       string
	GitEmail      string
	CommitMessage string
	Files         []string
	AutoDetect    bool
}

// parseLintBypassAuditFlags parses all lint-bypass-audit command flags
func parseLintBypassAuditFlags(cmd *cobra.Command) (*LintBypassAuditFlags, error) {
	flags := &LintBypassAuditFlags{}

	gitUser, err := cmd.Flags().GetString("git-user")
	if err != nil {
		return nil, errfmt.Newf("failed to get git-user flag").Wrap(err)
	}
	flags.GitUser = gitUser

	gitEmail, err := cmd.Flags().GetString("git-email")
	if err != nil {
		return nil, errfmt.Newf("failed to get git-email flag").Wrap(err)
	}
	flags.GitEmail = gitEmail

	commitMessage, err := cmd.Flags().GetString("commit-message")
	if err != nil {
		return nil, errfmt.Newf("failed to get commit-message flag").Wrap(err)
	}
	flags.CommitMessage = commitMessage

	files, err := cmd.Flags().GetStringArray("files")
	if err != nil {
		return nil, errfmt.Newf("failed to get files flag").Wrap(err)
	}
	flags.Files = files

	autoDetect, err := cmd.Flags().GetBool("auto-detect")
	if err != nil {
		return nil, errfmt.Newf("failed to get auto-detect flag").Wrap(err)
	}
	flags.AutoDetect = autoDetect

	return flags, nil
}

// autoDetectContext auto-detects context values if enabled
func autoDetectContext(flags *LintBypassAuditFlags) {
	if !flags.AutoDetect {
		return
	}

	// Auto-detect git user
	if flags.GitUser == emptyValue {
		if detected, err := detectGitConfig("user.name"); err == nil && detected != emptyValue {
			flags.GitUser = detected
		}
	}

	// Auto-detect git email
	if flags.GitEmail == emptyValue {
		if detected, err := detectGitConfig("user.email"); err == nil && detected != emptyValue {
			flags.GitEmail = detected
		}
	}

	// Auto-detect commit message (from HEAD or prepare-commit-msg)
	if flags.CommitMessage == emptyValue {
		if detected, err := detectCommitMessage(); err == nil && detected != emptyValue {
			flags.CommitMessage = detected
		}
	}

	// Auto-detect staged files
	if len(flags.Files) == 0 {
		if detected, err := detectStagedFiles(); err == nil && len(detected) > 0 {
			flags.Files = detected
		}
	}
}

// validateLintBypassAuditFlags validates required flags
func validateLintBypassAuditFlags(flags *LintBypassAuditFlags) error {
	// Git user is minimum requirement
	if flags.GitUser == emptyValue {
		return errfmt.Errorf("git user is required (provide --git-user or ensure git config user.name is set)")
	}
	return nil
}

// createLintBypassAuditEvent creates the audit event via coordinator
func createLintBypassAuditEvent(ctx context.Context, projectRoot string, flags *LintBypassAuditFlags) error {
	// Get storage provider for coordinator
	storageFactory, err := storage.NewStorageFactory(ctx, projectRoot)
	if err != nil {
		// Best effort - fall back to direct call if storage factory fails
		return storage.CreateLintBypassAuditEvent(projectRoot, flags.GitUser, flags.GitEmail, flags.CommitMessage, flags.Files)
	}

	storageProvider := storageFactory.GetStorage()
	if storageProvider == nil {
		// Best effort - fall back to direct call if storage provider is nil
		return storage.CreateLintBypassAuditEvent(projectRoot, flags.GitUser, flags.GitEmail, flags.CommitMessage, flags.Files)
	}

	// Emit via coordinator (async, non-blocking)
	emitLintBypassAuditEventViaCoordinator(
		ctx,
		projectRoot,
		storageProvider,
		flags.GitUser,
		flags.GitEmail,
		flags.CommitMessage,
		flags.Files,
		automationProfileHuman, // Default profile for automation commands
	)

	return nil
}

// outputAuditSuccess outputs success message
func outputAuditSuccess(cmd *cobra.Command, projectRoot string) {
	if cli.IsQuiet(cmd) {
		return
	}

	auditDir := filepath.Join(projectRoot, paths.ProcessAuditDir)
	msg := fmt.Sprintf("Created lint bypass audit event in %s\n", auditDir)
	//nolint:errcheck // Output errors are non-critical
	_ = cli.WriteOutput(cmd, []byte(msg))
}
