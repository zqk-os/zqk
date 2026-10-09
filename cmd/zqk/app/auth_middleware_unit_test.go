package app

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/zqk-os/zqk/pkg/authcred"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestResolveSecurityContext_UnitBranches(t *testing.T) {
	// 1. Invalid prefix
	secCtx, err := resolveSecurityContext("", "INVALID-ID")
	assert.Error(t, err)
	assert.Nil(t, secCtx)
	assert.Contains(t, err.Error(), "must use ACC-* form")

	// 2. System account with explicit persona
	secCtx, err = resolveSecurityContext("", pkgctx.SystemAccountID+"|role|PER-CUSTOM")
	assert.NoError(t, err)
	assert.NotNil(t, secCtx)
	assert.Equal(t, "PER-CUSTOM", secCtx.PersonaID)

	// 3. Test harness account
	secCtx, err = resolveSecurityContext("", "ACC-TEST-HARNESS|PER-TEST")
	assert.NoError(t, err)
	assert.NotNil(t, secCtx)
	assert.Equal(t, "PER-TEST", secCtx.PersonaID)

	// 4. Default swarm worker account
	secCtx, err = resolveSecurityContext("", authcred.DefaultSwarmWorkerAccount)
	assert.NoError(t, err)
	assert.NotNil(t, secCtx)
	assert.Equal(t, "PER-SWARM-WORKER", secCtx.PersonaID)

	// 5. Account lookup error on nonexistent project root
	secCtx, err = resolveSecurityContext("/tmp/nonexistent-root", "ACC-UNKNOWN")
	assert.Error(t, err)
	assert.Nil(t, secCtx)
}

func TestAuthMiddleware_Helpers(t *testing.T) {
	assert.Equal(t, "val", firstNonEmpty("", "  ", "val", "other"))
	assert.Equal(t, "", firstNonEmpty("", "  "))

	assert.Equal(t, "val", firstString([]string{"", "val", "other"}))
	assert.Equal(t, "", firstString([]string{"", "  "}))
}
