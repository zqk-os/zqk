package system

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupSyncCASProject(t *testing.T) string {
	t.Helper()
	t.Parallel()
	tmpDir := t.TempDir()
	testkit.RegisterTempProjectTeardown(t, tmpDir, nil)
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tmpDir, paths.ProcessBacklogDir), paths.DirPerm755))
	return tmpDir
}

func createTestCASFile(t *testing.T, dir, id string) (string, string) {
	t.Helper()
	content := []byte(fmt.Sprintf("id: %s\nkind: backlog_item\ntitle: Item %s\n", id, id))
	hash := sha256.Sum256(content)
	hashHex := hex.EncodeToString(hash[:])
	filePath := filepath.Join(dir, paths.ProcessBacklogDir, fmt.Sprintf("%s.yaml", hashHex))
	require.NoError(t, fileutil.WriteFile(filePath, content, paths.FilePerm644))
	return filePath, hashHex
}

func executeSyncCASCmd(projectRoot string, args ...string) (string, error) {
	cmd := NewSyncCASIndexCmd()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)
	if projectRoot != "" {
		cli.SetContext(cmd, cli.ContextForProjectRoot(projectRoot))
	}
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestNewSyncCASIndexCmd_StaticFloorInvariant(t *testing.T) {
	t.Parallel()

	cmd := NewSyncCASIndexCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "sync-cas-index", cmd.Use)
	assert.NotNil(t, cmd.Flags().Lookup("file"))
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
}

func TestSyncCASIndex_NegativeAdversarialBoundary_Errors(t *testing.T) {
	tmpDir := setupSyncCASProject(t)

	// Case 1: Missing --file flag
	_, errNoFile := executeSyncCASCmd(tmpDir)
	require.Error(t, errNoFile)
	assert.Contains(t, errNoFile.Error(), "--file is required")

	// Case 2: File does not exist
	missingFile := filepath.Join(tmpDir, "missing.yaml")
	_, errMissing := executeSyncCASCmd(tmpDir, "--file", missingFile)
	require.Error(t, errMissing)

	// Case 3: Invalid YAML
	badYamlFile := filepath.Join(tmpDir, paths.ProcessBacklogDir, "bad.yaml")
	require.NoError(t, fileutil.WriteFile(badYamlFile, []byte("key: 'unclosed quote"), paths.FilePerm644))
	_, errBadYaml := executeSyncCASCmd(tmpDir, "--file", badYamlFile)
	require.Error(t, errBadYaml)
	assert.Contains(t, errBadYaml.Error(), "parse YAML")

	// Case 4: Missing ID field
	noIDFile := filepath.Join(tmpDir, paths.ProcessBacklogDir, "noid.yaml")
	require.NoError(t, fileutil.WriteFile(noIDFile, []byte("title: No ID Object\nkind: backlog_item\n"), paths.FilePerm644))
	_, errNoID := executeSyncCASCmd(tmpDir, "--file", noIDFile)
	require.Error(t, errNoID)
	assert.Contains(t, errNoID.Error(), "has no id field")

	// Case 5: Unknown kind directory
	unknownDir := filepath.Join(tmpDir, paths.ProcessDir, "unknown_kind_folder")
	require.NoError(t, fileutil.MkdirAll(unknownDir, paths.DirPerm755))
	unknownKindFile := filepath.Join(unknownDir, "test.yaml")
	require.NoError(t, fileutil.WriteFile(unknownKindFile, []byte("id: UNK-1\ntitle: test\n"), paths.FilePerm644))
	_, errUnknownKind := executeSyncCASCmd(tmpDir, "--file", unknownKindFile)
	require.Error(t, errUnknownKind)
	assert.Contains(t, errUnknownKind.Error(), "unknown kind")
}

func TestSyncCASIndex_OperationalDynamicProof_DryRun(t *testing.T) {
	tmpDir := setupSyncCASProject(t)

	filePath, hashHex := createTestCASFile(t, tmpDir, "BLI-SYNC-DRY")
	assert.NotEmpty(t, hashHex)

	out, err := executeSyncCASCmd(tmpDir, "--file", filePath, "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "DRY RUN: would set CAS index")
	assert.Contains(t, out, "BLI-SYNC-DRY")
	assert.Contains(t, out, "backlog_item")
}

func TestSyncCASIndex_OperationalDynamicProof_SyncSuccess(t *testing.T) {
	tmpDir := setupSyncCASProject(t)

	filePath, hashHex := createTestCASFile(t, tmpDir, "BLI-SYNC-REAL")

	out, err := executeSyncCASCmd(tmpDir, "--file", filePath)
	require.NoError(t, err)
	assert.Contains(t, out, "Synced CAS index for BLI-SYNC-REAL (kind: backlog_item)")

	// Verify that the index was written on disk
	indexFile := filepath.Join(filepath.Dir(filePath), ".backlog_item.index")
	indexBytes, err := fileutil.ReadFile(indexFile)
	require.NoError(t, err)
	assert.Contains(t, string(indexBytes), "BLI-SYNC-REAL")
	assert.Contains(t, string(indexBytes), hashHex)
}
