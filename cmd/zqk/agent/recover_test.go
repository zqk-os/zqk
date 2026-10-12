package agent

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestRecoverCmd_NoArgs(t *testing.T) {
	cmd := NewRecoverCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	repoRoot := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), repoRoot)
	_ = fileutil.MkdirAll(filepath.Join(repoRoot, paths.ProjectDataDir, paths.StateDir, "cache"), paths.DirPerm755)
	_ = fileutil.MkdirAll(filepath.Join(repoRoot, paths.ProcessDir), paths.DirPerm755)

	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(repoRoot, nil))
		_ = fileutil.RemoveAll(filepath.Join(repoRoot, paths.ProjectDataDir))
	})

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
	cmd := NewRecoverCmd()
	cmd.SetArgs([]string{"PRI-1234"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)

	repoRoot := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), repoRoot)
	_ = fileutil.MkdirAll(filepath.Join(repoRoot, paths.ProjectDataDir, paths.StateDir, "cache"), paths.DirPerm755)
	_ = fileutil.MkdirAll(filepath.Join(repoRoot, paths.ProcessDir), paths.DirPerm755)

	t.Cleanup(func() {
		_ = storage.RunProjectTestTeardown(storage.TempProjectTeardown(repoRoot, nil))
		_ = fileutil.RemoveAll(filepath.Join(repoRoot, paths.ProjectDataDir))
	})

	err := cmd.Execute()
	assert.NoError(t, err)
}
