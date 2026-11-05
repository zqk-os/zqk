package system_test

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/system"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/systemcheck"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCheckIncremental_CleanWorkspace(t *testing.T) {
	cmd := system.NewCheckCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--incremental", "--tier", "1"})

	err := cmd.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Incremental mode")
	assert.Contains(t, out, "CAS membrane integrity verified")
	assert.Contains(t, out, "System check complete.")
}

func TestCheckIncremental_StagedOnlyFlag(t *testing.T) {
	cmd := system.NewCheckCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--staged"})

	err := cmd.Execute()
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Incremental mode")
	assert.Contains(t, out, "0 staged objects")
}

func TestCheckIncremental_JSONFormat(t *testing.T) {
	cmd := system.NewCheckCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cmd.SetArgs([]string{"--incremental", "--format", "json"})

	err := cmd.Execute()
	require.NoError(t, err)

	var summary systemcheck.IncrementalCheckSummary
	err = json.Unmarshal(buf.Bytes(), &summary)
	require.NoError(t, err, "output must be valid JSON: %s", buf.String())
	assert.True(t, summary.MembraneClean)
	assert.Equal(t, 0, summary.BlockingIssues)
}

func TestCheckIncremental_DirtyObjectValidation(t *testing.T) {
	tmpDir := t.TempDir()

	runGit := func(args ...string) {
		c := execwrap.Command("git", args...)
		c.Dir = tmpDir
		out, err := c.CombinedOutput()
		require.NoError(t, err, string(out))
	}

	runGit("init")
	runGit("config", "user.email", "test@zqk.dev")
	runGit("config", "user.name", "Test User")

	// Commit initial file
	require.NoError(t, fileutil.WriteFile(filepath.Join(tmpDir, "README.md"), []byte("init\n"), paths.FilePerm644))
	runGit("add", "README.md")
	runGit("commit", "-m", "init")

	// Create a valid CAS object in .zqk/process/backlog_items/
	procDir := filepath.Join(tmpDir, ".zqk/process/backlog_items")
	require.NoError(t, fileutil.EnsureDir(procDir))

	validContent := []byte("id: BLI-101\nkind: backlog_item\ntitle: Valid Item\nstatus: planned\n")
	validHash := storage.CalculateSHA256Hash(validContent)
	validFile := filepath.Join(procDir, validHash+".yaml")
	require.NoError(t, fileutil.WriteFile(validFile, validContent, paths.FilePerm644))

	// Stage the valid object
	runGit("add", ".zqk/process/backlog_items/"+validHash+".yaml")

	dirty, _, err := systemcheck.GetDirtyProcessFiles(tmpDir, true)
	require.NoError(t, err)
	require.Len(t, dirty, 1)
	assert.Equal(t, "BLI-101", dirty[0].ObjectID)

	memResults := systemcheck.VerifyDirtyCASMembrane(tmpDir, dirty, nil)
	assert.Empty(t, memResults, "valid object should have 0 membrane violations")

	// Now introduce a CAS Membrane Violation (conceptual status in .zqk/process/)
	badContent := []byte("id: BLI-102\nkind: backlog_item\ntitle: Bad Item\nstatus: conceptual\n")
	badHash := storage.CalculateSHA256Hash(badContent)
	badFile := filepath.Join(procDir, badHash+".yaml")
	require.NoError(t, fileutil.WriteFile(badFile, badContent, paths.FilePerm644))
	runGit("add", ".zqk/process/backlog_items/"+badHash+".yaml")

	dirtyBad, _, err := systemcheck.GetDirtyProcessFiles(tmpDir, true)
	require.NoError(t, err)
	require.Len(t, dirtyBad, 2)

	memResultsBad := systemcheck.VerifyDirtyCASMembrane(tmpDir, dirtyBad, nil)
	require.NotEmpty(t, memResultsBad)
	foundConceptual := false
	for _, r := range memResultsBad {
		for _, issue := range r.Issues {
			if strings.Contains(issue.Message, "Preliminary status (conceptual) detected") {
				foundConceptual = true
			}
		}
	}
	assert.True(t, foundConceptual, "should catch preliminary status violation in .zqk/process/")
}
