package agent

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
)

func TestNewSynthesizeSkillCmd_Structure(t *testing.T) {
	cmd := NewSynthesizeSkillCmd()
	require.NotNil(t, cmd)

	flags := cmd.Flags()
	assert.NotNil(t, flags.Lookup("capability"))
	assert.NotNil(t, flags.Lookup("provider"))
}

func TestSynthesizeSkillCommitContext(t *testing.T) {
	// Nil context
	ctxNil := synthesizeSkillCommitContext(nil)
	require.NotNil(t, ctxNil)
	assert.True(t, pkgctx.GetPromoteOnCreate(ctxNil))

	// Existing context
	baseCtx := context.Background()
	ctxExisting := synthesizeSkillCommitContext(baseCtx)
	require.NotNil(t, ctxExisting)
	assert.True(t, pkgctx.GetPromoteOnCreate(ctxExisting))
}
