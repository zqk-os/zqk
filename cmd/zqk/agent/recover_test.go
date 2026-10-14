package agent

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestRecoverCmd_NoArgs(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "agent.recover.unit",
		ForceRemoveRootOnCleanup: true,
	})
	repoRoot := proj.Root

	cmd := NewRecoverCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cli.SetContext(cmd, cli.ContextForProjectRoot(repoRoot))

	err := cmd.Execute()
	assert.NoError(t, err)
}

func TestRecoverCmd_TooManyArgs(t *testing.T) {
	cmd := NewRecoverCmd()
	cmd.SetArgs([]string{"PRI-1", "PRI-2"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	err := cmd.Execute()
	assert.Error(t, err)
}

func TestRecoverCmd_WithArg(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{
		Kind:                     "agent.recover.unit",
		ForceRemoveRootOnCleanup: true,
	})
	repoRoot := proj.Root

	cmd := NewRecoverCmd()
	cmd.SetArgs([]string{"PRI-1234"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cli.SetContext(cmd, cli.ContextForProjectRoot(repoRoot))

	err := cmd.Execute()
	assert.NoError(t, err)
}
