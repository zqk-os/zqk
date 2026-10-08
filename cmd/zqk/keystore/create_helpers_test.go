package keystore

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
)

func TestValidateKeyType(t *testing.T) {
	valid := []string{"password", "api_key", "oauth_token", "personal_access_token"}
	for _, k := range valid {
		assert.NoError(t, validateKeyType(k))
	}

	invalid := []string{"", "invalid_type", "symmetric_key", "jwt"}
	for _, k := range invalid {
		assert.Error(t, validateKeyType(k))
	}
}

func TestDetermineAccountID(t *testing.T) {
	// 1. Empty account ID with system account in secCtx -> error
	sysCtx := pkgctx.NewSystemSecurityContext()
	flags := &CreateFlags{AccountID: ""}
	_, err := determineAccountID(flags, sysCtx)
	assert.ErrorContains(t, err, "cannot use system account")

	// 2. Empty account ID with user account in secCtx -> returns user account
	userCtx := &pkgctx.SecurityContext{AccountID: "ACC-USER-123"}
	accID, err := determineAccountID(flags, userCtx)
	require.NoError(t, err)
	assert.Equal(t, "ACC-USER-123", accID)

	// 3. Explicit account ID matching user account -> returns user account
	flagsExplicit := &CreateFlags{AccountID: "ACC-USER-123"}
	accID2, err := determineAccountID(flagsExplicit, userCtx)
	require.NoError(t, err)
	assert.Equal(t, "ACC-USER-123", accID2)

	// 4. Non-admin trying other account -> error
	flagsOther := &CreateFlags{AccountID: "ACC-OTHER-456"}
	_, err = determineAccountID(flagsOther, userCtx)
	assert.ErrorContains(t, err, "permission denied")

	// 5. Admin trying other account -> success
	adminCtx := &pkgctx.SecurityContext{AccountID: "ACC-ADMIN-123", Roles: []string{"admin"}}
	accID3, err := determineAccountID(flagsOther, adminCtx)
	require.NoError(t, err)
	assert.Equal(t, "ACC-OTHER-456", accID3)
}

func TestHashFunctions(t *testing.T) {
	// hashUserKey (bcrypt)
	hash, err := hashUserKey("secret-password")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)

	// hashToken (SHA-256)
	tokenHash := hashToken("api-token-value")
	assert.NotEmpty(t, tokenHash)
	assert.Contains(t, tokenHash, "sha256:")
}

func TestBuildKeystoreEntryAndResults(t *testing.T) {
	flags := &CreateFlags{
		AccountID:   "ACC-TEST-001",
		KeyType:     "api_key",
		Title:       "Test Key Title",
		Description: "Detailed description of key",
		ExpiresAt:   "2026-12-31T23:59:59Z",
	}

	entry := buildKeystoreEntry(flags, "ACC-TEST-001", "sha256:fakehash", "")
	assert.Equal(t, objects.KindKeystoreEntry, entry[objects.FieldKeyKind])
	assert.Equal(t, "Test Key Title", entry[objects.FieldKeyTitle])
	assert.Equal(t, "ACC-TEST-001", entry[objects.FieldKeyAccountID])
	assert.Equal(t, "api_key", entry[objects.FieldKeyKeyType])
	assert.Equal(t, "Detailed description of key", entry[objects.FieldKeyDescription])
	assert.Equal(t, "2026-12-31T23:59:59Z", entry[objects.FieldKeyExpiresAt])

	// buildCreateResult
	res := buildCreateResult("KEY-123", "ACC-TEST-001", "api_key", "Test Key Title", "2026-12-31T23:59:59Z")
	assert.Equal(t, "KEY-123", res[objects.FieldKeyID])
	assert.Equal(t, "2026-12-31T23:59:59Z", res[objects.FieldKeyExpiresAt])

	// formatCreateOutputText
	txt := formatCreateOutputText(flags, "KEY-123", "ACC-TEST-001")
	assert.Contains(t, string(txt), "KEY-123")
	assert.Contains(t, string(txt), "ACC-TEST-001")
	assert.Contains(t, string(txt), "api_key")
}

func TestParseCreateFlags(t *testing.T) {
	cmd := NewCreateCmd()
	_ = cmd.Flags().Set("account-id", "ACC-TEST-001")
	_ = cmd.Flags().Set("key-type", "personal_access_token")
	_ = cmd.Flags().Set("credential", "secret-token")
	_ = cmd.Flags().Set("title", "My Token")
	_ = cmd.Flags().Set("description", "Token description")
	_ = cmd.Flags().Set("expires-at", "2027-01-01T00:00:00Z")

	flags := parseCreateFlags(cmd)
	assert.Equal(t, "ACC-TEST-001", flags.AccountID)
	assert.Equal(t, "personal_access_token", flags.KeyType)
	assert.Equal(t, "secret-token", flags.KeyData)
	assert.Equal(t, "My Token", flags.Title)
	assert.Equal(t, "Token description", flags.Description)
	assert.Equal(t, "2027-01-01T00:00:00Z", flags.ExpiresAt)
}

func TestCreateCmd_Execute(t *testing.T) {
	tmpDir, _, _ := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	ctx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())

	cmd := NewCreateCmd()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--title", "My CLI API Key", "--credential", "supersecret123", "--key-type", "api_key", "--account-id", "ACC-1785920548450214003-23d25bd5"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	// JSON format
	cmdJSON := NewCreateCmd()
	cmdJSON.SetContext(ctx)
	cmdJSON.SetArgs([]string{"--title", "My JSON Key", "--credential", "supersecret456", "--key-type", "api_key", "--account-id", "ACC-1785920548450214003-23d25bd5", "--format", "json"})
	bufJSON := new(bytes.Buffer)
	cmdJSON.SetOut(bufJSON)
	cmdJSON.SetErr(bufJSON)

	err = cmdJSON.Execute()
	require.NoError(t, err)
}
