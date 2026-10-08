package keystore

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/authcred"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/validation"
)

func TestAuthorizeIssueForAccount(t *testing.T) {
	// 1. Nil security context -> error
	err := authorizeIssueForAccount(nil, "ACC-TARGET")
	assert.ErrorContains(t, err, "missing security context")

	// 2. System account -> allowed
	sysCtx := pkgctx.NewSystemSecurityContext()
	assert.NoError(t, authorizeIssueForAccount(sysCtx, "ACC-TARGET"))

	// 3. Account owner -> allowed
	ownerCtx := &pkgctx.SecurityContext{AccountID: "ACC-OWNER"}
	assert.NoError(t, authorizeIssueForAccount(ownerCtx, "ACC-OWNER"))

	// 4. Admin role -> allowed for other accounts
	adminCtx := &pkgctx.SecurityContext{AccountID: "ACC-ADMIN", Roles: []string{"admin"}}
	assert.NoError(t, authorizeIssueForAccount(adminCtx, "ACC-OTHER"))

	// 5. Non-admin other account -> permission denied
	userCtx := &pkgctx.SecurityContext{AccountID: "ACC-USER", Roles: []string{"developer"}}
	err = authorizeIssueForAccount(userCtx, "ACC-OTHER")
	assert.ErrorContains(t, err, "permission denied")
}

func TestGenerateSeatCode(t *testing.T) {
	code, err := generateSeatCode()
	require.NoError(t, err)
	assert.NotEmpty(t, code)
	assert.True(t, strings.HasPrefix(code, authcred.AgentPrefix))
}

func TestIssueCmd_Execute(t *testing.T) {
	tmpDir, storageProvider, systemCtx := setupKeystoreTest(t)
	t.Setenv("ZQK_PROJECT_ROOT", tmpDir)
	ctx := pkgctx.NewSystemContext()

	accID := "ACC-1785920548450214003-23d25bd5"
	account := map[string]any{
		objects.FieldKeyKind:          objects.KindAccount,
		objects.FieldKeyID:            accID,
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
		objects.FieldKeyOriginProject: validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:  validation.DefaultOriginSystem,
	}
	_ = storageProvider.Create(ctx, systemCtx, account)

	cmdCtx := pkgctx.WithSecurityContext(context.Background(), pkgctx.NewSystemSecurityContext())

	cmd := NewIssueCmd()
	cmd.SetContext(cmdCtx)
	cmd.SetArgs([]string{"--account-id", accID, "--title", "Test Issue Key", "--no-seating-file"})
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)

	err := cmd.Execute()
	require.NoError(t, err)

	// JSON format
	cmdJSON := NewIssueCmd()
	cmdJSON.SetContext(cmdCtx)
	cmdJSON.SetArgs([]string{"--account-id", accID, "--title", "Test JSON Key", "--no-seating-file", "--format", "json"})
	bufJSON := new(bytes.Buffer)
	cmdJSON.SetOut(bufJSON)
	cmdJSON.SetErr(bufJSON)

	err = cmdJSON.Execute()
	require.NoError(t, err)
}
