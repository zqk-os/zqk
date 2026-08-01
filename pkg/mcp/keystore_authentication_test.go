package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/testkit"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// setupKeystoreAuthTest prepares an isolated project tree via [testkit.PrepareIsolatedTempProject].
// Callers must not use t.Parallel(): ZQK_TEST_ROOT uses t.Setenv.
func setupKeystoreAuthTest(t *testing.T) (string, *Server) {
	t.Helper()
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "pkg.mcp.keystore_auth",
		SkipSetupTestEnvironment: true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "keystore_layout",
				Fn: func() error {
					return paths.LayoutUnder(root).
						Dir(paths.ProcessDir, paths.DirPerm755).
						Dir(paths.ProcessKeystoreDir, paths.DirPerm700).
						Dir(paths.ProcessAccountsDir, paths.DirPerm755).
						Dir(paths.ProcessAuthStrategiesDir, paths.DirPerm755).
						Err()
				},
			}}
		},
	})
	tmpDir := proj.Root

	server := NewServer()
	initCtx := pkgctx.NewCliInitializationContext(func(string) string { return tmpDir }, tmpDir)
	server.SetCliInitializationContext(initCtx)

	return tmpDir, server
}

func TestValidateKeystoreKey_ValidKey(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	// Create account
	account := map[string]any{
		objects.FieldKeyID:            "account:developer",
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "developer",
		objects.FieldKeyRoles:         []string{"developer"},
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	accountFile := filepath.Join(tmpDir, paths.ProcessAccountsDir, "account-developer.yaml")
	// Write account file directly
	data, _ := yaml.Marshal(account)
	_ = os.WriteFile(accountFile, data, paths.FilePerm644) //nolint:errcheck // Test setup

	// Create keystore entry
	keystoreEntry := map[string]any{
		objects.FieldKeyID:             "KEY-001",
		objects.FieldKeyKind:           "keystore_entry",
		clientInfoAccountID:            "account:developer",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hash123",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         "active",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  "zqk",
		objects.FieldKeyOriginSystem:   "zqk",
	}
	keystoreFile := filepath.Join(tmpDir, paths.ProcessKeystoreDir, "KEY-001.yaml")
	keystoreData, _ := yaml.Marshal(keystoreEntry)
	_ = os.WriteFile(keystoreFile, keystoreData, paths.FilePerm600) //nolint:errcheck // Test setup

	// Validate keystore key
	accountID, roles, permissions, err := server.validateKeystoreKey(ctx, "KEY-001", tmpDir)
	if err != nil {
		t.Fatalf("Failed to validate keystore key: %v", err)
	}

	if accountID != "account:developer" {
		t.Errorf("Expected account_id 'account:developer', got '%s'", accountID)
	}
	if len(roles) != 1 || roles[0] != "developer" {
		t.Errorf("Expected roles ['developer'], got %v", roles)
	}
	_ = permissions // Suppress unused variable warning
}

// TestValidateUsernamePassword_ValidPassword checks that username/password succeeds when
// account has a password keystore entry and bcrypt hash matches.
func TestValidateUsernamePassword_ValidPassword(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	password := "testpassword"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}

	account := map[string]any{
		objects.FieldKeyID:            "account:developer",
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "developer",
		objects.FieldKeyRoles:         []string{"developer"},
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	accountFile := filepath.Join(tmpDir, paths.ProcessAccountsDir, "account-developer.yaml")
	accountData, _ := yaml.Marshal(account)
	_ = os.WriteFile(accountFile, accountData, paths.FilePerm644) //nolint:errcheck // Test setup

	keystoreEntry := map[string]any{
		objects.FieldKeyID:             "KEY-PW",
		objects.FieldKeyKind:           "keystore_entry",
		clientInfoAccountID:            "account:developer",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: string(hash),
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         "active",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  "zqk",
		objects.FieldKeyOriginSystem:   "zqk",
	}
	keystoreFile := filepath.Join(tmpDir, paths.ProcessKeystoreDir, "KEY-PW.yaml")
	keystoreData, _ := yaml.Marshal(keystoreEntry)
	_ = os.WriteFile(keystoreFile, keystoreData, paths.FilePerm600) //nolint:errcheck // Test setup

	accountID, roles, permissions, err := server.validateUsernamePassword(ctx, "developer", password, tmpDir)
	if err != nil {
		t.Fatalf("validateUsernamePassword: %v", err)
	}
	if accountID != "account:developer" {
		t.Errorf("accountID: got %s", accountID)
	}
	if len(roles) != 1 || roles[0] != "developer" {
		t.Errorf("roles: got %v", roles)
	}
	_ = permissions
}

// TestValidateUsernamePassword_WrongPassword checks that wrong password returns "invalid username or password".
func TestValidateUsernamePassword_WrongPassword(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	hash, err := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}

	account := map[string]any{
		objects.FieldKeyID:            "account:developer",
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "developer",
		objects.FieldKeyRoles:         []string{"developer"},
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	accountFile := filepath.Join(tmpDir, paths.ProcessAccountsDir, "account-developer.yaml")
	accountData, _ := yaml.Marshal(account)
	_ = os.WriteFile(accountFile, accountData, paths.FilePerm644) //nolint:errcheck // Test setup

	keystoreEntry := map[string]any{
		objects.FieldKeyID:             "KEY-PW",
		objects.FieldKeyKind:           "keystore_entry",
		clientInfoAccountID:            "account:developer",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: string(hash),
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         "active",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  "zqk",
		objects.FieldKeyOriginSystem:   "zqk",
	}
	keystoreFile := filepath.Join(tmpDir, paths.ProcessKeystoreDir, "KEY-PW.yaml")
	keystoreData, _ := yaml.Marshal(keystoreEntry)
	_ = os.WriteFile(keystoreFile, keystoreData, paths.FilePerm600) //nolint:errcheck // Test setup

	_, _, _, err = server.validateUsernamePassword(ctx, "developer", "wrongpassword", tmpDir)
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
	if err.Error() != "invalid username or password" {
		t.Errorf("error: got %q", err.Error())
	}
}

// TestValidateUsernamePassword_NoPasswordKeystoreEntry checks that when no password
// keystore entry exists for the account, validation returns "invalid username or password".
func TestValidateUsernamePassword_NoPasswordKeystoreEntry(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	account := map[string]any{
		objects.FieldKeyID:            "account:developer",
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "developer",
		objects.FieldKeyRoles:         []string{"developer"},
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	accountFile := filepath.Join(tmpDir, paths.ProcessAccountsDir, "account-developer.yaml")
	accountData, _ := yaml.Marshal(account)
	_ = os.WriteFile(accountFile, accountData, paths.FilePerm644) //nolint:errcheck // Test setup
	// No keystore entry with key_type password for this account

	_, _, _, err := server.validateUsernamePassword(ctx, "developer", "anypassword", tmpDir)
	if err == nil {
		t.Fatal("expected error when no password keystore entry")
	}
	if err.Error() != "invalid username or password" {
		t.Errorf("error: got %q", err.Error())
	}
}

// TestValidateUsernamePassword_RevokedPasswordEntry checks that a revoked password
// keystore entry yields "invalid username or password".
func TestValidateUsernamePassword_RevokedPasswordEntry(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	password := "testpassword"
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt.GenerateFromPassword: %v", err)
	}

	account := map[string]any{
		objects.FieldKeyID:            "account:developer",
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "developer",
		objects.FieldKeyRoles:         []string{"developer"},
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	accountFile := filepath.Join(tmpDir, paths.ProcessAccountsDir, "account-developer.yaml")
	accountData, _ := yaml.Marshal(account)
	_ = os.WriteFile(accountFile, accountData, paths.FilePerm644) //nolint:errcheck // Test setup

	keystoreEntry := map[string]any{
		objects.FieldKeyID:             "KEY-PW",
		objects.FieldKeyKind:           "keystore_entry",
		clientInfoAccountID:            "account:developer",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: string(hash),
		objects.FieldKeyRevoked:        true,
		objects.FieldKeyRevokedAt:      "2026-01-01T00:00:00Z",
		objects.FieldKeyStatus:         "active",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  "zqk",
		objects.FieldKeyOriginSystem:   "zqk",
	}
	keystoreFile := filepath.Join(tmpDir, paths.ProcessKeystoreDir, "KEY-PW.yaml")
	keystoreData, _ := yaml.Marshal(keystoreEntry)
	_ = os.WriteFile(keystoreFile, keystoreData, paths.FilePerm600) //nolint:errcheck // Test setup

	_, _, _, err = server.validateUsernamePassword(ctx, "developer", password, tmpDir)
	if err == nil {
		t.Fatal("expected error for revoked password entry")
	}
	if err.Error() != "invalid username or password" {
		t.Errorf("error: got %q", err.Error())
	}
}

func TestValidateKeystoreKey_RevokedKey(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	// Create keystore entry that is revoked
	keystoreEntry := map[string]any{
		objects.FieldKeyID:             "KEY-002",
		objects.FieldKeyKind:           "keystore_entry",
		clientInfoAccountID:            "account:developer",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hash456",
		objects.FieldKeyRevoked:        true,
		objects.FieldKeyRevokedAt:      "2026-01-01T00:00:00Z",
		objects.FieldKeyStatus:         "active",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  "zqk",
		objects.FieldKeyOriginSystem:   "zqk",
	}
	keystoreFile := filepath.Join(tmpDir, paths.ProcessKeystoreDir, "KEY-002.yaml")
	data, _ := yaml.Marshal(keystoreEntry)
	_ = os.WriteFile(keystoreFile, data, paths.FilePerm600) //nolint:errcheck // Test setup

	// Should fail with revoked error
	_, _, _, err := server.validateKeystoreKey(ctx, "KEY-002", tmpDir)
	if err == nil {
		t.Errorf("Expected error for revoked key, but got none")
	}
	if err != nil && err.Error() != "keystore key has been revoked" {
		t.Errorf("Expected 'keystore key has been revoked' error, got: %v", err)
	}
}

func TestValidateKeystoreKey_ExpiredKey(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	// Create keystore entry that is expired
	keystoreEntry := map[string]any{
		objects.FieldKeyID:             "KEY-003",
		objects.FieldKeyKind:           "keystore_entry",
		clientInfoAccountID:            "account:developer",
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "hash789",
		objects.FieldKeyExpiresAt:      "2020-01-01T00:00:00Z", // Expired
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         "active",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  "zqk",
		objects.FieldKeyOriginSystem:   "zqk",
	}
	keystoreFile := filepath.Join(tmpDir, paths.ProcessKeystoreDir, "KEY-003.yaml")
	data, _ := yaml.Marshal(keystoreEntry)
	_ = os.WriteFile(keystoreFile, data, paths.FilePerm600) //nolint:errcheck // Test setup

	// Should fail with expired error
	_, _, _, err := server.validateKeystoreKey(ctx, "KEY-003", tmpDir)
	if err == nil {
		t.Errorf("Expected error for expired key, but got none")
	}
	if err != nil && err.Error() != "keystore key has expired" {
		t.Errorf("Expected 'keystore key has expired' error, got: %v", err)
	}
}

func TestLoadEnabledAuthStrategies(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	// Create auth strategy objects
	strategies := []map[string]any{
		{
			objects.FieldKeyID:            "AUTH-001",
			objects.FieldKeyKind:          "auth_strategy",
			objects.FieldKeyStrategyType:  "keystore",
			objects.FieldKeyEnabled:       true,
			objects.FieldKeyStatus:        "active",
			objects.FieldKeyPriority:      1,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyOriginProject: "zqk",
			objects.FieldKeyOriginSystem:  "zqk",
		},
		{
			objects.FieldKeyID:            "AUTH-002",
			objects.FieldKeyKind:          "auth_strategy",
			objects.FieldKeyStrategyType:  "username_password",
			objects.FieldKeyEnabled:       true,
			objects.FieldKeyStatus:        "active",
			objects.FieldKeyPriority:      2,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyOriginProject: "zqk",
			objects.FieldKeyOriginSystem:  "zqk",
		},
		{
			objects.FieldKeyID:            "AUTH-003",
			objects.FieldKeyKind:          "auth_strategy",
			objects.FieldKeyStrategyType:  "oauth",
			objects.FieldKeyEnabled:       false, // Disabled
			objects.FieldKeyStatus:        "active",
			objects.FieldKeyPriority:      3,
			objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
			objects.FieldKeyOriginProject: "zqk",
			objects.FieldKeyOriginSystem:  "zqk",
		},
	}

	authStrategiesDir := filepath.Join(tmpDir, paths.ProcessAuthStrategiesDir)
	for _, strategy := range strategies {
		strategyFile := filepath.Join(authStrategiesDir, strategy[objects.FieldKeyID].(string)+".yaml")
		data, _ := yaml.Marshal(strategy)
		_ = os.WriteFile(strategyFile, data, paths.FilePerm644) //nolint:errcheck // Test setup
	}

	// Load enabled strategies
	enabled, err := server.loadEnabledAuthStrategies(ctx)
	if err != nil {
		t.Fatalf("Failed to load auth strategies: %v", err)
	}

	// Should have keystore and username_password enabled, but not oauth
	if !enabled["keystore"] {
		t.Errorf("Expected keystore to be enabled")
	}
	if !enabled["username_password"] {
		t.Errorf("Expected username_password to be enabled")
	}
	if enabled["oauth"] {
		t.Errorf("Expected oauth to be disabled")
	}
}

func TestHasCredentials(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		clientInfo map[string]any
		expected   bool
	}{
		{
			name:       "keystore_key_id",
			clientInfo: map[string]any{"keystore_key_id": "KEY-001"},
			expected:   true,
		},
		{
			name:       "username_password",
			clientInfo: map[string]any{objects.FieldKeyUsername: "developer", "password": "pass123"},
			expected:   true,
		},
		{
			name:       "oauth_token",
			clientInfo: map[string]any{"oauth_token": "token123"},
			expected:   true,
		},
		{
			name:       "personal_access_token",
			clientInfo: map[string]any{"personal_access_token": "pat123"},
			expected:   true,
		},
		{
			name:       "no_credentials",
			clientInfo: map[string]any{},
			expected:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasCredentials(tt.clientInfo)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func patHashHex(pat string) string {
	h := sha256.Sum256([]byte(pat))
	return hex.EncodeToString(h[:])
}

func TestValidatePersonalAccessToken_ValidPAT(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	pat := "my-secret-pat-token"
	credHash := "sha256:" + patHashHex(pat)

	account := map[string]any{
		objects.FieldKeyID:            "account:automation",
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "automation",
		objects.FieldKeyRoles:         []string{"automation"},
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	accountFile := filepath.Join(tmpDir, paths.ProcessAccountsDir, "account-automation.yaml")
	accountData, _ := yaml.Marshal(account)
	_ = os.WriteFile(accountFile, accountData, paths.FilePerm644) //nolint:errcheck // Test setup

	keystoreEntry := map[string]any{
		objects.FieldKeyID:             "KEY-PAT",
		objects.FieldKeyKind:           "keystore_entry",
		clientInfoAccountID:            "account:automation",
		objects.FieldKeyKeyType:        "personal_access_token",
		objects.FieldKeyCredentialHash: credHash,
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         "active",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  "zqk",
		objects.FieldKeyOriginSystem:   "zqk",
	}
	keystoreFile := filepath.Join(tmpDir, paths.ProcessKeystoreDir, "KEY-PAT.yaml")
	keystoreData, _ := yaml.Marshal(keystoreEntry)
	_ = os.WriteFile(keystoreFile, keystoreData, paths.FilePerm600) //nolint:errcheck // Test setup

	accountID, roles, permissions, err := server.validatePersonalAccessToken(ctx, pat, tmpDir)
	if err != nil {
		t.Fatalf("validatePersonalAccessToken: %v", err)
	}
	if accountID != "account:automation" {
		t.Errorf("accountID: got %s", accountID)
	}
	if len(roles) != 1 || roles[0] != "automation" {
		t.Errorf("roles: got %v", roles)
	}
	_ = permissions
}

func TestValidatePersonalAccessToken_InvalidPAT(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	pat := "valid-pat"
	credHash := "sha256:" + patHashHex(pat)

	account := map[string]any{
		objects.FieldKeyID:            "account:automation",
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "automation",
		objects.FieldKeyRoles:         []string{"automation"},
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	accountFile := filepath.Join(tmpDir, paths.ProcessAccountsDir, "account-automation.yaml")
	accountData, _ := yaml.Marshal(account)
	_ = os.WriteFile(accountFile, accountData, paths.FilePerm644) //nolint:errcheck // Test setup

	keystoreEntry := map[string]any{
		objects.FieldKeyID:             "KEY-PAT",
		objects.FieldKeyKind:           "keystore_entry",
		clientInfoAccountID:            "account:automation",
		objects.FieldKeyKeyType:        "personal_access_token",
		objects.FieldKeyCredentialHash: credHash,
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         "active",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  "zqk",
		objects.FieldKeyOriginSystem:   "zqk",
	}
	keystoreFile := filepath.Join(tmpDir, paths.ProcessKeystoreDir, "KEY-PAT.yaml")
	keystoreData, _ := yaml.Marshal(keystoreEntry)
	_ = os.WriteFile(keystoreFile, keystoreData, paths.FilePerm600) //nolint:errcheck // Test setup

	_, _, _, err := server.validatePersonalAccessToken(ctx, "wrong-pat", tmpDir)
	if err == nil {
		t.Fatal("expected error for invalid PAT")
	}
	if err.Error() != "personal access token authentication not yet implemented" {
		t.Errorf("error: got %q", err.Error())
	}
}

func TestValidatePersonalAccessToken_RevokedPAT(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	pat := "my-pat"
	credHash := "sha256:" + patHashHex(pat)

	account := map[string]any{
		objects.FieldKeyID:            "account:automation",
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "automation",
		objects.FieldKeyRoles:         []string{"automation"},
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	accountFile := filepath.Join(tmpDir, paths.ProcessAccountsDir, "account-automation.yaml")
	accountData, _ := yaml.Marshal(account)
	_ = os.WriteFile(accountFile, accountData, paths.FilePerm644) //nolint:errcheck // Test setup

	keystoreEntry := map[string]any{
		objects.FieldKeyID:             "KEY-PAT",
		objects.FieldKeyKind:           "keystore_entry",
		clientInfoAccountID:            "account:automation",
		objects.FieldKeyKeyType:        "personal_access_token",
		objects.FieldKeyCredentialHash: credHash,
		objects.FieldKeyRevoked:        true,
		objects.FieldKeyRevokedAt:      "2026-01-01T00:00:00Z",
		objects.FieldKeyStatus:         "active",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  "zqk",
		objects.FieldKeyOriginSystem:   "zqk",
	}
	keystoreFile := filepath.Join(tmpDir, paths.ProcessKeystoreDir, "KEY-PAT.yaml")
	keystoreData, _ := yaml.Marshal(keystoreEntry)
	_ = os.WriteFile(keystoreFile, keystoreData, paths.FilePerm600) //nolint:errcheck // Test setup

	_, _, _, err := server.validatePersonalAccessToken(ctx, pat, tmpDir)
	if err == nil {
		t.Fatal("expected error for revoked PAT")
	}
	if err.Error() != "personal access token has been revoked" {
		t.Errorf("error: got %q", err.Error())
	}
}

func TestValidatePersonalAccessToken_ExpiredPAT(t *testing.T) {
	tmpDir, server := setupKeystoreAuthTest(t)
	ctx := pkgctx.NewSystemContext()

	pat := "my-pat"
	credHash := "sha256:" + patHashHex(pat)

	account := map[string]any{
		objects.FieldKeyID:            "account:automation",
		objects.FieldKeyKind:          "account",
		objects.FieldKeyUsername:      "automation",
		objects.FieldKeyRoles:         []string{"automation"},
		objects.FieldKeyStatus:        "active",
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: "zqk",
		objects.FieldKeyOriginSystem:  "zqk",
	}
	accountFile := filepath.Join(tmpDir, paths.ProcessAccountsDir, "account-automation.yaml")
	accountData, _ := yaml.Marshal(account)
	_ = os.WriteFile(accountFile, accountData, paths.FilePerm644) //nolint:errcheck // Test setup

	keystoreEntry := map[string]any{
		objects.FieldKeyID:             "KEY-PAT",
		objects.FieldKeyKind:           "keystore_entry",
		clientInfoAccountID:            "account:automation",
		objects.FieldKeyKeyType:        "personal_access_token",
		objects.FieldKeyCredentialHash: credHash,
		objects.FieldKeyExpiresAt:      "2020-01-01T00:00:00Z",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         "active",
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  "zqk",
		objects.FieldKeyOriginSystem:   "zqk",
	}
	keystoreFile := filepath.Join(tmpDir, paths.ProcessKeystoreDir, "KEY-PAT.yaml")
	keystoreData, _ := yaml.Marshal(keystoreEntry)
	_ = os.WriteFile(keystoreFile, keystoreData, paths.FilePerm600) //nolint:errcheck // Test setup

	_, _, _, err := server.validatePersonalAccessToken(ctx, pat, tmpDir)
	if err == nil {
		t.Fatal("expected error for expired PAT")
	}
	if err.Error() != "personal access token has expired" {
		t.Errorf("error: got %q", err.Error())
	}
}
