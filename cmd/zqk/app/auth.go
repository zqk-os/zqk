// Package app: auth command group (login, logout) for session lifecycle.
// Login creates or reuses a session and persists it (file + env hint). Logout ends the session and clears state.

package app

import (
	"context"
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// NewAuthCmd returns the top-level auth command (login, logout).
func NewAuthCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Session and authentication",
		"Login and logout for CLI session lifecycle.",
		"",
		"Session ID can be stored in ZQK_SESSION_ID (env) or in .zqk/state/session (file).",
		"Multiple terminals share the same session via the file; a file lock ensures consistency.",
		"Use 'auth login' to create or reuse a session; use 'auth logout' to end it.",
	).
		AddExample("Create or reuse session", "%s auth login").
		AddExample("End session", "%s auth logout")

	authCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAppAuthCommandBuilder(), &cobra.Command{
		Use: "auth",
	})
	helpBuilder.ApplyToCommand(authCmd)
	authCmd.AddCommand(NewLoginCmd())
	authCmd.AddCommand(NewLogoutCmd())
	return authCmd
}

// NewLoginCmd returns the login command.
func NewLoginCmd() *cobra.Command {
	loginCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAppLoginCommandBuilder(), &cobra.Command{
		Use:   "login",
		Short: "Create or reuse a CLI session (persisted for multiple terminals)",
		Long:  "Creates a new session or reuses the current one if still active. Session is written to .zqk/state/session (under lock) so other terminals can reuse it. Optionally set ZQK_SESSION_ID in your shell to tie the session to this process tree.",
	})
	cli.RequireSession(loginCmd, false) // login owns create/reuse; root must not start session
	cli.BindAsyncProgress(loginCmd, runLogin)
	cli.AddCommonFlags(loginCmd)
	return loginCmd
}

func runLogin(cmd *cobra.Command, _ []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == EmptyValue {
		return errfmt.Errorf("project root not found (run from project dir or set ZQK_PROJECT_ROOT)")
	}
	sp, err := cli.GetObjectStorageForCommand(cmd, projectRoot)
	if err != nil || sp == nil {
		return errfmt.Newf("storage not available").Wrap(err)
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background() // Background: request-or-shutdown derived
	}
	title := cmd.CommandPath()
	accountID := pkgctx.SystemAccountID
	if secCtx := pkgctx.GetSecurityContext(ctx); secCtx != nil && secCtx.AccountID != "" {
		accountID = secCtx.AccountID
	}
	sessionID, reused := TryReuseSession(ctx, projectRoot, title, accountID, sp)
	if !reused {
		sessionID = StartZqkSession(ctx, projectRoot, title, accountID, sp)
		if sessionID == EmptyValue {
			return errfmt.Errorf("failed to create session")
		}
		WritePersistedSessionID(ctx, projectRoot, sessionID, accountID, sp)
	}
	profile := profileHuman
	if c := cli.GetContext(cmd); c != nil {
		profile = c.Profile
	}
	logger := logging.GetLoggerFromProfile(profile)
	if reused {
		logging.Fluent(logger).Info("Using existing session").
			String("session_id", sessionID).
			Log()
	} else {
		logging.Fluent(logger).Info("Session created").
			String("session_id", sessionID).
			Log()
	}

	// Persist the token to ~/.zqk/credentials
	home, err := fileutil.UserHomeDir()
	if err == nil {
		credDir := filepath.Join(home, paths.ProjectDataDir)
		_ = fileutil.MkdirAll(credDir, 0700)
		credPath := filepath.Join(credDir, "credentials")
		_ = fileutil.WriteSecureFile(credPath, []byte(sessionID))
	}

	// Hint for tying session to this shell (env is process-specific; file is shared across terminals)
	logging.Fluent(logger).Info("To use this session in this shell only, run: export " + zqkenv.SessionID().Name() + "=" + sessionID).Log()
	return cli.WriteOutput(cmd, []byte("session_id: "+sessionID+"\n"))
}

// NewLogoutCmd returns the logout command.
func NewLogoutCmd() *cobra.Command {
	logoutCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewAppLogoutCommandBuilder(), &cobra.Command{
		Use:   "logout",
		Short: "End the current CLI session and clear persisted state",
		Long:  "Ends the current session (status set to completed), removes .zqk/state/session, and optionally unset ZQK_SESSION_ID in your shell.",
	})
	cli.RequireSession(logoutCmd, false) // logout ends session; root must not start/reuse
	cli.BindAsyncProgress(logoutCmd, runLogout)
	cli.AddCommonFlags(logoutCmd)
	return logoutCmd
}

func runLogout(cmd *cobra.Command, _ []string) error {
	projectRoot := cli.ResolveProjectRoot(".")
	if projectRoot == EmptyValue {
		return errfmt.Errorf("project root not found")
	}
	sessionID := GetCurrentSessionID(projectRoot)
	if sessionID == EmptyValue {
		profile := profileHuman
		if c := cli.GetContext(cmd); c != nil {
			profile = c.Profile
		}
		logging.Fluent(logging.GetLoggerFromProfile(profile)).Info("No current session").Log()
		return nil
	}
	accountID := pkgctx.SystemAccountID
	if secCtx := pkgctx.GetSecurityContext(cmd.Context()); secCtx != nil && secCtx.AccountID != "" {
		accountID = secCtx.AccountID
	}
	sp, err := cli.GetObjectStorageForCommand(cmd, projectRoot)
	if err == nil && sp != nil {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background() // Background: request-or-shutdown derived
		}
		EndZqkSession(ctx, projectRoot, sessionID, zqkStatusDone, accountID, sp)
	}
	ClearPersistedSession(projectRoot)
	profile := profileHuman
	if c := cli.GetContext(cmd); c != nil {
		profile = c.Profile
	}
	logging.Fluent(logging.GetLoggerFromProfile(profile)).Info("Session ended").
		String("session_id", sessionID).
		Log()
	logging.Fluent(logging.GetLoggerFromProfile(profile)).Info(
		"To clear from your shell, run: unset " + zqkenv.SessionID().Name(),
	).Log()
	return nil
}
