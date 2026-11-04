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

	"github.com/zqk-os/zqk/pkg/authcred"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func setupCycle104KeystoreTest(t *testing.T) (string, *storage.FileObjectStorage, *pkgctx.SecurityContext) {
	t.Helper()
	t.Setenv(zqkenv.TestAllowCASFallthrough().Name(), "1")
	t.Setenv(zqkenv.PrivilegedWriterSocket().Name(), zqkenv.UnreachableTestSocketPath())

	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:            "keystore_cycle104",
		SeedSchemaPlane: true,
		AppendStagesBeforeStorage: func(root string) []testkit.NamedTestStep {
			return []testkit.NamedTestStep{{
				Name: "keystore_storage_layout",
				Fn: func() error {
					processDir := datacell.ProcessPrimaryDir(root)
					keystoreDir := filepath.Join(processDir, "keystore")
					return fileutil.MkdirAll(keystoreDir, paths.DirPerm700)
				},
			}}
		},
	})

	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	specsDir := filepath.Join(proj.Root, paths.ProcessInternalObjectSpecsDir)
	validator := validation.GetIDValidator()
	validator.SetSpecsDirForTest(specsDir)
	if err := validator.ReloadPatterns(); err != nil {
		t.Fatalf("Failed to reload ID patterns: %v", err)
	}

	sysCtx := pkgctx.NewSystemSecurityContext()
	return proj.Root, proj.FileStorage, sysCtx
}

func executeCycle104Cmd(cmd *cobra.Command, ctx context.Context, args []string) (string, error) {
	buf := new(bytes.Buffer)
	ctxWithWriter := pkgctx.WithCommandOutputWriter(ctx, buf)
	cmd.SetContext(ctxWithWriter)
	cmd.SetArgs(args)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	err := cmd.ExecuteContext(ctxWithWriter)
	return buf.String(), err
}

func seedCycle104Account(t *testing.T, storageProvider *storage.FileObjectStorage, ctx context.Context, sysCtx *pkgctx.SecurityContext, accID, username string) {
	t.Helper()
	account := map[string]any{
		objects.FieldKeyKind:          objects.KindAccount,
		objects.FieldKeyID:            accID,
		objects.FieldKeyTitle:         username,
		objects.FieldKeyUsername:      username,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, sysCtx, account))
}

func TestCycle104_CreateCommandBranches(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupCycle104KeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	accOwner := "ACC-COMMUNITY-001"
	accOther := "ACC-COMMUNITY-002"
	seedCycle104Account(t, storageProvider, ctx, systemCtx, accOwner, "community_user")
	seedCycle104Account(t, storageProvider, ctx, systemCtx, accOther, "other_user")

	ownerCtx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{AccountID: accOwner})
	otherUserCtx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{AccountID: accOther})
	adminCtx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{AccountID: "ACC-ADMIN-001", Roles: []string{"admin"}})

	// 1. Invalid key-type branch
	{
		cmd := NewCreateCmd()
		_, err := executeCycle104Cmd(cmd, ownerCtx, []string{
			"--key-type", "unsupported_token_type",
			"--credential", "testpass",
			"--title", "Bad Type",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid key-type")
	}

	// 2. Permission denied: non-admin trying to create key for another account
	{
		cmd := NewCreateCmd()
		_, err := executeCycle104Cmd(cmd, otherUserCtx, []string{
			"--account-id", accOwner,
			"--key-type", "api_key",
			"--credential", "tok-12345",
			"--title", "Spoof Key",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied: only admins can create keys for other accounts")
	}

	// 3. Password bcrypt error: password > 72 bytes triggers bcrypt.ErrPasswordTooLong
	{
		cmd := NewCreateCmd()
		longPassword := strings.Repeat("A", 100)
		_, err := executeCycle104Cmd(cmd, ownerCtx, []string{
			"--key-type", "password",
			"--credential", longPassword,
			"--title", "Excessively Long Password",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to hash secret")
	}

	// 4. Successful creation of password key with default text output including description and expiration
	{
		cmd := NewCreateCmd()
		out, err := executeCycle104Cmd(cmd, ownerCtx, []string{
			"--key-type", "password",
			"--credential", "validSecurePassword99!",
			"--title", "Database Master Password",
			"--description", "Production DB access credential",
			"--expires-at", "2028-12-31T23:59:59Z",
		})
		require.NoError(t, err)
		assert.Contains(t, out, "Keystore entry created successfully")
		assert.Contains(t, out, "ID:")
		assert.Contains(t, out, "Account:    "+accOwner)
		assert.Contains(t, out, "Key Type:   password")
		assert.Contains(t, out, "Title:      Database Master Password")
		assert.Contains(t, out, "Description: Production DB access credential")
		assert.Contains(t, out, "Expires:    2028-12-31T23:59:59Z")
		assert.Contains(t, out, "Keep your credential secure")
	}

	// 5. Successful creation with admin context for other account in JSON format
	{
		cmd := NewCreateCmd()
		out, err := executeCycle104Cmd(cmd, adminCtx, []string{
			"--account-id", accOwner,
			"--key-type", "api_key",
			"--credential", "admin-minted-secret-key",
			"--title", "Admin Minted Service Key",
			"--format", "json",
			"--expires-at", "2029-01-01T00:00:00Z",
		})
		require.NoError(t, err)
		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &parsed))
		assert.Equal(t, accOwner, parsed[objects.FieldKeyAccountID])
		assert.Equal(t, "api_key", parsed[objects.FieldKeyKeyType])
		assert.Equal(t, "Admin Minted Service Key", parsed[objects.FieldKeyTitle])
		assert.Equal(t, objects.ObjectStatusCreated, parsed[objects.FieldKeyStatus])
		assert.NotEmpty(t, parsed[objects.FieldKeyID])
	}

	// 6. Successful creation in YAML and JSONL formats
	{
		cmdYAML := NewCreateCmd()
		outYAML, err := executeCycle104Cmd(cmdYAML, ownerCtx, []string{
			"--key-type", "personal_access_token",
			"--credential", "pat-sample-credential",
			"--title", "YAML Formatted PAT",
			"--format", "yaml",
		})
		require.NoError(t, err)
		assert.Contains(t, outYAML, "account_id: "+accOwner)
		assert.Contains(t, outYAML, "key_type: personal_access_token")

		cmdJSONL := NewCreateCmd()
		outJSONL, err := executeCycle104Cmd(cmdJSONL, ownerCtx, []string{
			"--key-type", "oauth_token",
			"--credential", "oauth-sample-token",
			"--title", "JSONL Formatted OAuth",
			"--format", "jsonl",
		})
		require.NoError(t, err)
		var parsedJSONL map[string]any
		require.NoError(t, json.Unmarshal([]byte(outJSONL), &parsedJSONL))
		assert.Equal(t, "oauth_token", parsedJSONL[objects.FieldKeyKeyType])
	}
}

func TestCycle104_IssueCommandBranches(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupCycle104KeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	accID := "ACC-COMMUNITY-001"
	seedCycle104Account(t, storageProvider, ctx, systemCtx, accID, "worker_agent")

	ownerCtx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{AccountID: accID})
	otherUserCtx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{AccountID: "ACC-COMMUNITY-002"})
	adminCtx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{AccountID: "ACC-ADMIN-001", Roles: []string{"admin"}})

	// 1. Invalid account ID format (not starting with ACC-)
	{
		cmd := NewIssueCmd()
		_, err := executeCycle104Cmd(cmd, ownerCtx, []string{
			"--account-id", "INVALID-PREFIX-001",
			"--title", "Malformed Account ID",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "account-id must be ACC-* form")
	}

	// 2. Permission denied: other user without admin role
	{
		cmd := NewIssueCmd()
		_, err := executeCycle104Cmd(cmd, otherUserCtx, []string{
			"--account-id", accID,
			"--title", "Unauthorized Issue",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied: only admins (or the account owner) can issue keys")
	}

	// 3. Issue with --no-seating-file in JSON format
	{
		cmd := NewIssueCmd()
		out, err := executeCycle104Cmd(cmd, ownerCtx, []string{
			"--account-id", accID,
			"--title", "Seat Key Without File",
			"--no-seating-file",
			"--format", "json",
		})
		require.NoError(t, err)
		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &parsed))
		assert.Equal(t, accID, parsed[objects.FieldKeyAccountID])
		assert.NotEmpty(t, parsed["api_key"])
		assert.NotEmpty(t, parsed["fingerprint"])
		assert.Empty(t, parsed["seating_file"])
		assert.Contains(t, parsed["env_hint"], zqkenv.APIKey().Name())

		// Verify seating file was NOT created
		seatingPath := authcred.SeatKeyPath(tmpDir, accID)
		_, statErr := fileutil.Stat(seatingPath)
		assert.True(t, fileutil.IsNotExist(statErr))
	}

	// 4. Issue with admin context in YAML format
	{
		cmd := NewIssueCmd()
		out, err := executeCycle104Cmd(cmd, adminCtx, []string{
			"--account-id", accID,
			"--title", "Admin Issued Seat",
			"--format", "yaml",
		})
		require.NoError(t, err)
		assert.Contains(t, out, "account_id: "+accID)
		assert.Contains(t, out, "api_key:")
		assert.Contains(t, out, "seating_file:")
	}

	// 5. Issue with text output format (verifying export instruction)
	{
		cmd := NewIssueCmd()
		out, err := executeCycle104Cmd(cmd, ownerCtx, []string{
			"--account-id", accID,
			"--title", "CLI Human Seat Key",
		})
		require.NoError(t, err)
		assert.Contains(t, out, "API key issued (plaintext shown once)")
		assert.Contains(t, out, "Key ID:")
		assert.Contains(t, out, "Account:      "+accID)
		assert.Contains(t, out, "Fingerprint:")
		assert.Contains(t, out, "API Key:")
		assert.Contains(t, out, "Seating file:")
		assert.Contains(t, out, "export "+zqkenv.APIKey().String()+"=")
	}
}

func TestCycle104_ListCommandBranches(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupCycle104KeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	accID := "ACC-COMMUNITY-001"
	seedCycle104Account(t, storageProvider, ctx, systemCtx, accID, "listing_user")
	userCtx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{AccountID: accID})

	// 1. Empty list: default text format
	{
		cmd := NewListCmd()
		out, err := executeCycle104Cmd(cmd, userCtx, []string{})
		require.NoError(t, err)
		assert.Equal(t, "No keystore entries found.\n", out)
	}

	// 2. Empty list: JSON, JSONL, YAML formats
	{
		cmdJSON := NewListCmd()
		outJSON, err := executeCycle104Cmd(cmdJSON, userCtx, []string{"--format", "json"})
		require.NoError(t, err)
		assert.Contains(t, outJSON, "[]")

		cmdYAML := NewListCmd()
		outYAML, err := executeCycle104Cmd(cmdYAML, userCtx, []string{"--format", "yaml"})
		require.NoError(t, err)
		assert.Contains(t, outYAML, "[]")

		cmdJSONL := NewListCmd()
		outJSONL, err := executeCycle104Cmd(cmdJSONL, userCtx, []string{"--format", "jsonl"})
		require.NoError(t, err)
		assert.Contains(t, outJSONL, "[]")
	}

	// 3. Seed one active entry and test singular pluralize formatting
	entry1 := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "First Active Key",
		objects.FieldKeyAccountID:      accID,
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:fakehash01",
		objects.FieldKeySalt:           "",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, entry1))
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, entry1[objects.FieldKeyID].(string))

	{
		cmd := NewListCmd()
		out, err := executeCycle104Cmd(cmd, userCtx, []string{})
		require.NoError(t, err)
		assert.Contains(t, out, "Found 1 keystore entry:")
		assert.Contains(t, out, "Title: First Active Key")
		assert.Contains(t, out, "Status: Active")
		assert.Contains(t, out, "Sensitive fields (credential_hash, salt) are never displayed")
		assert.NotContains(t, out, "sha256:fakehash01")
	}

	// 4. Seed a second entry marked as revoked with expires_at and last_used_at
	entry2 := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Second Revoked Key",
		objects.FieldKeyAccountID:      accID,
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "sha256:fakehash02",
		objects.FieldKeySalt:           "",
		objects.FieldKeyRevoked:        true,
		objects.FieldKeyExpiresAt:      "2027-10-10T00:00:00Z",
		objects.FieldKeyLastUsedAt:     "2026-10-15T12:00:00Z",
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, entry2))
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, entry2[objects.FieldKeyID].(string))

	{
		cmd := NewListCmd()
		out, err := executeCycle104Cmd(cmd, userCtx, []string{})
		require.NoError(t, err)
		assert.Contains(t, out, "Found 2 keystore entries:")
		assert.Contains(t, out, "Status: REVOKED")
		assert.Contains(t, out, "Expires: 2027-10-10T00:00:00Z")
		assert.Contains(t, out, "Last Used: 2026-10-15T12:00:00Z")
	}

	// 5. List with filters, sorting, and pagination
	{
		cmdFiltered := NewListCmd()
		outFiltered, err := executeCycle104Cmd(cmdFiltered, userCtx, []string{
			"--filter", "key_type=password",
			"--format", "json",
		})
		require.NoError(t, err)
		var listJSON []map[string]any
		require.NoError(t, json.Unmarshal([]byte(outFiltered), &listJSON))
		assert.Len(t, listJSON, 1)
		assert.Equal(t, "Second Revoked Key", listJSON[0][objects.FieldKeyTitle])

		// Sorting and pagination flags
		cmdSorted := NewListCmd()
		outSorted, err := executeCycle104Cmd(cmdSorted, userCtx, []string{
			"--sort-by", "title",
			"--sort-asc=false",
			"--limit", "1",
			"--offset", "0",
			"--format", "json",
		})
		require.NoError(t, err)
		var sortedJSON []map[string]any
		require.NoError(t, json.Unmarshal([]byte(outSorted), &sortedJSON))
		assert.Len(t, sortedJSON, 1)
		assert.Equal(t, "Second Revoked Key", sortedJSON[0][objects.FieldKeyTitle])
	}
}

func TestCycle104_RotateCommandBranches(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupCycle104KeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	accID := "ACC-COMMUNITY-001"
	seedCycle104Account(t, storageProvider, ctx, systemCtx, accID, "rotating_user")

	ownerCtx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{AccountID: accID})
	otherUserCtx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{AccountID: "ACC-COMMUNITY-002"})
	adminCtx := pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{AccountID: "ACC-ADMIN-001", Roles: []string{"admin"}})
	systemCmdCtx := pkgctx.WithSecurityContext(context.Background(), systemCtx)

	// Seed an active API key entry
	entryAPI := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Primary API Key",
		objects.FieldKeyAccountID:      accID,
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:initialhash",
		objects.FieldKeySalt:           "",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, entryAPI))
	keyIDAPI := entryAPI[objects.FieldKeyID].(string)
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, keyIDAPI)

	// Seed an active Password entry
	entryPwd := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Primary Password Key",
		objects.FieldKeyAccountID:      accID,
		objects.FieldKeyKeyType:        "password",
		objects.FieldKeyCredentialHash: "$2a$10$abcdefghijklmnopqrstuvwxyz012345",
		objects.FieldKeySalt:           "",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}
	require.NoError(t, storageProvider.Create(ctx, systemCtx, entryPwd))
	keyIDPwd := entryPwd[objects.FieldKeyID].(string)
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, keyIDPwd)

	// 1. Missing flags error
	{
		cmd := NewRotateCmd()
		_, err := executeCycle104Cmd(cmd, systemCmdCtx, []string{keyIDAPI})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "must specify either --new-credential, --revoke, or --revoke-old")
	}

	// 2. Non-existent entry ID error
	{
		cmd := NewRotateCmd()
		_, err := executeCycle104Cmd(cmd, systemCmdCtx, []string{"KEY-NONEXISTENT-001", "--revoke"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to read keystore entry")
	}

	// 3. Object is not a keystore_entry (target is the account object)
	{
		cmd := NewRotateCmd()
		_, err := executeCycle104Cmd(cmd, systemCmdCtx, []string{accID, "--revoke"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "is not a keystore_entry")
	}

	// 4. Revocation permission denied: unauthorized user
	{
		cmd := NewRotateCmd()
		_, err := executeCycle104Cmd(cmd, otherUserCtx, []string{keyIDAPI, "--revoke"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied: only owner, admin, or system can revoke keys")
	}

	// 5. Rotation permission denied: non-system context cannot update credential_hash
	{
		cmd := NewRotateCmd()
		_, err := executeCycle104Cmd(cmd, ownerCtx, []string{keyIDAPI, "--new-credential", "new-secret"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied: only system can rotate keys")
	}

	// 6. Password rotation bcrypt error with credential > 72 bytes
	{
		cmd := NewRotateCmd()
		longCred := strings.Repeat("B", 100)
		_, err := executeCycle104Cmd(cmd, systemCmdCtx, []string{keyIDPwd, "--new-credential", longCred})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to hash secret")
	}

	// 7. Successful revocation by key owner in JSON format
	{
		cmd := NewRotateCmd()
		out, err := executeCycle104Cmd(cmd, ownerCtx, []string{keyIDAPI, "--revoke", "--format", "json"})
		require.NoError(t, err)
		var parsed map[string]any
		require.NoError(t, json.Unmarshal([]byte(out), &parsed))
		assert.Equal(t, keyIDAPI, parsed[objects.FieldKeyID])
		assert.Equal(t, objects.ObjectStatusUpdated, parsed[objects.FieldKeyStatus])
		actions, ok := parsed["actions"].([]any)
		require.True(t, ok)
		assert.Contains(t, actions, "revoked")
	}

	// 8. Successful rotation and revoke-old by system in text format
	{
		cmd := NewRotateCmd()
		out, err := executeCycle104Cmd(cmd, systemCmdCtx, []string{
			keyIDPwd,
			"--new-credential", "rotatedSecurePassword456!",
			"--revoke-old",
		})
		require.NoError(t, err)
		assert.Contains(t, out, "Keystore entry updated successfully")
		assert.Contains(t, out, "ID: "+keyIDPwd)
		assert.Contains(t, out, "Actions: rotated, revoked")
		assert.Contains(t, out, "Key has been rotated")
		assert.Contains(t, out, "Key has been revoked")
	}

	// 9. Admin revocation in YAML format
	{
		cmd := NewRotateCmd()
		out, err := executeCycle104Cmd(cmd, adminCtx, []string{keyIDPwd, "--revoke", "--format", "yaml"})
		require.NoError(t, err)
		assert.Contains(t, out, "id: "+keyIDPwd)
		assert.Contains(t, out, "status: updated")
	}
}

func TestCycle104_HelperUnitsAndEdgeCases(t *testing.T) {
	// 1. validateKeyType
	assert.NoError(t, validateKeyType("password"))
	assert.NoError(t, validateKeyType("api_key"))
	assert.NoError(t, validateKeyType("oauth_token"))
	assert.NoError(t, validateKeyType("personal_access_token"))
	assert.Error(t, validateKeyType(""))
	assert.Error(t, validateKeyType("invalid_key_type"))

	// 2. pluralize
	assert.Equal(t, "ies", pluralize(0))
	assert.Equal(t, "y", pluralize(1))
	assert.Equal(t, "ies", pluralize(2))
	assert.Equal(t, "ies", pluralize(10))

	// 3. determineAccountID edge cases
	sysCtx := pkgctx.NewSystemSecurityContext()
	userCtx := &pkgctx.SecurityContext{AccountID: "ACC-COMMUNITY-001"}
	adminCtx := &pkgctx.SecurityContext{AccountID: "ACC-ADMIN-001", Roles: []string{"admin"}}

	// Empty account ID with system context returns error
	_, err := determineAccountID(&CreateFlags{AccountID: ""}, sysCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot use system account")

	// Empty account ID with user context returns user account
	acc1, err := determineAccountID(&CreateFlags{AccountID: ""}, userCtx)
	require.NoError(t, err)
	assert.Equal(t, "ACC-COMMUNITY-001", acc1)

	// User setting other account without admin returns permission denied
	_, err = determineAccountID(&CreateFlags{AccountID: "ACC-OTHER-002"}, userCtx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "permission denied")

	// Admin setting other account succeeds
	accAdminSet, err := determineAccountID(&CreateFlags{AccountID: "ACC-OTHER-002"}, adminCtx)
	require.NoError(t, err)
	assert.Equal(t, "ACC-OTHER-002", accAdminSet)

	// 4. authorizeIssueForAccount edge cases
	assert.ErrorContains(t, authorizeIssueForAccount(nil, "ACC-COMMUNITY-001"), "missing security context")
	assert.NoError(t, authorizeIssueForAccount(sysCtx, "ACC-COMMUNITY-001"))
	assert.NoError(t, authorizeIssueForAccount(userCtx, "ACC-COMMUNITY-001"))
	assert.NoError(t, authorizeIssueForAccount(adminCtx, "ACC-COMMUNITY-001"))
	assert.ErrorContains(t, authorizeIssueForAccount(userCtx, "ACC-OTHER-002"), "permission denied")

	// 5. checkPermissions unit checks
	isAdmin, hasPerm, err := checkPermissions(sysCtx, "ACC-COMMUNITY-001", true)
	require.NoError(t, err)
	assert.True(t, isAdmin)
	assert.False(t, hasPerm)

	_, _, err = checkPermissions(userCtx, "ACC-COMMUNITY-001", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "only system can rotate keys")

	isAdmin, hasPerm, err = checkPermissions(adminCtx, "ACC-COMMUNITY-001", false)
	require.NoError(t, err)
	assert.True(t, isAdmin)

	// 6. buildRevocationUpdates
	assert.Nil(t, buildRevocationUpdates(false, false))
	revUpdates := buildRevocationUpdates(true, false)
	require.NotNil(t, revUpdates)
	assert.Equal(t, true, revUpdates[objects.FieldKeyRevoked])
	assert.NotEmpty(t, revUpdates[objects.FieldKeyRevokedAt])

	// 7. buildRotationUpdates
	rotEmpty, err := buildRotationUpdates("", "api_key")
	require.NoError(t, err)
	assert.Nil(t, rotEmpty)

	rotToken, err := buildRotationUpdates("secret-api-val", "api_key")
	require.NoError(t, err)
	require.NotNil(t, rotToken)
	assert.NotEmpty(t, rotToken[objects.FieldKeyCredentialHash])

	rotPassword, err := buildRotationUpdates("shortpwd", "password")
	require.NoError(t, err)
	require.NotNil(t, rotPassword)
	assert.NotEmpty(t, rotPassword[objects.FieldKeyCredentialHash])

	// 8. buildAllUpdates empty error
	_, err = buildAllUpdates(&RotateFlags{}, "api_key")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no updates specified")

	// 9. determineUpdateContext
	assert.Equal(t, pkgctx.SystemAccountID, determineUpdateContext(userCtx, "new-secret-val").AccountID)
	assert.Equal(t, userCtx.AccountID, determineUpdateContext(userCtx, "").AccountID)

	// 10. appendAccountTokenFingerprint on non-existent account
	tmpDir, _, systemStorageCtx := setupCycle104KeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	cliProc, err := cli.NewProcessor(&cobra.Command{})
	require.NoError(t, err)
	_ = systemStorageCtx
	err = appendAccountTokenFingerprint(cliProc, "ACC-DOES-NOT-EXIST-001", "KEY-DEMO-001", "sha256:fake", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read account")
}
