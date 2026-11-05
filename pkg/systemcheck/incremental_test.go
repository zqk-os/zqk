package systemcheck_test

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestIsProcessYAMLFile(t *testing.T) {
	assert.True(t, systemcheck.IsProcessYAMLFile(".zqk/process/backlog_items/abc.yaml"))
	assert.True(t, systemcheck.IsProcessYAMLFile(".zqk/process/goals/123.yml"))
	assert.False(t, systemcheck.IsProcessYAMLFile(".zqk/process/.internal/cache.yaml"))
	assert.False(t, systemcheck.IsProcessYAMLFile(".zqk/process/backlog_items/abc.txt"))
	assert.False(t, systemcheck.IsProcessYAMLFile("cmd/zqk/main.go"))
	assert.False(t, systemcheck.IsProcessYAMLFile(".zqk/state/session.lock"))
}

func TestInferKindFromProcessPath(t *testing.T) {
	assert.Equal(t, "backlog_item", systemcheck.InferKindFromProcessPath(".zqk/process/backlog_items/abc.yaml"))
	assert.Equal(t, "goal", systemcheck.InferKindFromProcessPath(".zqk/process/goals/abc.yaml"))
	assert.Equal(t, "requirement", systemcheck.InferKindFromProcessPath(".zqk/process/requirements/abc.yaml"))
	assert.Equal(t, "custom_kind", systemcheck.InferKindFromProcessPath(".zqk/process/custom_kind/abc.yaml"))
}

func TestVerifyMembraneInvariants(t *testing.T) {
	// Running on the current repo root should pass without error
	issues, err := systemcheck.VerifyMembraneInvariants(".")
	require.NoError(t, err)
	for _, issue := range issues {
		assert.NotEqual(t, "CAS Membrane coverage gap", issue.Message)
	}
}

func TestVerifyDirtyCASMembrane_HashMismatch(t *testing.T) {
	content := []byte("id: BLI-TEST-001\nkind: backlog_item\ntitle: Test Item\nstatus: planned\n")
	fakeHash := "0000000000000000000000000000000000000000000000000000000000000000"

	dirty := []systemcheck.DirtyProcessFile{
		{
			Path:     filepath.Join(".zqk/process/backlog_items", fakeHash+".yaml"),
			Kind:     "backlog_item",
			ObjectID: "BLI-TEST-001",
			Status:   "planned",
			Content:  content,
		},
	}

	results := systemcheck.VerifyDirtyCASMembrane(".", dirty, nil)
	require.Len(t, results, 1)
	require.Len(t, results[0].Issues, 1)
	assert.Equal(t, 1, results[0].Issues[0].Tier)
	assert.Equal(t, "integrity", results[0].Issues[0].Category)
	assert.Contains(t, results[0].Issues[0].Message, "CAS Membrane Violation: Hash mismatch")
}

func TestVerifyDirtyCASMembrane_ConceptualProhibited(t *testing.T) {
	content := []byte("id: BLI-TEST-002\nkind: backlog_item\ntitle: Test Item\nstatus: conceptual\n")
	actualHash := storage.CalculateSHA256Hash(content)

	dirty := []systemcheck.DirtyProcessFile{
		{
			Path:     filepath.Join(".zqk/process/backlog_items", actualHash+".yaml"),
			Kind:     "backlog_item",
			ObjectID: "BLI-TEST-002",
			Status:   "conceptual",
			Content:  content,
		},
	}

	results := systemcheck.VerifyDirtyCASMembrane(".", dirty, nil)
	require.Len(t, results, 1)
	require.Len(t, results[0].Issues, 1)
	assert.Equal(t, 1, results[0].Issues[0].Tier)
	assert.Equal(t, "integrity", results[0].Issues[0].Category)
	assert.Contains(t, results[0].Issues[0].Message, "Preliminary status (conceptual) detected")
}

func TestVerifyDirtyCASMembrane_Valid(t *testing.T) {
	content := []byte("id: BLI-TEST-003\nkind: backlog_item\ntitle: Test Item\nstatus: planned\n")
	actualHash := storage.CalculateSHA256Hash(content)

	dirty := []systemcheck.DirtyProcessFile{
		{
			Path:     filepath.Join(".zqk/process/backlog_items", actualHash+".yaml"),
			Kind:     "backlog_item",
			ObjectID: "BLI-TEST-003",
			Status:   "planned",
			Content:  content,
		},
	}

	results := systemcheck.VerifyDirtyCASMembrane(".", dirty, nil)
	assert.Empty(t, results)
}

func TestGetDirtyProcessFiles_GitIntegration(t *testing.T) {
	tmpDir := t.TempDir()

	// Initialize git repo in tmpDir
	runGit := func(args ...string) {
		cmd := execwrap.Command("git", args...)
		cmd.Dir = tmpDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	runGit("init")
	runGit("config", "user.email", "test@zqk.dev")
	runGit("config", "user.name", "Test User")

	// Create initial file & commit
	initialFile := filepath.Join(tmpDir, "README.md")
	require.NoError(t, fileutil.WriteFile(initialFile, []byte("# Test Repo\n"), paths.FilePerm644))
	runGit("add", "README.md")
	runGit("commit", "-m", "Initial commit")

	// Initially no dirty files under .zqk/process
	dirty, deleted, err := systemcheck.GetDirtyProcessFiles(tmpDir, false)
	require.NoError(t, err)
	assert.Empty(t, dirty)
	assert.Empty(t, deleted)

	// Create a new file under .zqk/process/backlog_items/
	procDir := filepath.Join(tmpDir, ".zqk/process/backlog_items")
	require.NoError(t, fileutil.EnsureDir(procDir))

	content := []byte("id: BLI-101\nkind: backlog_item\ntitle: Dirty Item\nstatus: planned\n")
	hash := storage.CalculateSHA256Hash(content)
	filePath := filepath.Join(procDir, hash+".yaml")
	require.NoError(t, fileutil.WriteFile(filePath, content, paths.FilePerm644))

	// Before staging: unstaged/untracked dirty file
	dirty, _, err = systemcheck.GetDirtyProcessFiles(tmpDir, false)
	require.NoError(t, err)
	require.Len(t, dirty, 1)
	assert.Equal(t, "BLI-101", dirty[0].ObjectID)
	assert.Equal(t, "backlog_item", dirty[0].Kind)
	assert.False(t, dirty[0].Staged)

	// stagedOnly=true should NOT see unstaged file
	dirtyStaged, _, err := systemcheck.GetDirtyProcessFiles(tmpDir, true)
	require.NoError(t, err)
	assert.Empty(t, dirtyStaged)

	// Now stage the file
	runGit("add", ".zqk/process/backlog_items/"+hash+".yaml")

	// stagedOnly=true should see it now
	dirtyStaged, _, err = systemcheck.GetDirtyProcessFiles(tmpDir, true)
	require.NoError(t, err)
	require.Len(t, dirtyStaged, 1)
	assert.Equal(t, "BLI-101", dirtyStaged[0].ObjectID)
	assert.True(t, dirtyStaged[0].Staged)
}
