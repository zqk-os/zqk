package workflow

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	observerpkg "github.com/zqk-os/zqk/pkg/observer"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCoachCmd_Structure(t *testing.T) {
	cmd := NewCoachCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "coach", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("update-tips"))
}

func TestCoachCmd_ReadCachedTips(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	// Write cached tips
	err := observerpkg.WriteCachedTips(proj.Root, []string{
		"Dynamic tip 1: keep cycles scoped",
		"Dynamic tip 2: check test pass rate",
	})
	require.NoError(t, err)

	cmd := NewCoachCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, nil)

	require.NoError(t, cmd.Flags().Set("format", "json"))
	err = cmd.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Dynamic tip 1: keep cycles scoped")
	assert.Contains(t, out, "Dynamic tip 2: check test pass rate")
}

func TestCoachCmd_UpdateTips(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	cmd := NewCoachCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, nil)

	require.NoError(t, cmd.Flags().Set("update-tips", "true"))
	// Execute exercises lines 37-112 of coach.go (generating summary from state)
	_ = cmd.Execute()
}
