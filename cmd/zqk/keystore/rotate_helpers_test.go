package keystore

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestValidateKeystoreEntry(t *testing.T) {
	valid := map[string]any{
		objects.FieldKeyKind: objects.KindKeystoreEntry,
	}
	assert.NoError(t, validateKeystoreEntry(valid, "KEY-1"))

	invalid := map[string]any{
		objects.FieldKeyKind: objects.KindBacklogItem,
	}
	assert.ErrorContains(t, validateKeystoreEntry(invalid, "KEY-1"), "is not a keystore_entry")
}

func TestCheckPermissions(t *testing.T) {
	// 1. Regular user accessing their own account
	userCtx := &pkgctx.SecurityContext{AccountID: "ACC-USER-1"}
	isAdmin, hasPerm, err := checkPermissions(userCtx, "ACC-USER-1", false)
	assert.False(t, isAdmin)
	assert.True(t, hasPerm)
	assert.NoError(t, err)

	// 2. Regular user requires system -> permission denied
	_, _, err = checkPermissions(userCtx, "ACC-USER-1", true)
	assert.ErrorContains(t, err, "only system can rotate keys")

	// 3. System context
	sysCtx := pkgctx.NewSystemSecurityContext()
	_, _, err = checkPermissions(sysCtx, "ACC-USER-1", true)
	assert.NoError(t, err)

	// 4. Admin role detection
	adminCtx := &pkgctx.SecurityContext{AccountID: "ACC-ADMIN-1", Roles: []string{"admin"}}
	isAdmin, _, err = checkPermissions(adminCtx, "ACC-OTHER", false)
	assert.True(t, isAdmin)
	assert.NoError(t, err)
}

func TestBuildRevocationAndRotationUpdates(t *testing.T) {
	// Revocation updates
	rev := buildRevocationUpdates(true, false)
	require.NotNil(t, rev)
	assert.True(t, rev[objects.FieldKeyRevoked].(bool))
	assert.NotEmpty(t, rev[objects.FieldKeyRevokedAt])

	nilRev := buildRevocationUpdates(false, false)
	assert.Nil(t, nilRev)

	// Rotation updates - password
	pwUpdates, err := buildRotationUpdates("new-password", "password")
	require.NoError(t, err)
	assert.NotEmpty(t, pwUpdates[objects.FieldKeyCredentialHash])

	// Rotation updates - token
	tokUpdates, err := buildRotationUpdates("new-token", "api_key")
	require.NoError(t, err)
	assert.Contains(t, tokUpdates[objects.FieldKeyCredentialHash].(string), "sha256:")

	// Empty value
	emptyUpdates, err := buildRotationUpdates("", "api_key")
	require.NoError(t, err)
	assert.Nil(t, emptyUpdates)
}

func TestBuildAllUpdatesAndContext(t *testing.T) {
	// Empty flags -> error
	flagsEmpty := &RotateFlags{}
	_, err := buildAllUpdates(flagsEmpty, "api_key")
	assert.ErrorContains(t, err, "no updates specified")

	// Both rotation and revocation
	flagsBoth := &RotateFlags{NewKeyData: "new-secret", RevokeOld: true}
	allUpdates, err := buildAllUpdates(flagsBoth, "api_key")
	require.NoError(t, err)
	assert.True(t, allUpdates[objects.FieldKeyRevoked].(bool))
	assert.NotEmpty(t, allUpdates[objects.FieldKeyCredentialHash])

	// determineUpdateContext
	userCtx := &pkgctx.SecurityContext{AccountID: "ACC-USER-1"}
	sysCtx := determineUpdateContext(userCtx, "new-secret")
	assert.Equal(t, pkgctx.SystemAccountID, sysCtx.AccountID)

	userCtxRet := determineUpdateContext(userCtx, "")
	assert.Equal(t, "ACC-USER-1", userCtxRet.AccountID)

	// buildRotateResult and formatRotateOutputText
	res := buildRotateResult("KEY-100", flagsBoth)
	assert.Equal(t, "KEY-100", res[objects.FieldKeyID])
	txt := formatRotateOutputText("KEY-100", flagsBoth, res)
	assert.Contains(t, string(txt), "KEY-100")
	assert.Contains(t, string(txt), "rotated")
}

func TestParseRotateFlags(t *testing.T) {
	// 1. Empty flags -> error
	cmd := NewRotateCmd()
	flags, err := parseRotateFlags(cmd)
	assert.ErrorContains(t, err, "must specify either")
	assert.Nil(t, flags)

	// 2. --revoke
	cmd = NewRotateCmd()
	_ = cmd.Flags().Set("revoke", "true")
	flags, err = parseRotateFlags(cmd)
	require.NoError(t, err)
	assert.True(t, flags.Revoke)
	assert.False(t, flags.RevokeOld)

	// 3. --revoke-old
	cmd = NewRotateCmd()
	_ = cmd.Flags().Set("revoke-old", "true")
	flags, err = parseRotateFlags(cmd)
	require.NoError(t, err)
	assert.False(t, flags.Revoke)
	assert.True(t, flags.RevokeOld)

	// 4. --new-credential
	cmd = NewRotateCmd()
	_ = cmd.Flags().Set("new-credential", "new-secret-val")
	flags, err = parseRotateFlags(cmd)
	require.NoError(t, err)
	assert.Equal(t, "new-secret-val", flags.NewKeyData)
}

func TestRotateCmd_ExecuteRevoke(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	entry := map[string]any{
		objects.FieldKeyKind:           objects.KindKeystoreEntry,
		objects.FieldKeyTitle:          "Key To Rotate",
		objects.FieldKeyAccountID:      "ACC-1785920548450214003-23d25bd5",
		objects.FieldKeyKeyType:        "api_key",
		objects.FieldKeyCredentialHash: "sha256:fakehash",
		objects.FieldKeyRevoked:        false,
		objects.FieldKeyStatus:         objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion:  objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject:  validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:   validation.DefaultOriginSystem,
	}

	require.NoError(t, storageProvider.Create(ctx, systemCtx, entry))
	id, _ := entry[objects.FieldKeyID].(string)
	promoteKeystoreEntryOffDraft(t, storageProvider, ctx, systemCtx, id)

	cmdCtx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())

	cmd := NewRotateCmd()
	cmd.SetContext(cmdCtx)
	cmd.SetArgs([]string{id, "--revoke"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	// JSON format
	cmdJSON := NewRotateCmd()
	cmdJSON.SetContext(cmdCtx)
	cmdJSON.SetArgs([]string{id, "--revoke", "--format", "json"})
	bufJSON := new(bytes.Buffer)
	cmdJSON.SetOut(bufJSON)
	cmdJSON.SetErr(bufJSON)

	err = cmdJSON.Execute()
	require.NoError(t, err)
}
