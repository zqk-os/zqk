package workflow

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/testkit"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestNewCheckCmd(t *testing.T) {
	cmd := NewCheckCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "check", cmd.Use)
}

func TestRunCheck_InGitRepo(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Setenv(zqkenv.ProjectRoot().Name(), proj.Root)

	runGit := func(args ...string) {
		c := testkit.ManagedCommand(t, t.Context(), "git", args...)
		c.Dir = proj.Root
		out, err := c.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	// Initialize git repo in proj.Root
	runGit("init")
	runGit("config", "user.name", "Tester")
	runGit("config", "user.email", "tester@test.com")
	runGit("commit", "--allow-empty", "-m", "initial commit")

	store := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-CHECK-01": {
				objects.FieldKeyKind:            objects.KindPriorityPlan,
				objects.FieldKeyID:              "PRI-CHECK-01",
				objects.FieldKeyStatus:          objects.ObjectStatusActive,
				objects.FieldKeyBacklogItemRefs: []any{"BLI-CHECK-01"},
			},
		},
	}

	// 1. Run on main branch (should report on main branch violation)
	cmd := NewCheckCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd, proj.Root, store)

	err := runCheck(cmd, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Currently on main branch")

	// 2. Checkout feature branch feat/BLI-CHECK-01
	runGit("checkout", "-b", "feat/BLI-CHECK-01")

	buf.Reset()
	cmd2 := NewCheckCmd()
	cmd2.SetOut(&buf)
	cmd2.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd2, proj.Root, store)

	err = runCheck(cmd2, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Workflow Adherence Score")

	// 3. Checkout branch with item NOT in plan
	runGit("checkout", "-b", "feat/BLI-UNKNOWN")

	buf.Reset()
	cmd3 := NewCheckCmd()
	cmd3.SetOut(&buf)
	cmd3.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd3, proj.Root, store)

	err = runCheck(cmd3, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "not in the active priority plan")

	// 4. Item in plan but not first item
	storeMulti := &mockWhatsNextStorage{
		data: map[string]map[string]any{
			"PRI-CHECK-01": {
				objects.FieldKeyKind:            objects.KindPriorityPlan,
				objects.FieldKeyID:              "PRI-CHECK-01",
				objects.FieldKeyStatus:          objects.ObjectStatusActive,
				objects.FieldKeyBacklogItemRefs: []any{"BLI-FIRST", "BLI-CHECK-01"},
			},
		},
	}
	runGit("checkout", "feat/BLI-CHECK-01")

	buf.Reset()
	cmd4 := NewCheckCmd()
	cmd4.SetOut(&buf)
	cmd4.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	initTestCmdContext(t, cmd4, proj.Root, storeMulti)

	err = runCheck(cmd4, nil)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "not the first non-completed item")
}
