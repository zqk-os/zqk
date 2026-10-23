package utility

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	cli "github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewFixHashesCmd(t *testing.T) {
	cmd := NewFixHashesCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "fix-hashes [path...]", cmd.Use)

	assert.NotNil(t, cmd.Flags().Lookup("recursive"))
	assert.NotNil(t, cmd.Flags().Lookup("force"))
	assert.NotNil(t, cmd.Flags().Lookup("quiet"))
	assert.NotNil(t, cmd.Flags().Lookup("kind"))
	assert.NotNil(t, cmd.Flags().Lookup("internal"))
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))

	// Test Args validator
	assert.Error(t, cmd.Args(cmd, []string{}))
	assert.NoError(t, cmd.Args(cmd, []string{"some/path.yaml"}))
}

func TestRunFixHashes_Flows(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})
	cmd := NewFixHashesCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(cmd.Context(), &buf)
	cmd.SetContext(ctx)

	// Create test object file
	itemsDir := filepath.Join(datacell.ProcessPrimaryDir(proj.Root), objects.GetDirectoryFromKind("backlog_item"))
	require.NoError(t, fileutil.MkdirAll(itemsDir, paths.DirPerm755))
	itemFile := filepath.Join(itemsDir, "BLI-001.yaml")
	itemData := []byte("id: BLI-001\nkind: backlog_item\ntitle: Test Item\n")
	require.NoError(t, fileutil.WriteFile(itemFile, itemData, paths.FilePerm644))

	// 1. Run on specific file with missing hash
	buf.Reset()
	err := runFixHashes(cmd, []string{itemFile}, false, false, false, "", false, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Fixed hash for")

	// 2. Run again (already up to date -> skipped)
	buf.Reset()
	err = runFixHashes(cmd, []string{itemFile}, false, false, false, "", false, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "0 fixed, 1 skipped")

	// 3. Mutate file to cause hash mismatch
	tamperedData := []byte("id: BLI-001\nkind: backlog_item\ntitle: Tampered Item\n")
	require.NoError(t, fileutil.WriteFile(itemFile, tamperedData, paths.FilePerm644))

	// Without force -> should fail
	buf.Reset()
	err = runFixHashes(cmd, []string{itemFile}, false, false, false, "", false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "hash fixing completed with 1 error(s)")

	// With dry-run
	buf.Reset()
	err = runFixHashes(cmd, []string{itemFile}, false, true, false, "", false, true)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Dry run: no changes made")

	// With force (no dry-run) -> should fix and succeed
	buf.Reset()
	err = runFixHashes(cmd, []string{itemFile}, false, true, false, "", false, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Fixed hash for")

	// 4. Run with --kind
	buf.Reset()
	err = runFixHashes(cmd, []string{}, false, false, false, "backlog_item", false, false)
	require.NoError(t, err)

	// 5. Run with --internal
	auditDir := filepath.Join(datacell.ProcessPrimaryDir(proj.Root), objects.GetDirectoryFromKind(objects.KindAuditEvent))
	require.NoError(t, fileutil.MkdirAll(auditDir, paths.DirPerm755))
	auditFile := filepath.Join(auditDir, "AUD-001.yaml")
	require.NoError(t, fileutil.WriteFile(auditFile, []byte("id: AUD-001\nkind: audit_event\ntitle: Audit\n"), paths.FilePerm644))

	buf.Reset()
	err = runFixHashes(cmd, []string{}, false, false, false, "", true, false)
	require.NoError(t, err)

	// 6. No files found
	err = runFixHashes(cmd, []string{filepath.Join(proj.Root, "nonexistent")}, false, false, false, "", false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no YAML files found to process")
}

func TestFixHashesHelpers(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	t.Cleanup(func() {
		cli.UnregisterStorageForProjectRoot(proj.Root)
	})

	// Test outputSummary
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)

	res := &FixHashesResult{
		FixedCount:   2,
		SkippedCount: 1,
		ErrorCount:   1,
		TotalFiles:   4,
		Errors:       []string{"err1"},
		Quiet:        false,
		DryRun:       false,
	}
	outputSummary(cmd, res)
	assert.Contains(t, buf.String(), "Hash fix summary: 2 fixed, 1 skipped, 1 errors (total: 4 objects)")

	// Test shouldSkipFileWalkCandidate
	assert.True(t, shouldSkipFileWalkCandidate("path", nil, assert.AnError))

	// Test validateFixHashesFlags
	flags := &FixHashesFlags{
		Recursive: false,
		Kind:      "backlog_item",
		Internal:  true,
	}
	assert.Error(t, validateFixHashesFlags(flags, []string{}))

	flags.Internal = false
	assert.Error(t, validateFixHashesFlags(flags, []string{"file.yaml"}))

	// Test showProgress
	buf.Reset()
	showProgress(cmd, false, 5, 10)
	showProgress(cmd, true, 5, 10)     // quiet mode
	showProgress(cmd, false, 0, 20)    // total > 10 && current == 0
	showProgress(cmd, false, 100, 200) // total > 100 && current%100 == 0

	// Test collectFilesFromArgs
	files, errs := collectFilesFromArgs([]string{proj.Root}, false, false)
	assert.Empty(t, files)
	assert.NotEmpty(t, errs)

	// Non-yaml file
	nonYaml := filepath.Join(proj.Root, "test.txt")
	require.NoError(t, fileutil.WriteFile(nonYaml, []byte("hello"), paths.FilePerm644))
	_, errsNonYaml := collectFilesFromArgs([]string{nonYaml}, false, false)
	assert.NotEmpty(t, errsNonYaml)

	// Recursive walk finding yaml
	filesRec, errsRec := collectFilesFromArgs([]string{proj.Root}, true, false)
	assert.Empty(t, errsRec)
	assert.NotEmpty(t, filesRec)

	// Nonexistent path
	_, errsMissing := collectFilesFromArgs([]string{filepath.Join(proj.Root, "nonexistent-dir")}, false, false)
	assert.NotEmpty(t, errsMissing)

	// Test processFileForHashFix - unparseable file error path
	invalidYaml := filepath.Join(proj.Root, "invalid.yaml")
	require.NoError(t, fileutil.WriteFile(invalidYaml, []byte(":::invalid: yaml:"), paths.FilePerm644))
	resErr := &FixHashesResult{}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	processFileForHashFix(cmd, context.Background(), invalidYaml, proj.Root, &FixHashesFlags{}, logger, resErr)
	assert.Equal(t, 1, resErr.ErrorCount)

	// Test processFileForHashFix - missing fields skipped path
	missingFieldsYaml := filepath.Join(proj.Root, "missing.yaml")
	require.NoError(t, fileutil.WriteFile(missingFieldsYaml, []byte("title: Missing Fields\n"), paths.FilePerm644))
	resSkip := &FixHashesResult{}
	processFileForHashFix(cmd, context.Background(), missingFieldsYaml, proj.Root, &FixHashesFlags{}, logger, resSkip)
	assert.Equal(t, 1, resSkip.SkippedCount)
}
