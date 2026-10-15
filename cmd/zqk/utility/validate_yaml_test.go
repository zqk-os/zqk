package utility

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/testkit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestNewValidateYAMLCmd(t *testing.T) {
	cmd := NewValidateYAMLCmd()
	require.NotNil(t, cmd)
	assert.Equal(t, "validate-yaml [file...]", cmd.Use)

	assert.NotNil(t, cmd.Flags().Lookup("recursive"))
	assert.NotNil(t, cmd.Flags().Lookup("quiet"))
	assert.NotNil(t, cmd.Flags().Lookup("register-hashes"))
}

func TestValidateYAML_DirectFunctions(t *testing.T) {
	tmpDir := t.TempDir()

	validFile := filepath.Join(tmpDir, "valid.yaml")
	require.NoError(t, fileutil.WriteFile(validFile, []byte("title: Valid\ncount: 42\n"), paths.FilePerm644))

	invalidFile := filepath.Join(tmpDir, "invalid.yaml")
	require.NoError(t, fileutil.WriteFile(invalidFile, []byte("title: [unclosed list\n"), paths.FilePerm644))

	nonYamlFile := filepath.Join(tmpDir, "notes.txt")
	require.NoError(t, fileutil.WriteFile(nonYamlFile, []byte("just text"), paths.FilePerm644))

	subDir := filepath.Join(tmpDir, "subdir")
	require.NoError(t, fileutil.MkdirAll(subDir, paths.DirPerm755))
	nestedFile := filepath.Join(subDir, "nested.yml")
	require.NoError(t, fileutil.WriteFile(nestedFile, []byte("nested: true\n"), paths.FilePerm644))

	// 1. validateYAMLFile
	assert.NoError(t, validateYAMLFile(validFile))
	assert.Error(t, validateYAMLFile(invalidFile))
	assert.Error(t, validateYAMLFile(filepath.Join(tmpDir, "non_existent.yaml")))

	// 2. collectYAMLFiles - single files
	files, errs := collectYAMLFiles([]string{validFile, nonYamlFile}, false, false)
	assert.Equal(t, 2, len(files))
	assert.True(t, len(errs) > 0, "should warn about non-yaml extension")

	// 2b. collectYAMLFiles - directory non-recursive
	files, errs = collectYAMLFiles([]string{tmpDir}, false, false)
	assert.Empty(t, files)
	assert.True(t, len(errs) > 0, "should warn directory needs --recursive")

	// 2c. collectYAMLFiles - directory recursive
	files, errs = collectYAMLFiles([]string{tmpDir}, true, false)
	assert.True(t, len(files) >= 3)
	assert.Empty(t, errs)

	// 2d. collectYAMLFiles - missing file
	files, errs = collectYAMLFiles([]string{filepath.Join(tmpDir, "missing.yaml")}, false, false)
	assert.Empty(t, files)
	assert.Equal(t, 1, len(errs))

	// 3. getRelativePath & findZQKPath
	assert.Equal(t, "valid.yaml", getRelativePath(tmpDir, validFile))
	assert.NotEmpty(t, getRelativePath(tmpDir, "/some/external/file"))
	assert.NotEmpty(t, findZQKPath(tmpDir))

	// Create fake zqk binary in root to test finding it
	fakeZqk := filepath.Join(tmpDir, "zqk")
	require.NoError(t, fileutil.WriteFile(fakeZqk, []byte("#!/bin/sh\nexit 0\n"), paths.FilePerm755))
	assert.Equal(t, fakeZqk, findZQKPath(tmpDir))
}

func TestRunValidateYAML_ValidAndInvalid(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	cmd := NewValidateYAMLCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)

	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(cmd.Context(), &buf)
	cmd.SetContext(ctx)

	validFile := filepath.Join(proj.Root, "test_valid.yaml")
	require.NoError(t, fileutil.WriteFile(validFile, []byte("key: value\nlist:\n  - a\n  - b\n"), paths.FilePerm644))

	// Valid run
	err := runValidateYAML(cmd, []string{validFile}, false, false, false)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "is valid")
	assert.Contains(t, buf.String(), "Validation summary: 1 valid, 0 invalid")

	// Invalid run
	invalidFile := filepath.Join(proj.Root, "test_invalid.yaml")
	require.NoError(t, fileutil.WriteFile(invalidFile, []byte("invalid: yaml: : ["), paths.FilePerm644))

	buf.Reset()
	err = runValidateYAML(cmd, []string{invalidFile}, false, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validation failed")
	assert.Contains(t, buf.String(), "Validation summary: 0 valid, 1 invalid")

	// No files found
	buf.Reset()
	err = runValidateYAML(cmd, []string{filepath.Join(proj.Root, "missing_dir")}, false, false, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no YAML files found to validate")
}

func TestOutputValidationSummary(t *testing.T) {
	cmd := &cobra.Command{}
	var buf bytes.Buffer
	ctx := pkgctx.WithCommandOutputWriter(context.Background(), &buf)
	cmd.SetContext(ctx)

	res := &ValidateYAMLResult{
		ValidCount:     2,
		InvalidCount:   1,
		FixedCount:     1,
		Errors:         []string{"file1.yaml: bad indentation"},
		Quiet:          false,
		RegisterHashes: true,
	}

	outputValidationSummary(cmd, res)
	out := buf.String()
	assert.Contains(t, out, "Validation summary: 2 valid, 1 invalid, 1 hashes registered")
	assert.Contains(t, out, "file1.yaml: bad indentation")
}

func TestRegisterHashViaSystemCheck(t *testing.T) {
	proj := testkit.PrepareIsolatedTempProject(t, &testkit.IsolatedTempProjectOptions{})
	cmd := NewValidateYAMLCmd()
	initTestCmdContext(t, cmd, proj.Root, nil)
	cliCtx := cli.GetContext(cmd)
	require.NotNil(t, cliCtx)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Non-existent file
	err := registerHashViaSystemCheck(cliCtx, logger, filepath.Join(proj.Root, "nope.yaml"), false)
	assert.Error(t, err)

	// Non-yaml content
	badYaml := filepath.Join(proj.Root, "bad.yaml")
	require.NoError(t, fileutil.WriteFile(badYaml, []byte("{bad: yaml: ["), paths.FilePerm644))
	err = registerHashViaSystemCheck(cliCtx, logger, badYaml, false)
	assert.Error(t, err)

	// Valid YAML but no kind -> skip
	noKindYaml := filepath.Join(proj.Root, "no_kind.yaml")
	require.NoError(t, fileutil.WriteFile(noKindYaml, []byte("title: Just a doc\n"), paths.FilePerm644))
	err = registerHashViaSystemCheck(cliCtx, logger, noKindYaml, false)
	assert.NoError(t, err)

	// Valid YAML with kind but no ID -> skip
	noIDYaml := filepath.Join(proj.Root, "no_id.yaml")
	require.NoError(t, fileutil.WriteFile(noIDYaml, []byte("kind: backlog_item\ntitle: Item\n"), paths.FilePerm644))
	err = registerHashViaSystemCheck(cliCtx, logger, noIDYaml, false)
	assert.NoError(t, err)

	// Valid YAML with kind and ID, hash registry has matching hash -> no-op
	kindDir := filepath.Join(proj.Root, "items")
	require.NoError(t, fileutil.MkdirAll(kindDir, paths.DirPerm755))
	itemYaml := filepath.Join(kindDir, "BLI-001.yaml")
	itemContent := []byte("id: BLI-001\nkind: backlog_item\ntitle: Item 1\n")
	require.NoError(t, fileutil.WriteFile(itemYaml, itemContent, paths.FilePerm644))

	hasher := sha256.New()
	hasher.Write(itemContent)
	expectedHash := fmt.Sprintf("%x", hasher.Sum(nil))

	reg := storage.NewHashRegistry(pkgctx.NewSystemContext(), "backlog_item", kindDir)
	reg.SetHash("BLI-001.yaml", expectedHash)
	require.NoError(t, reg.Save())

	err = registerHashViaSystemCheck(cliCtx, logger, itemYaml, false)
	assert.NoError(t, err)

	// Hash mismatch -> should detect tampering
	reg.SetHash("BLI-001.yaml", "0000000000000000000000000000000000000000000000000000000000000000")
	require.NoError(t, reg.Save())

	err = registerHashViaSystemCheck(cliCtx, logger, itemYaml, false)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "hash mismatch detected")

	// Missing hash in registry calls registerHashViaSystemCheckCommand
	missingHashYaml := filepath.Join(kindDir, "BLI-002.yaml")
	require.NoError(t, fileutil.WriteFile(missingHashYaml, []byte("id: BLI-002\nkind: backlog_item\ntitle: Item 2\n"), paths.FilePerm644))
	_ = registerHashViaSystemCheck(cliCtx, logger, missingHashYaml, false)
	_ = registerHashViaSystemCheckCommand(cliCtx, logger, missingHashYaml, "BLI-002", false)
	_ = registerHashViaSystemCheckCommand(cliCtx, logger, missingHashYaml, "BLI-002", true)
}
