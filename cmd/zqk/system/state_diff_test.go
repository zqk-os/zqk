package system

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func writeTestCSnap(t *testing.T, dir, filename string, objs []map[string]any) string {
	t.Helper()
	logger := logging.GetLoggerFromProfile("debug")
	now := time.Now().UTC()
	cs, err := storage.CreateCompressedSnapshot(objs, now, logger)
	require.NoError(t, err)

	targetPath := filepath.Join(dir, filename)
	err = storage.WriteCompressedSnapshot(cs, targetPath)
	require.NoError(t, err)
	return targetPath
}

func TestNewStateDiffCmd_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	cmd := NewStateDiffCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "state-diff <snapshot-a> [snapshot-b]", cmd.Use)
	assert.NotNil(t, cmd.RunE)
	assert.NotNil(t, cmd.Flags().Lookup("brief"))
}

func TestStateDiff_NegativeAdversarialBoundary_InvalidSnapshots(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	// Case 1: Non-existent snapshot A
	cmd := NewStateDiffCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{filepath.Join(tmpDir, "non_existent_a.csnap")})

	err := cmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read snapshot A")

	// Case 2: Valid snapshot A, non-existent snapshot B
	validA := writeTestCSnap(t, tmpDir, "valid_a.csnap", []map[string]any{
		{"id": "GOAL-1", "kind": "goal", "title": "Goal 1"},
	})

	cmd2 := NewStateDiffCmd()
	cmd2.SilenceErrors = true
	cmd2.SilenceUsage = true
	var buf2 bytes.Buffer
	cmd2.SetOut(&buf2)
	cmd2.SetErr(&buf2)
	cmd2.SetArgs([]string{validA, filepath.Join(tmpDir, "non_existent_b.csnap")})

	err2 := cmd2.Execute()
	require.Error(t, err2)
	assert.Contains(t, err2.Error(), "failed to read snapshot B")

	// Case 3: Corrupted snapshot file
	corruptFile := filepath.Join(tmpDir, "corrupted.csnap")
	require.NoError(t, fileutil.WriteFile(corruptFile, []byte("NOT_A_VALID_GZIP_OR_CSNAP"), paths.FilePerm644))

	cmd3 := NewStateDiffCmd()
	cmd3.SilenceErrors = true
	cmd3.SilenceUsage = true
	var buf3 bytes.Buffer
	cmd3.SetOut(&buf3)
	cmd3.SetErr(&buf3)
	cmd3.SetArgs([]string{corruptFile})

	err3 := cmd3.Execute()
	require.Error(t, err3)
	assert.Contains(t, err3.Error(), "failed to read snapshot A")
}

func TestStateDiff_OperationalDynamicProof_TwoSnapshots(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	objsA := []map[string]any{
		{"id": "GOAL-101", "kind": "goal", "title": "Goal A", "status": "open"},
		{"id": "REQ-201", "kind": "requirement", "title": "Requirement B", "status": "draft"},
	}
	objsB := []map[string]any{
		{"id": "REQ-201", "kind": "requirement", "title": "Requirement B", "status": "complete"}, // Modified
		{"id": "BLI-301", "kind": "backlog_item", "title": "BLI C", "status": "planned"},         // Added
		// GOAL-101 is Removed
	}

	snapA := writeTestCSnap(t, tmpDir, "snapshot_a.csnap", objsA)
	snapB := writeTestCSnap(t, tmpDir, "snapshot_b.csnap", objsB)

	cmd := NewStateDiffCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{snapA, snapB})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Summary: 1 added, 1 removed, 1 modified")
	assert.Contains(t, output, "[Added Objects]")
	assert.Contains(t, output, "+ BLI-301 (backlog_item)")
	assert.Contains(t, output, "[Removed Objects]")
	assert.Contains(t, output, "- GOAL-101 (goal)")
	assert.Contains(t, output, "[Modified Objects]")
	assert.Contains(t, output, "~ REQ-201 (requirement)")
}

func TestStateDiff_OperationalDynamicProof_BriefFlag(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	objsA := []map[string]any{
		{"id": "GOAL-101", "kind": "goal", "title": "Goal A"},
	}
	objsB := []map[string]any{
		{"id": "BLI-301", "kind": "backlog_item", "title": "BLI C"},
	}

	snapA := writeTestCSnap(t, tmpDir, "snap_a.csnap", objsA)
	snapB := writeTestCSnap(t, tmpDir, "snap_b.csnap", objsB)

	cmd := NewStateDiffCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--brief", snapA, snapB})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Summary: 1 added, 1 removed, 0 modified")
	assert.NotContains(t, output, "[Added Objects]")
	assert.NotContains(t, output, "[Removed Objects]")
}

func TestStateDiff_OperationalDynamicProof_IdenticalSnapshots(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	objs := []map[string]any{
		{"id": "GOAL-101", "kind": "goal", "title": "Goal A", "status": "active"},
		{"id": "REQ-201", "kind": "requirement", "title": "Req B", "status": "active"},
	}

	snap := writeTestCSnap(t, tmpDir, "identical.csnap", objs)

	cmd := NewStateDiffCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{snap, snap})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Summary: 0 added, 0 removed, 0 modified")
	assert.NotContains(t, output, "[Added Objects]")
	assert.NotContains(t, output, "[Removed Objects]")
	assert.NotContains(t, output, "[Modified Objects]")
}

func TestStateDiff_OperationalDynamicProof_LiveSystemState(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)

	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProcessDir), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProjectDataDir), paths.DirPerm755))

	objsA := []map[string]any{
		{"id": "GOAL-101", "kind": "goal", "title": "Goal 101"},
	}
	snapA := writeTestCSnap(t, tmpDir, "snapshot_live_cmp.csnap", objsA)

	cmd := NewStateDiffCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(tmpDir))
	cmd.SetArgs([]string{snapA})

	err := cmd.Execute()
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "Extracting live system state for comparison...")
	assert.Contains(t, output, "=== Diff Results ===")
	assert.Contains(t, output, "Summary:")
}
