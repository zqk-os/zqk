package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/brand"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/stampmemo"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// AuthMiddleware intercepts commands to enforce Identity/Role RBAC checks.
func AuthMiddleware(cmd *cobra.Command, projectRoot string) error {
	// Builtins that may lack annotations. Everything else walks AnnRequiresSession
	// (including ancestors) — no command-name allowlists.
	switch cmd.Name() {
	case "help", "version", "completion", "quickstart", "start-here":
		return nil
	}
	if cli.SessionOptional(cmd) {
		return nil
	}

	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background() // Background: request-or-shutdown derived
	}

	isTest := false
	bypassVal := zqkenv.TestBypassAuth().Get()
	if bypassVal == "1" {
		isTest = true
	} else if bypassVal != "0" {
		if testing.Testing() || strings.HasSuffix(os.Args[0], ".test") {
			isTest = true
		}
	}

	if isTest || zqkenv.DevCodegen().Get() == "1" || zqkenv.Codegen().Get() == "1" {
		secCtx := pkgctx.NewTestSecurityContext()
		cmd.SetContext(pkgctx.WithSecurityContext(ctx, secCtx))
		return nil
	}

	// Fail-closed uninitialized kernel hint: if projectRoot is empty or uninitialized,
	// guide the user to run system init instead of failing with missing account schema.
	exe := brand.ExecutableName()
	if projectRoot == "" {
		return errfmt.Errorf("kernel not initialized: run '%s system init' to initialize project kernel", exe)
	}
	if _, statErr := fileutil.Stat(paths.ProjectDataDirPath(projectRoot)); statErr != nil {
		return errfmt.Errorf("kernel not initialized: run '%s system init' to initialize project kernel", exe)
	}
	if specErr := authcred.RequireRBACSpecs(projectRoot); specErr != nil {
		return errfmt.Errorf("kernel not initialized: run '%s system init' to initialize project kernel (failed to load account schema: %v)", exe, specErr)
	}

	rawAuthInput := zqkenv.APIKey().Get()
	credPath := authcred.ResolveCredentialPath(projectRoot)
	credentialsToken := authcred.ReadCredentialToken(credPath)

	if rawAuthInput == "" && credentialsToken == "" {
		return errfmt.Errorf("unauthorized: missing token in ~/%s/credentials or %s", paths.ProjectDataDir, zqkenv.APIKey())
	}

	// Inject SecurityContext for the CLI processor
	authPrincipalCandidate := rawAuthInput
	if authPrincipalCandidate == "" {
		authPrincipalCandidate = credentialsToken
	}

	var resolvedAccountID string
	if authcred.LooksLikeSessionToken(authPrincipalCandidate) {
		acc, err := resolveSessionAccountID(ctx, cmd, projectRoot, authPrincipalCandidate)
		if err != nil {
			return err
		}
		resolvedAccountID = acc
	} else if authcred.LooksLikeIssuedSecret(authPrincipalCandidate) {
		// POL-AGENT-API-KEY-001: opaque secrets resolve via keystore fingerprint.
		match, resolveErr := authcred.ResolveSecret(projectRoot, authPrincipalCandidate)
		if resolveErr != nil {
			return errfmt.Errorf("unauthorized: invalid API key (not an ACC-* id and no keystore match); see POL-AGENT-API-KEY-001")
		}
		resolvedAccountID = match.AccountID
	} else {
		resolvedAccountID = authPrincipalCandidate
	}

	secCtx, err := resolveSecurityContext(projectRoot, resolvedAccountID)
	if err != nil {
		// Fallback to local system account ONLY if the credentials came from a global/home file
		// (~/<brand>/credentials) and that foreign account does not exist in this project,
		// but this project has its own ACC-SYSTEM (COMMUNITY_FIRST_RUN: Leftover ~/.zqk/credentials must not block an empty directory).
		if rawAuthInput == "" && credPath != paths.CredentialsPath(projectRoot) && authcred.HasAccountInIndex(projectRoot, pkgctx.SystemAccountID) {
			if sysCtx, sysErr := resolveSecurityContext(projectRoot, pkgctx.SystemAccountID); sysErr == nil {
				secCtx = sysCtx
				err = nil
				// Self-heal: persist the working local credentials file so future invocations hit the project credentials directly.
				localCred := paths.CredentialsPath(projectRoot)
				if !fileutil.Exists(localCred) {
					_ = fileutil.EnsureDir(filepath.Dir(localCred))
					_ = fileutil.WriteSecureFile(localCred, []byte(pkgctx.SystemAccountID+"\n"))
				}
			}
		}
		if err != nil {
			return err
		}
	}

	// WARNING: We cannot use context.WithValue on cmd.Context() directly if it's already set by cobra,
	// but cmd.SetContext allows overriding it.
	cmd.SetContext(pkgctx.WithSecurityContext(ctx, secCtx))
	authcred.WriteIdentityStatus(projectRoot, authcred.SnapshotFromSecurityContext(secCtx, nil))

	return nil
}

type sessionAccountHit struct {
	sessionID string
	accountID string
}

// sessionAccounts is keyed by project root (one live token per project).
// Session ids live in the value, not the key — an unbounded ZS-/ZQK-* stream
// would leak in a long-lived MCP daemon. See pkg/stampmemo.
var sessionAccounts stampmemo.Table[sessionAccountHit]

func resolveSessionAccountID(ctx context.Context, cmd *cobra.Command, projectRoot, token string) (string, error) {
	sessionID := token
	var explicitPersona string
	if idx := strings.Index(sessionID, "|"); idx != -1 {
		explicitPersona = sessionID[idx:]
		sessionID = sessionID[:idx]
	}
	stamp := stampmemo.OfAll(
		paths.SessionStatePath(projectRoot),
		authcred.ResolveCredentialPath(projectRoot),
	)
	load := func() (sessionAccountHit, error) {
		accID, err := readSessionAccountID(ctx, cmd, projectRoot, sessionID)
		if err != nil {
			return sessionAccountHit{}, err
		}
		return sessionAccountHit{sessionID: sessionID, accountID: accID}, nil
	}
	hit, err := sessionAccounts.Load(projectRoot, stamp, load)
	if err == nil && hit.sessionID != sessionID {
		sessionAccounts.Delete(projectRoot)
		hit, err = sessionAccounts.Load(projectRoot, stamp, load)
	}
	if err != nil {
		return "", err
	}
	return hit.accountID + explicitPersona, nil
}

func readSessionAccountID(ctx context.Context, cmd *cobra.Command, projectRoot, sessionID string) (string, error) {
	sp, err := cli.GetObjectStorageForCommand(cmd, projectRoot)
	if err != nil || sp == nil {
		return "", errfmt.Errorf("unauthorized: storage not available to validate session: %w", err)
	}
	systemSecCtx := pkgctx.NewSystemSecurityContext()
	sessionObj, readErr := sp.Read(ctx, systemSecCtx, sessionID)
	if readErr != nil {
		if errors.Is(readErr, storage.ErrObjectNotFound) {
			if fileSID := strings.TrimSpace(ReadPersistedSessionID(projectRoot)); fileSID != "" && fileSID != sessionID {
				if retry, err2 := sp.Read(ctx, systemSecCtx, fileSID); err2 == nil {
					sessionObj = retry
					readErr = nil
				}
			}
		}
		if readErr != nil {
			if errors.Is(readErr, storage.ErrObjectNotFound) {
				return "", errfmt.Errorf("unauthorized: session %s not found", sessionID)
			}
			return "", errfmt.Errorf("unauthorized: session %s not found: %w", sessionID, readErr)
		}
	}
	if accID, exists := sessionObj[objects.FieldKeyAccountID].(string); exists && accID != "" {
		return accID, nil
	}
	return "", errfmt.Errorf("unauthorized: session %s has no account_id", sessionID)
}

func resolveSecurityContext(projectRoot string, rawAccountID string) (*pkgctx.SecurityContext, error) {
	var explicitPersona string

	// Tripartite Identity parsing: Account|Role|Persona
	parts := strings.Split(rawAccountID, "|")
	accountID := parts[0]
	if len(parts) >= 3 {
		// explicitRole = parts[1] (not currently utilized for filtering, but reserved for future role-based strictness)
		explicitPersona = parts[2]
	} else if len(parts) == 2 {
		explicitPersona = parts[1]
	}

	if explicitPersona == "" {
		explicitPersona = zqkenv.Persona().Get()
	}

	if accountID == pkgctx.SystemAccountID || accountID == "agent-session-token" {
		secCtx := pkgctx.NewSystemSecurityContext()
		secCtx.PersonaID = explicitPersona
		return secCtx, nil
	}
	if accountID == pkgctx.TestHarnessAccountID || accountID == "ACC-TEST-HARNESS" {
		secCtx := pkgctx.NewTestSecurityContext()
		secCtx.PersonaID = explicitPersona
		return secCtx, nil
	}
	if accountID == authcred.DefaultSwarmWorkerAccount {
		secCtx := pkgctx.NewSecurityContext(authcred.DefaultSwarmWorkerAccount, []string{"swarm_worker"}, []string{"*"})
		secCtx.PersonaID = firstNonEmpty(explicitPersona, "PER-SWARM-WORKER")
		return secCtx, nil
	}

	if !strings.HasPrefix(accountID, "ACC-") {
		return nil, errfmt.Errorf("unauthorized: account id must use ACC-* form (got %q); see POL-AGENT-ACCOUNT-LOGIN-001", accountID)
	}

	acc, err := authcred.LookupBoundAccount(projectRoot, accountID)
	if err != nil {
		return nil, err
	}
	roles := acc.Roles
	boundPersona := firstNonEmpty(explicitPersona, acc.PersonaRef, acc.Persona, firstString(acc.PersonaRefs))

	if len(roles) == 0 || boundPersona == "" {
		return nil, errfmt.Errorf("unauthorized: account %s is not RBAC-ready (need roles + persona_ref; CRI-ACCOUNT-RBAC-READY)", accountID)
	}

	activeVocabularySchemes := authcred.VocabularySchemesForPersona(projectRoot, boundPersona)

	// Role permissions + aliases come from role objects (SeatDirectory), not a code table.
	permissions := authcred.PermissionsForAssignedRoles(authcred.NewDiskSeatDirectory(projectRoot), roles)

	return &pkgctx.SecurityContext{
		AccountID:               accountID,
		Roles:                   roles,
		Permissions:             permissions,
		NamespaceID:             "*",
		PersonaID:               boundPersona,
		ActiveVocabularySchemes: activeVocabularySchemes,
	}, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func firstString(vals []string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
