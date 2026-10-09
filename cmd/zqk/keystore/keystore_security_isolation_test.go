package keystore

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/zqk-os/zqk/pkg/authcred"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func executeKeystoreCmd(cmd *cobra.Command, ctx context.Context, args []string) (string, error) {
	buf := new(bytes.Buffer)
	ctxWithWriter := pkgctx.WithCommandOutputWriter(ctx, buf)
	cmd.SetContext(ctxWithWriter)
	cmd.SetArgs(args)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.ExecuteContext(ctxWithWriter)
	return buf.String(), err
}

func TestKeystoreSecurity_KeyGenerationAndCustodyEdgeCases(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	accID := "ACC-1785920548450214003-23d25bd5"
	account := map[string]any{
		objects.FieldKeyKind:          objects.KindAccount,
		objects.FieldKeyID:            accID,
		objects.FieldKeyTitle:         "Test Swarm Worker Account",
		objects.FieldKeyUsername:      "swarm_worker",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, account))

	cmdCtx := pkgctx.WithSecurityContext(context.Background(), systemCtx)

	// 1. Issue with seating file generation (default: writes .zqk/seating/credentials/<accID>)
	cmd := NewIssueCmd()
	out, err := executeKeystoreCmd(cmd, cmdCtx, []string{
		"--account-id", accID,
		"--title", "Agent Seat 1",
		"--description", "Seated worker credential",
		"--expires-at", "2027-12-31T23:59:59Z",
	})
	require.NoError(t, err)
	assert.Contains(t, out, "API key issued")
	assert.Contains(t, out, accID)
	assert.Contains(t, out, "Seating file:")

	// Verify seating file existence and permissions (0600)
	seatingPath := authcred.SeatKeyPath(tmpDir, accID)
	info, err := fileutil.Stat(seatingPath)
	require.NoError(t, err)
	assert.Equal(t, fileutil.FileMode(0600), info.Mode().Perm())

	// Verify the seating file content matches LoadSeatCredential and APIKeyForSeat
	seatKey, err := authcred.LoadSeatCredential(tmpDir, accID)
	require.NoError(t, err)
	assert.NotEmpty(t, seatKey)
	assert.True(t, strings.HasPrefix(seatKey, authcred.AgentPrefix))
	assert.Equal(t, seatKey, authcred.APIKeyForSeat(tmpDir, accID))

	// 2. Issue with --no-seating-file
	accID2 := "ACC-1785920548450214003-99999999"
	cmdNoSeat := NewIssueCmd()
	_, err = executeKeystoreCmd(cmdNoSeat, cmdCtx, []string{
		"--account-id", accID2,
		"--title", "Unseated Agent",
		"--no-seating-file",
	})
	require.NoError(t, err)
	noSeatPath := authcred.SeatKeyPath(tmpDir, accID2)
	_, err = fileutil.Stat(noSeatPath)
	assert.True(t, fileutil.IsNotExist(err))

	// 3. Issue with JSON and YAML formats
	cmdJSON := NewIssueCmd()
	outJSON, err := executeKeystoreCmd(cmdJSON, cmdCtx, []string{
		"--account-id", accID,
		"--title", "JSON Seat Key",
		"--no-seating-file",
		"--format", "json",
	})
	require.NoError(t, err)
	var jsonMap map[string]any
	require.NoError(t, json.Unmarshal([]byte(outJSON), &jsonMap))
	assert.NotEmpty(t, jsonMap["api_key"])
	assert.NotEmpty(t, jsonMap["fingerprint"])
	assert.Contains(t, jsonMap["env_hint"], "ZQK_API_KEY=")

	// 4. Invalid account ID format (must be ACC-*)
	cmdInvalidAcc := NewIssueCmd()
	_, err = executeKeystoreCmd(cmdInvalidAcc, cmdCtx, []string{
		"--account-id", "USER-12345",
		"--title", "Bad Account",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "account-id must be ACC-* form")

	// 5. Unauthorized issue attempt (regular user attempting to issue for another account)
	userCtx := pkgctx.NewSecurityContext("ACC-OTHER-USER", []string{"developer"}, []string{"read:*"})
	cmdUnauthorized := NewIssueCmd()
	_, err = executeKeystoreCmd(cmdUnauthorized, pkgctx.WithSecurityContext(context.Background(), userCtx), []string{
		"--account-id", accID,
		"--title", "Unauthorized Issue",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")

	// 6. Graceful handling when account object is missing in storage during issue
	cmdMissingAcc := NewIssueCmd()
	outMissing, err := executeKeystoreCmd(cmdMissingAcc, cmdCtx, []string{
		"--account-id", "ACC-1785920548450214003-NONEXISTENT",
		"--title", "Orphan Key",
		"--no-seating-file",
	})
	require.NoError(t, err)
	assert.Contains(t, outMissing, "API key issued")
}

func TestKeystoreSecurity_RotationErrorHandling(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	accID := "ACC-1785920548450214003-23d25bd5"
	ownerCtx := pkgctx.NewSecurityContext(accID, []string{"developer"}, []string{"read:*", "write:*"})
	adminCtx := pkgctx.NewSecurityContext("ACC-ADMIN-1", []string{"admin"}, []string{"read:*", "write:*"})

	// Create an active key
	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Key For Rotation Security Tests",
		objects.FieldKeyAccountID:      accID,
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:initialhash",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, entry))
	keyID := entry[objects.FieldKeyID].(string)
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, keyID)

	// 1. Missing / Nonexistent key ID
	cmdNotFound := NewRotateCmd()
	_, err := executeKeystoreCmd(cmdNotFound, pkgctx.WithSecurityContext(context.Background(), systemCtx), []string{
		"KEY-NONEXISTENT-999",
		"--revoke",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read keystore entry")

	// 2. Target object is not a keystore_entry (e.g. Account)
	accObj := map[string]any{
		objects.FieldKeyKind:          objects.KindAccount,
		objects.FieldKeyID:            "ACC-NOT-A-KEY",
		objects.FieldKeyTitle:         "Account Object",
		objects.FieldKeyUsername:      "notakey",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, accObj))

	cmdWrongKind := NewRotateCmd()
	_, err = executeKeystoreCmd(cmdWrongKind, pkgctx.WithSecurityContext(context.Background(), systemCtx), []string{
		"ACC-NOT-A-KEY",
		"--revoke",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is not a keystore_entry")

	// 3. No action flags provided
	cmdNoFlags := NewRotateCmd()
	_, err = executeKeystoreCmd(cmdNoFlags, pkgctx.WithSecurityContext(context.Background(), systemCtx), []string{
		keyID,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "must specify either --new-credential, --revoke, or --revoke-old")

	// 4. Non-system user attempting credential rotation
	cmdUserRot := NewRotateCmd()
	_, err = executeKeystoreCmd(cmdUserRot, pkgctx.WithSecurityContext(context.Background(), ownerCtx), []string{
		keyID,
		"--new-credential", "new-unauth-secret",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "only system can rotate keys")

	// 5. Unauthorized third-party user attempting revocation
	otherUserCtx := pkgctx.NewSecurityContext("ACC-OTHER-USER", []string{"developer"}, []string{"read:*"})
	cmdUnauthRev := NewRotateCmd()
	_, err = executeKeystoreCmd(cmdUnauthRev, pkgctx.WithSecurityContext(context.Background(), otherUserCtx), []string{
		keyID,
		"--revoke",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied: only owner, admin, or system can revoke keys")

	// 6. Admin can revoke
	cmdAdminRev := NewRotateCmd()
	outAdminRev, err := executeKeystoreCmd(cmdAdminRev, pkgctx.WithSecurityContext(context.Background(), adminCtx), []string{
		keyID,
		"--revoke",
		"--format", "json",
	})
	require.NoError(t, err)
	var rotResult map[string]any
	require.NoError(t, json.Unmarshal([]byte(outAdminRev), &rotResult))
	assert.Equal(t, keyID, rotResult["id"])

	// 7. Password key type rotation with bcrypt verification
	pwEntry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Password Key",
		objects.FieldKeyAccountID:      accID,
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "$2a$10$oldfakebcrypthashvalue",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, pwEntry))
	pwKeyID := pwEntry[objects.FieldKeyID].(string)
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, pwKeyID)

	cmdPwRot := NewRotateCmd()
	_, err = executeKeystoreCmd(cmdPwRot, pkgctx.WithSecurityContext(context.Background(), systemCtx), []string{
		pwKeyID,
		"--new-credential", "newly-rotated-password",
		"--revoke-old",
	})
	require.NoError(t, err)

	// Read and verify updated password hash with bcrypt
	updatedPwEntry, err := storageProvider.Read(ctx, systemCtx, pwKeyID)
	require.NoError(t, err)
	newHash, ok := updatedPwEntry[objects.FieldKeyCredentialHash].(string)
	require.True(t, ok)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(newHash), []byte("newly-rotated-password")))
	assert.True(t, updatedPwEntry[objects.FieldKeyRevoked].(bool))
}

func TestKeystoreSecurity_IdempotentRotationAndRevocation(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	accID := "ACC-1785920548450214003-23d25bd5"
	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Idempotency Test Key",
		objects.FieldKeyAccountID:      accID,
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:initialhash",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, entry))
	keyID := entry[objects.FieldKeyID].(string)
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, keyID)

	cmdCtx := pkgctx.WithSecurityContext(context.Background(), systemCtx)

	// First revocation
	cmdRev1 := NewRotateCmd()
	outRev1, err := executeKeystoreCmd(cmdRev1, cmdCtx, []string{keyID, "--revoke"})
	require.NoError(t, err)
	assert.Contains(t, outRev1, "revoked")

	// Second revocation (idempotent)
	cmdRev2 := NewRotateCmd()
	outRev2, err := executeKeystoreCmd(cmdRev2, cmdCtx, []string{keyID, "--revoke"})
	require.NoError(t, err)
	assert.Contains(t, outRev2, "revoked")

	readEntry, err := storageProvider.Read(ctx, systemCtx, keyID)
	require.NoError(t, err)
	assert.True(t, readEntry[objects.FieldKeyRevoked].(bool))

	// Sequential rotation: rotate credential twice
	cmdRot1 := NewRotateCmd()
	_, err = executeKeystoreCmd(cmdRot1, cmdCtx, []string{keyID, "--new-credential", "rotation-pass-1"})
	require.NoError(t, err)

	cmdRot2 := NewRotateCmd()
	outRot2, err := executeKeystoreCmd(cmdRot2, cmdCtx, []string{keyID, "--new-credential", "rotation-pass-2", "--format", "yaml"})
	require.NoError(t, err)
	assert.Contains(t, outRot2, "status: updated")
}

func TestKeystoreSecurity_CreateKeyFlagsAndEdgeCases(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	accID := "ACC-1785920548450214003-23d25bd5"
	userCtx := pkgctx.NewSecurityContext(accID, []string{"developer"}, []string{"read:*", "write:*"})
	adminCtx := pkgctx.NewSecurityContext("ACC-ADMIN-1", []string{"admin"}, []string{"read:*", "write:*"})

	// 1. Invalid key type
	cmdBadType := NewCreateCmd()
	_, err := executeKeystoreCmd(cmdBadType, pkgctx.WithSecurityContext(context.Background(), userCtx), []string{
		"--title", "Bad Type Key",
		"--credential", "mysecret",
		"--key-type", "rsa_private_key",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid key-type")

	// 2. System context without --account-id must fail
	cmdSysNoAcc := NewCreateCmd()
	_, err = executeKeystoreCmd(cmdSysNoAcc, pkgctx.WithSecurityContext(context.Background(), systemCtx), []string{
		"--title", "System Key",
		"--credential", "mysecret",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use system account")

	// 3. Non-admin attempting to create key for another account
	cmdNonAdminOther := NewCreateCmd()
	_, err = executeKeystoreCmd(cmdNonAdminOther, pkgctx.WithSecurityContext(context.Background(), userCtx), []string{
		"--title", "Other Key",
		"--credential", "mysecret",
		"--account-id", "ACC-SOMEONE-ELSE",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied: only admins can create keys for other accounts")

	// 4. Create password key type (bcrypt verified)
	cmdPw := NewCreateCmd()
	outPw, err := executeKeystoreCmd(cmdPw, pkgctx.WithSecurityContext(context.Background(), userCtx), []string{
		"--title", "My Secure Password",
		"--credential", "bcrypt-raw-password-1234",
		"--key-type", "password",
		"--description", "Database pass",
		"--expires-at", "2028-01-01T00:00:00Z",
		"--format", "json",
	})
	require.NoError(t, err)
	var pwCreated map[string]any
	require.NoError(t, json.Unmarshal([]byte(outPw), &pwCreated))
	createdID := pwCreated["id"].(string)

	createdEntry, err := storageProvider.Read(ctx, systemCtx, createdID)
	require.NoError(t, err)
	storedHash := createdEntry[objects.FieldKeyCredentialHash].(string)
	assert.NoError(t, bcrypt.CompareHashAndPassword([]byte(storedHash), []byte("bcrypt-raw-password-1234")))

	// 5. Admin creating token for another account with YAML format
	cmdAdminOther := NewCreateCmd()
	outAdminOther, err := executeKeystoreCmd(cmdAdminOther, pkgctx.WithSecurityContext(context.Background(), adminCtx), []string{
		"--title", "Delegated Token",
		"--credential", "token-secret-string",
		"--key-type", "oauth_token",
		"--account-id", "ACC-DELEGATED-USER",
		"--format", "yaml",
	})
	require.NoError(t, err)
	assert.Contains(t, outAdminOther, "ACC-DELEGATED-USER")
}

func TestKeystoreSecurity_ListEdgeCasesAndSecurityIsolation(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	accID := "ACC-1785920548450214003-23d25bd5"

	// 1. Empty list output
	cmdEmpty := NewListCmd()
	outEmpty, err := executeKeystoreCmd(cmdEmpty, pkgctx.WithSecurityContext(context.Background(), systemCtx), nil)
	require.NoError(t, err)
	assert.Contains(t, outEmpty, "No keystore entries found.")

	// Empty list JSON format
	cmdEmptyJSON := NewListCmd()
	outEmptyJSON, err := executeKeystoreCmd(cmdEmptyJSON, pkgctx.WithSecurityContext(context.Background(), systemCtx), []string{"--format", "json"})
	require.NoError(t, err)
	assert.Equal(t, "[]", strings.TrimSpace(outEmptyJSON))

	// Populate entries
	// Entry 1: Active, with full metadata
	secretDigest := authcred.HashAPIKey("supersecret-raw-value")
	entry1 := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Alpha Key",
		objects.FieldKeyAccountID:      accID,
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: secretDigest,
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyExpiresAt:      "2029-01-01T00:00:00Z",
		objects.FieldKeyLastUsedAt:     "2026-10-08T12:00:00Z",
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, entry1))
	id1 := entry1[objects.FieldKeyID].(string)
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, id1)

	// Entry 2: Revoked
	entry2 := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Revoked PAT Key",
		objects.FieldKeyAccountID:      accID,
		objects.FieldKeyKeyType:        "personal_access_token",
		objects.FieldKeyCredentialHash: "sha256:seconddigest",
		objects.FieldKeyRevoked:        true,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, entry2))
	id2 := entry2[objects.FieldKeyID].(string)
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, id2)

	// 2. Text output inspection
	cmdText := NewListCmd()
	outText, err := executeKeystoreCmd(cmdText, pkgctx.WithSecurityContext(context.Background(), systemCtx), nil)
	require.NoError(t, err)
	assert.Contains(t, outText, "Found 2 keystore entries:")
	assert.Contains(t, outText, "Title: Alpha Key")
	assert.Contains(t, outText, "Status: Active")
	assert.Contains(t, outText, "Status: REVOKED")
	assert.Contains(t, outText, "Expires: 2029-01-01T00:00:00Z")
	assert.Contains(t, outText, "Last Used: 2026-10-08T12:00:00Z")
	assert.Contains(t, outText, "Sensitive fields (credential_hash, salt) are never displayed")

	// Ensure secretDigest is NEVER in the text output
	assert.NotContains(t, outText, secretDigest)

	// 3. Filtering and pagination
	cmdFilter := NewListCmd()
	outFilter, err := executeKeystoreCmd(cmdFilter, pkgctx.WithSecurityContext(context.Background(), systemCtx), []string{
		"--filter", "key_type=api_key",
		"--sort-by", "title",
		"--sort-asc=false",
		"--offset", "0",
		"--limit", "10",
		"--format", "json",
	})
	require.NoError(t, err)
	var listArr []map[string]any
	require.NoError(t, json.Unmarshal([]byte(outFilter), &listArr))
	assert.Len(t, listArr, 1)
	assert.Equal(t, "Alpha Key", listArr[0]["title"])

	// Format YAML and JSONL
	cmdYAML := NewListCmd()
	outYAML, err := executeKeystoreCmd(cmdYAML, pkgctx.WithSecurityContext(context.Background(), systemCtx), []string{"--format", "yaml"})
	require.NoError(t, err)
	assert.Contains(t, outYAML, "Alpha Key")

	cmdJSONL := NewListCmd()
	_, err = executeKeystoreCmd(cmdJSONL, pkgctx.WithSecurityContext(context.Background(), systemCtx), []string{"--format", "jsonl"})
	require.NoError(t, err)
}

func TestKeystoreSecurity_StoragePermissionsAndPathIsolation(t *testing.T) {
	tmpDir, _, systemCtx := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)

	// 1. Verify keystore directory permissions are 0700
	processDir := datacell.ProcessPrimaryDir(tmpDir)
	keystoreDir := filepath.Join(processDir, "keystore")
	dirInfo, err := fileutil.Stat(keystoreDir)
	require.NoError(t, err)
	assert.Equal(t, fileutil.FileMode(0700), dirInfo.Mode().Perm())

	// 2. Environment isolation test: WithSeatAPIKeyEnv strips sensitive parent variables
	parentEnv := []string{
		"PATH=/usr/bin:/bin",
		"USER=testuser",
		"AWS_SECRET_ACCESS_KEY=should-be-stripped",
		"GITHUB_TOKEN=should-be-stripped",
		"ZQK_CUSTOM_VAR=allowed",
		"ZQK_API_KEY=old-key-to-replace",
	}
	seatKey := "mock-seat-key-placeholder"
	filtered := authcred.WithSeatAPIKeyEnv(parentEnv, seatKey, tmpDir)

	hasAPIKey := false
	hasProjectRoot := false
	for _, env := range filtered {
		assert.False(t, strings.HasPrefix(env, "AWS_SECRET_ACCESS_KEY="))
		assert.False(t, strings.HasPrefix(env, "GITHUB_TOKEN="))
		if env == "ZQK_API_KEY="+seatKey {
			hasAPIKey = true
		}
		if env == zqkenv.ProjectRoot().Name()+"="+tmpDir {
			hasProjectRoot = true
		}
	}
	assert.True(t, hasAPIKey)
	assert.True(t, hasProjectRoot)

	// 3. Credential path resolution fallback test
	credPath := authcred.ResolveCredentialPath(tmpDir)
	assert.NotEmpty(t, credPath)

	_ = systemCtx
}
