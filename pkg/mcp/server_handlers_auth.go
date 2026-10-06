package mcp

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"context"
	"crypto/subtle"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/authcred"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/id_generation"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// key_type values for keystore entries
const (
	keyTypePassword            = "password"
	keyTypePersonalAccessToken = "personal_access_token"
	keyTypeAPIKey              = "api_key"
)

// validateProjectRootForProcessData ensures project root is the real repo root, not source (pkg/mcp, cmd/, etc.).
// Prevents writing process data (mcp_sessions, etc.) into package or command directories.
func validateProjectRootForProcessData(projectRoot string) error {
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root not available")
	}
	abs, err := filepath.Abs(projectRoot)
	if err != nil {
		abs = filepath.Clean(projectRoot)
	}
	normalized := filepath.ToSlash(abs)
	// Reject if project root is inside pkg/mcp or cmd (would corrupt source tree)
	if strings.Contains(normalized, "/pkg/mcp") || strings.Contains(normalized, "/cmd/") {
		return errfmt.Errorf("project root must be the repository root, not a package or command path (got %s)", projectRoot)
	}
	// Require .zqk/process to exist so we don't write into wrong place
	docsProcess := datacell.ProcessPrimaryDir(abs)
	if info, err := fileutil.Stat(docsProcess); err != nil || !info.IsDir() {
		return errfmt.Errorf("project root must contain %s directory (got %s)", paths.ProcessDir, projectRoot)
	}
	return nil
}

// createAuthenticationSession creates an authentication session object for an unauthenticated client
// Returns the session ID if successful, or empty string and error if creation fails
// Uses direct file I/O to avoid import cycle with storage package
func (s *Server) createAuthenticationSession(ctx context.Context, clientID, clientName, accountID string) (string, error) {
	// Get project root and ensure it is the real repo root (never pkg/mcp or cmd/)
	projectRoot := s.GetProjectRoot()
	if err := validateProjectRootForProcessData(projectRoot); err != nil {
		return "", err
	}

	// Sessions directory (must exist before we allocate an ID so the sequence file can be created)
	sessionsDir := datacell.CellCASPrimaryDir(projectRoot, "mcp_sessions")
	if err := fileutil.MkdirAll(sessionsDir, paths.DirPerm755); err != nil {
		return "", errfmt.Newf("failed to create sessions directory").Wrap(err)
	}

	// Generate session ID using the same sequential mechanism as other kinds (cross-process safe)
	generator := id_generation.GetBatchIDGenerator(ctx, sessionsDir, objects.KindMcpSession, "MCP", 3, 1)
	sessionID, err := generator.GenerateNextID()
	if err != nil {
		return "", errfmt.Newf("failed to generate session ID").Wrap(err)
	}

	// Get namespace_id for mcp_session kind (required for validation)
	registry := validation.GetNamespaceRegistry()
	namespaceID := registry.GetNamespaceForKind(objects.KindMcpSession)

	// Create session object
	sessionObj := map[string]any{
		fieldKind:                        objects.KindMcpSession,
		fieldID:                          sessionID,
		sessionFieldClientID:             clientID,
		sessionFieldClientName:           clientName,
		sessionFieldAuthenticationStatus: mcpSessionAuthPending,
		sessionFieldLastActivity:         zqktime.NowRFC3339UTC(),
		fieldCreatedAt:                   zqktime.NowRFC3339UTC(),
		fieldCreatedBy:                   pkgctx.SystemAccountID,
		fieldUpdatedAt:                   zqktime.NowRFC3339UTC(),
		fieldUpdatedBy:                   pkgctx.SystemAccountID,
		fieldStatus:                      objects.ObjectStatusInProgress,
		fieldOriginProject:               "zqk",
		fieldOriginSystem:                "zqk",
		fieldSchemaVersion:               objects.DefaultSchemaVersion,
	}
	if accountID != emptyValue {
		sessionObj[clientInfoAccountID] = accountID
	}

	// Write session object directly to file (avoiding storage import cycle)
	sessionFile := filepath.Join(sessionsDir, fmt.Sprintf("%s.yaml", sessionID))

	// Write YAML file (simplified - just create the file with basic structure)
	// In production, this should use proper YAML marshaling
	file, err := fileutil.Create(sessionFile)
	if err != nil {
		return "", errfmt.Newf("failed to create session file").Wrap(err)
	}
	defer file.Close()

	// Write basic YAML structure
	fmt.Fprintf(file, "%s: %s\n", fieldID, sessionID)
	fmt.Fprintf(file, "%s: %s\n", fieldKind, objects.KindMcpSession)
	fmt.Fprintf(file, "%s: %s\n", fieldNamespaceID, namespaceID)
	fmt.Fprintf(file, "%s: %s\n", sessionFieldClientID, clientID)
	fmt.Fprintf(file, "%s: %s\n", sessionFieldClientName, clientName)
	fmt.Fprintf(file, "%s: %s\n", sessionFieldAuthenticationStatus, mcpSessionAuthPending)
	fmt.Fprintf(file, "%s: %s\n", sessionFieldLastActivity, zqktime.NowRFC3339UTC())
	if accountID != emptyValue {
		fmt.Fprintf(file, "%s: %s\n", clientInfoAccountID, accountID)
	}
	fmt.Fprintf(file, "%s: %s\n", fieldCreatedAt, zqktime.NowRFC3339UTC())
	fmt.Fprintf(file, "%s: %s\n", fieldCreatedBy, pkgctx.SystemAccountID)
	fmt.Fprintf(file, "%s: %s\n", fieldUpdatedAt, zqktime.NowRFC3339UTC())
	fmt.Fprintf(file, "%s: %s\n", fieldUpdatedBy, pkgctx.SystemAccountID)
	fmt.Fprintf(file, "%s: in_progress\n", fieldStatus)
	fmt.Fprintf(file, "%s: zqk\n", fieldOriginProject)
	fmt.Fprintf(file, "%s: zqk\n", fieldOriginSystem)
	fmt.Fprintf(file, "%s: %s\n", fieldSchemaVersion, objects.DefaultSchemaVersion)

	return sessionID, nil
}

// updateAuthenticationSession updates an authentication session when authentication succeeds
// Uses direct file I/O to avoid import cycle with storage package
func (s *Server) updateAuthenticationSession(_ context.Context, sessionID string, secCtx *pkgctx.SecurityContext, _ map[string]any) error {
	projectRoot := s.GetProjectRoot()
	if err := validateProjectRootForProcessData(projectRoot); err != nil {
		return err
	}

	// Read existing session file
	sessionsDir := datacell.CellCASPrimaryDir(projectRoot, "mcp_sessions")
	sessionFile := filepath.Join(sessionsDir, fmt.Sprintf("%s.yaml", sessionID))

	// Check if file exists
	if _, err := fileutil.Stat(sessionFile); fileutil.IsNotExist(err) {
		return errfmt.Errorf("authentication session not found: %s", sessionID)
	}

	// For now, just log the update - full YAML parsing/updating would require more complex logic
	// In production, this should properly parse and update the YAML file
	// For MVP, we'll just append update info or rewrite the file
	file, err := fileutil.OpenFile(sessionFile, fileutil.O_APPEND|fileutil.O_WRONLY, paths.FilePerm644)
	if err != nil {
		return errfmt.Newf("failed to open session file").Wrap(err)
	}
	defer file.Close()

	// Append update information
	fmt.Fprintf(file, "\n# Authentication completed\n")
	fmt.Fprintf(file, "%s: %s\n", sessionFieldAuthenticationStatus, mcpSessionAuthAuthenticated)
	fmt.Fprintf(file, "%s: %s\n", sessionFieldLastActivity, zqktime.NowRFC3339UTC())
	if secCtx != nil {
		if secCtx.AccountID != emptyValue {
			fmt.Fprintf(file, "%s: %s\n", clientInfoAccountID, secCtx.AccountID)
		}
		if len(secCtx.Roles) > 0 {
			fmt.Fprintf(file, "%s:\n", clientInfoRoles)
			for _, role := range secCtx.Roles {
				fmt.Fprintf(file, "  - %s\n", role)
			}
		}
		if len(secCtx.Permissions) > 0 {
			fmt.Fprintf(file, "%s:\n", clientInfoPermissions)
			for _, perm := range secCtx.Permissions {
				fmt.Fprintf(file, "  - %s\n", perm)
			}
		}
	}
	fmt.Fprintf(file, "%s: %s\n", fieldUpdatedAt, zqktime.NowRFC3339UTC())
	fmt.Fprintf(file, "%s: %s\n", fieldUpdatedBy, pkgctx.SystemAccountID)

	return nil
}

// hasCredentials checks if any credentials are provided in clientInfo
func hasCredentials(clientInfo map[string]any) bool {
	if keystoreKeyID, ok := clientInfo[clientInfoKeystoreKeyID].(string); ok && keystoreKeyID != emptyValue {
		return true
	}
	if username, ok := clientInfo[clientInfoUsername].(string); ok && username != emptyValue {
		return true
	}
	if password, ok := clientInfo[clientInfoPassword].(string); ok && password != emptyValue {
		return true
	}
	if oauthToken, ok := clientInfo[clientInfoOAuthTok].(string); ok && oauthToken != emptyValue {
		return true
	}
	if pat, ok := clientInfo[clientInfoPersonalAccessToken].(string); ok && pat != emptyValue {
		return true
	}
	return false
}

// validateCredentialsAndResolveAccount validates credentials and resolves the account
// Returns: accountID, roles, permissions, error
func (s *Server) validateCredentialsAndResolveAccount(ctx context.Context, clientInfo map[string]any, initParams ...InitializeParams) (accountID string, roles, permissions []string, err error) {
	projectRoot := s.GetProjectRoot()
	if projectRoot == emptyValue {
		return "", nil, nil, errfmt.Errorf("project root not available")
	}

	// Check for keystore key ID authentication
	if keystoreKeyID, ok := clientInfo[clientInfoKeystoreKeyID].(string); ok && keystoreKeyID != emptyValue {
		return s.validateKeystoreKey(ctx, keystoreKeyID, projectRoot)
	}

	// Check for username/password authentication
	if username, ok := clientInfo[clientInfoUsername].(string); ok && username != emptyValue {
		password, hasPassword := clientInfo[clientInfoPassword].(string)
		if (!hasPassword || password == emptyValue) && len(initParams) > 0 && initParams[0].Capabilities != nil {
			password, hasPassword = initParams[0].Capabilities["password"].(string)
		}
		if !hasPassword || password == emptyValue {
			return "", nil, nil, errfmt.Errorf("password required for username authentication")
		}
		return s.validateUsernamePassword(ctx, username, password, projectRoot)
	}

	// Check for OAuth token authentication
	if oauthToken, ok := clientInfo[clientInfoOAuthTok].(string); ok && oauthToken != emptyValue {
		return s.validateOAuthToken(ctx, oauthToken, projectRoot)
	}

	// Check for Personal Access Token (PAT) authentication
	if pat, ok := clientInfo[clientInfoPersonalAccessToken].(string); ok && pat != emptyValue {
		return s.validatePersonalAccessToken(ctx, pat, projectRoot)
	}

	return "", nil, nil, errfmt.Errorf("no valid credentials provided")
}

// validateUsernamePassword validates username/password and resolves account.
// Password is checked against a keystore entry with key_type "password" for the account
// (credential_hash must be a bcrypt hash). If no password keystore entry exists, returns
// "invalid username or password".
func (s *Server) validateUsernamePassword(_ context.Context, username, password, projectRoot string) (accountID string, roles, permissions []string, err error) {
	// Prefer ACC-* via migrate map / username index (account:username files are retired).
	// TRACK: / f021fe6f02
	accountID = authcred.CanonicalAccountID(projectRoot, "account:"+strings.ToLower(username))
	var accountObj map[string]any
	if accountID != emptyValue {
		accountObj, err = s.loadAccountObject(accountID, projectRoot)
	}
	if err != nil || accountObj == nil {
		accountObj, err = s.loadAccountByEmail(username, projectRoot)
		if err != nil {
			return "", nil, nil, errfmt.Errorf("invalid username or password")
		}
	}
	if id, ok := accountObj[objects.FieldKeyID].(string); ok && id != emptyValue {
		accountID = id
	}

	status, _ := accountObj[objects.FieldKeyStatus].(string)
	if status != objects.ObjectStatusActive {
		return "", nil, nil, errfmt.Errorf("account is not active")
	}

	// Find password keystore entry for this account
	keyEntry, _, err := s.findKeystoreEntryByAccountAndType(projectRoot, accountID, keyTypePassword)
	if err != nil || keyEntry == nil {
		return "", nil, nil, errfmt.Errorf("invalid username or password")
	}

	// Check revoked/expired (same as validateKeystoreKey)
	if revoked, ok := keyEntry[objects.FieldKeyRevoked].(bool); ok && revoked {
		return "", nil, nil, errfmt.Errorf("invalid username or password")
	}
	if expiresAt, ok := keyEntry[objects.FieldKeyExpiresAt].(string); ok && expiresAt != emptyValue {
		expiresTime, parseErr := time.Parse(time.RFC3339, expiresAt)
		if parseErr == nil && time.Now().UTC().After(expiresTime) {
			return "", nil, nil, errfmt.Errorf("invalid username or password")
		}
	}

	storedHash, ok := keyEntry[objects.FieldKeyCredentialHash].(string)
	if !ok || storedHash == emptyValue {
		return "", nil, nil, errfmt.Errorf("invalid username or password")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(password)); err != nil {
		return "", nil, nil, errfmt.Errorf("invalid username or password")
	}

	roles, permissions = extractRolesAndPermissions(accountObj)
	return accountID, roles, permissions, nil
}

// validateKeystoreKey validates a keystore key and resolves account
// Note: This validates the key exists and is active, but does NOT validate the credential
// Credential validation should happen when the key is used for authentication
//
//nolint:unparam // permissions return kept for interface consistency
func (s *Server) validateKeystoreKey(_ context.Context, keyID, projectRoot string) (accountID string, roles, permissions []string, err error) {
	// Load keystore entry directly (system can see all fields)
	keyEntry, err := s.loadKeystoreEntry(keyID, projectRoot)
	if err != nil {
		return "", nil, nil, errfmt.Newf("keystore key not found").Wrap(err)
	}

	// Check if key is revoked
	if revoked, ok := keyEntry[objects.FieldKeyRevoked].(bool); ok && revoked {
		return "", nil, nil, errfmt.Errorf("keystore key has been revoked")
	}

	// Check if key is expired
	if expiresAt, ok := keyEntry[objects.FieldKeyExpiresAt].(string); ok && expiresAt != emptyValue {
		expiresTime, err := time.Parse(time.RFC3339, expiresAt)
		if err == nil && time.Now().UTC().After(expiresTime) {
			return "", nil, nil, errfmt.Errorf("keystore key has expired")
		}
	}

	// Get account ID from keystore entry
	accountID, ok := keyEntry[clientInfoAccountID].(string)
	if !ok || accountID == emptyValue {
		return "", nil, nil, errfmt.Errorf("keystore key has no associated account")
	}

	// Update last_used_at
	updates := map[string]any{
		objects.FieldKeyLastUsedAt: zqktime.NowRFC3339UTC(),
	}
	if err := s.updateKeystoreEntry(keyID, projectRoot, updates); err != nil {
		// Log but don't fail authentication
		if s.getTraceWriter() != nil {
			logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
			logging.Fluent(logger).Warn("Failed to update keystore entry last_used_at").
				WithError(err).
				EmitComponent("mcp_server").
				String("trace", "true").
				Log()
		}
	}

	roles, permissions, err = s.loadAccountRolesAndPermissions(accountID, projectRoot)
	if err != nil {
		return "", nil, nil, err
	}
	return accountID, roles, permissions, nil
}

// findKeystoreEntryByAccountAndType finds a keystore entry by account_id and key_type.
// Lists .zqk/process/keystore/*.yaml, parses each, and returns the first match.
// Returns (entry, keyID, nil) or (nil, "", err) if not found.
func (s *Server) findKeystoreEntryByAccountAndType(projectRoot, accountID, keyType string) (map[string]any, string, error) {
	recs, err := authcred.ListKeystoreRecords(projectRoot)
	if err != nil {
		return nil, "", err
	}
	for _, rec := range recs {
		if rec.AccountID() == accountID && rec.KeyType() == keyType {
			return rec.Entry, rec.KeyID, nil
		}
	}
	return nil, "", errfmt.Errorf("no keystore entry for account %s with key_type %s", accountID, keyType)
}

// keystoreEntryWithID holds a keystore entry and its file key ID
type keystoreEntryWithID struct {
	Entry map[string]any
	KeyID string
}

// listKeystoreEntriesByKeyTypes lists keystore entries whose key_type is in keyTypes.
// Reads .zqk/process/keystore/*.yaml and returns matching entries with their key IDs.
func (s *Server) listKeystoreEntriesByKeyTypes(projectRoot string, keyTypes map[string]bool) ([]keystoreEntryWithID, error) {
	recs, err := authcred.ListKeystoreRecords(projectRoot)
	if err != nil {
		return nil, err
	}
	var result []keystoreEntryWithID
	for _, rec := range recs {
		if keyTypes[rec.KeyType()] {
			result = append(result, keystoreEntryWithID{Entry: rec.Entry, KeyID: rec.KeyID})
		}
	}
	return result, nil
}

// normalizeCredentialHashForPAT returns the hex part of credential_hash for PAT comparison.
// Supports stored format "sha256:hex" or plain "hex".
func normalizeCredentialHashForPAT(stored string) string {
	const prefix = "sha256:"
	if strings.HasPrefix(stored, prefix) {
		return strings.TrimSpace(stored[len(prefix):])
	}
	return strings.TrimSpace(stored)
}

// loadKeystoreEntry loads a keystore entry by ID
func (s *Server) loadKeystoreEntry(keyID, projectRoot string) (map[string]any, error) {
	recs, err := authcred.ListKeystoreRecords(projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to read keystore entry").Wrap(err)
	}
	for _, rec := range recs {
		if rec.KeyID == keyID || rec.FileID == keyID {
			return rec.Entry, nil
		}
	}
	return nil, errfmt.Errorf("failed to read keystore entry")
}

// updateKeystoreEntry updates a keystore entry
func (s *Server) updateKeystoreEntry(keyID, projectRoot string, updates map[string]any) error {
	keystoreDir := paths.KeystoreDirPath(projectRoot)
	keyFile := filepath.Join(keystoreDir, fmt.Sprintf("%s.yaml", keyID))

	// Read existing entry
	data, err := fileutil.ReadFile(keyFile)
	if err != nil {
		return errfmt.Newf("failed to read keystore entry").Wrap(err)
	}

	var entry map[string]any
	if err := yaml.Unmarshal(data, &entry); err != nil {
		return errfmt.Newf("failed to parse keystore entry").Wrap(err)
	}

	// Apply updates
	for k, v := range updates {
		entry[k] = v
	}

	// Update metadata
	entry[fieldUpdatedAt] = zqktime.NowRFC3339UTC()
	entry[fieldUpdatedBy] = SystemAccountID

	// Write back
	updatedData, err := yaml.Marshal(entry)
	if err != nil {
		return errfmt.Newf("failed to marshal keystore entry").Wrap(err)
	}

	if err := fileutil.WriteFile(keyFile, updatedData, paths.FilePerm600); err != nil {
		return errfmt.Newf("failed to write keystore entry").Wrap(err)
	}
	authcred.InvalidateKeystore(projectRoot)
	return nil
}

// validateOAuthToken validates OAuth token and resolves account.
// Not yet implemented: JWT or OAuth introspection can be added per docs/refactoring/BLI-952-AUTH-PLAN.md Phase 3.
func (s *Server) validateOAuthToken(_ context.Context, token, projectRoot string) (accountID string, roles, permissions []string, err error) {
	return "", nil, nil, errfmt.Errorf("OAuth token authentication not yet implemented")
}

// validatePersonalAccessToken validates PAT and resolves account.
// PAT is the secret; we compare its SHA256 digest to keystore entries with key_type
// "personal_access_token" or "api_key" (credential_hash stored as hex or "sha256:hex").
// On match we check revoked/expired, then load account and return roles/permissions.
func (s *Server) validatePersonalAccessToken(_ context.Context, pat, projectRoot string) (accountID string, roles, permissions []string, err error) {
	pat = strings.TrimSpace(pat)
	if pat == emptyValue {
		return "", nil, nil, errfmt.Errorf("empty personal access token")
	}
	patHashHex := authcred.NormalizeCredentialHash(authcred.HashAPIKey(pat))

	keyTypes := map[string]bool{keyTypePersonalAccessToken: true, keyTypeAPIKey: true}
	entries, err := s.listKeystoreEntriesByKeyTypes(projectRoot, keyTypes)
	if err != nil {
		return "", nil, nil, errfmt.Newf("failed to list keystore").Wrap(err)
	}

	for _, item := range entries {
		entry := item.Entry
		storedHash, ok := entry[objects.FieldKeyCredentialHash].(string)
		if !ok || storedHash == emptyValue {
			continue
		}
		normalized := normalizeCredentialHashForPAT(storedHash)
		if normalized == emptyValue {
			continue
		}
		// Constant-time compare to avoid timing leaks
		if subtle.ConstantTimeCompare([]byte(normalized), []byte(patHashHex)) != 1 {
			continue
		}

		// Check revoked/expired (same as validateKeystoreKey)
		if revoked, ok := entry[objects.FieldKeyRevoked].(bool); ok && revoked {
			return "", nil, nil, errfmt.Errorf("personal access token has been revoked")
		}
		if expiresAt, ok := entry[objects.FieldKeyExpiresAt].(string); ok && expiresAt != emptyValue {
			expiresTime, parseErr := time.Parse(time.RFC3339, expiresAt)
			if parseErr == nil && time.Now().UTC().After(expiresTime) {
				return "", nil, nil, errfmt.Errorf("personal access token has expired")
			}
		}

		accountID, ok = entry[clientInfoAccountID].(string)
		if !ok || accountID == emptyValue {
			return "", nil, nil, errfmt.Errorf("personal access token has no associated account")
		}

		roles, permissions, err = s.loadAccountRolesAndPermissions(accountID, projectRoot)
		if err != nil {
			return "", nil, nil, err
		}
		return accountID, roles, permissions, nil
	}

	return "", nil, nil, errfmt.Errorf("personal access token authentication not yet implemented")
}

// loadAccountObject loads an account object by ACC-* id (CAS via .account.index).
// Legacy account:username / account-username.yaml paths are retired.
// TRACK: follow-up in kernel backlog
func (s *Server) loadAccountObject(accountID, projectRoot string) (map[string]any, error) {
	accountID = strings.TrimSpace(accountID)
	if canon := authcred.CanonicalAccountID(projectRoot, accountID); canon != "" {
		accountID = canon
	}
	data, ok := authcred.AccountYAML(projectRoot, accountID)
	if !ok {
		return nil, errfmt.Errorf("account not found: %s", accountID)
	}

	var account map[string]any
	if err := yaml.Unmarshal(data, &account); err != nil {
		return nil, errfmt.Newf("failed to parse account file").Wrap(err)
	}

	return account, nil
}

// loadAccountByEmail loads an account object by email
func (s *Server) loadAccountByEmail(email, projectRoot string) (map[string]any, error) {
	var found map[string]any
	authcred.WalkAccountYAML(projectRoot, func(_ string, data []byte) bool {
		var account map[string]any
		if err := yaml.Unmarshal(data, &account); err != nil {
			return true
		}
		if accountEmail, ok := account[objects.FieldKeyEmail].(string); ok && accountEmail == email {
			found = account
			return false
		}
		return true
	})
	if found == nil {
		return nil, errfmt.Errorf("account not found for email: %s", email)
	}
	return found, nil
}

// extractRolesFromAccount is a convenience wrapper around ExtractRolesFromAccount.
// Kept for backward compatibility with existing code.
func extractRolesFromAccount(account map[string]any) []string {
	return ExtractRolesFromAccount(account)
}

// extractPermissionsFromAccount is a convenience wrapper around ExtractPermissionsFromAccount.
// Kept for backward compatibility with existing code.
func extractPermissionsFromAccount(account map[string]any) []string {
	return ExtractPermissionsFromAccount(account)
}

func extractRolesAndPermissions(account map[string]any) ([]string, []string) {
	return extractRolesFromAccount(account), extractPermissionsFromAccount(account)
}

func (s *Server) loadAccountRolesAndPermissions(accountID, projectRoot string) ([]string, []string, error) {
	accountObj, err := s.loadAccountObject(accountID, projectRoot)
	if err != nil {
		return nil, nil, errfmt.Newf("failed to load account").Wrap(err)
	}
	roles, permissions := extractRolesAndPermissions(accountObj)
	return roles, permissions, nil
}

// loadEnabledAuthStrategies loads enabled authentication strategies from auth_strategy objects
// Returns a map of strategy_type -> enabled (true/false)
func (s *Server) loadEnabledAuthStrategies(_ context.Context) (map[string]bool, error) {
	projectRoot := s.GetProjectRoot()
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root not available")
	}

	strategies := make(map[string]bool)
	configStrategyIDs := make(map[string]bool)
	if s.config != nil && len(s.config.MCPServer.Security.AuthStrategies) > 0 {
		for _, id := range s.config.MCPServer.Security.AuthStrategies {
			configStrategyIDs[id] = true
		}
	}

	for _, strategy := range authcred.ListAuthStrategyRecords(projectRoot) {
		if !s.isStrategyStatusActive(strategy.Status) {
			continue
		}
		if len(configStrategyIDs) > 0 && !configStrategyIDs[strategy.ID] {
			continue
		}
		if strategy.Enabled {
			strategies[strategy.Type] = true
		}
	}

	return strategies, nil
}

// isStrategyStatusActive checks if a strategy status is active based on lifecycle definition.
// Returns true if status is not terminal, not archived, and not system (error).
// If the auth_strategy lifecycle cannot be loaded, returns false (no fallback to status == "active").
func (s *Server) isStrategyStatusActive(status string) bool {
	loader := objects.GetGlobalLifecycleLoader()
	if loader == nil {
		if s.getTraceWriter() != nil {
			logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
			logging.Fluent(logger).Debug("auth_strategy lifecycle loader not available; treating strategy status as inactive").
				EmitComponent("mcp_server").
				String("status", status).
				Log()
		}
		return false
	}
	lifecycle, err := loader.LoadLifecycle(objects.KindAuthStrategy)
	if err != nil || lifecycle == nil {
		if s.getTraceWriter() != nil {
			logger := logging.GetLoggerFromProfile(DefaultLoggingProfile)
			logging.Fluent(logger).Debug("auth_strategy lifecycle load failed; treating strategy status as inactive").
				EmitComponent("mcp_server").
				String("status", status).
				WithError(err).
				Log()
		}
		return false
	}
	// Find the status in lifecycle definition
	for _, st := range lifecycle.Statuses {
		if st.Value == status {
			return !st.Terminal && !st.Archive && !st.System
		}
	}
	// Status not found in lifecycle - consider inactive
	return false
}
