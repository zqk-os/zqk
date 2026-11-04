package tray

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	traypkg "github.com/zqk-os/zqk/pkg/tray"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func setupTestProjectWithCorruptTrayYAML(t *testing.T) string {
	t.Helper()
	tempDir := t.TempDir()
	zqkDir := filepath.Join(tempDir, paths.ProjectDataDir)
	require.NoError(t, fileutil.MkdirAll(zqkDir, paths.DirPerm755))
	trayPath := datacell.TrayYAMLPath(tempDir)
	require.NoError(t, fileutil.WriteFile(trayPath, []byte("invalid: [yaml: unclosed"), paths.FilePerm644))
	return tempDir
}

func setupTestAuditorKey(t *testing.T, keystoreDir string, filename string) (*ecdsa.PrivateKey, string) {
	t.Helper()
	require.NoError(t, fileutil.MkdirAll(keystoreDir, paths.DirPerm755))
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	privBytes, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	privPem := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
	privPath := filepath.Join(keystoreDir, filename)
	require.NoError(t, fileutil.WriteFile(privPath, privPem, paths.FilePerm600))
	return privKey, privPath
}

func executeCommandWithBuffer(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestResolveRoot_MissingContextAndUnresolvable(t *testing.T) {
	// 1. Missing context returns error
	cmd := NewTrayCmd()
	cli.SetContext(cmd, nil)
	_, err := resolveRoot(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get context")

	// 2. Empty context ProjectRoot and unresolvable from empty temp dir
	emptyDir := t.TempDir()
	t.Setenv(zqkenv.ProjectRoot().Name(), "")
	t.Setenv(zqkenv.TestRoot().Name(), "")
	t.Setenv("PROJECT_ROOT", "")
	t.Setenv("ZQK_PROJECT_ROOT", "")
	t.Setenv("TEST_ROOT", "")
	t.Setenv("ZQK_TEST_ROOT", "")
	t.Chdir(emptyDir)

	cmdWithContext := NewTrayCmd()
	cli.SetContext(cmdWithContext, cli.ContextForProjectRoot("   "))
	_, err = resolveRoot(cmdWithContext)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "project root not found")
}

func TestLoadTrayEntries_CorruptAndErrorPropagation(t *testing.T) {
	corruptDir := setupTestProjectWithCorruptTrayYAML(t)

	// 1. loadTrayEntries error on corrupt YAML
	cmd := NewTrayCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(corruptDir))
	_, _, err := loadTrayEntries(cmd)
	require.Error(t, err)

	// 2. findTrayEntry error propagation
	_, _, err = findTrayEntry(cmd, "any-entry")
	require.Error(t, err)

	// 3. runList error propagation
	err = runList(cmd, nil)
	require.Error(t, err)

	// 4. runShow error propagation
	err = runShow(cmd, []string{"any-entry"})
	require.Error(t, err)

	// 5. runExplain error propagation
	err = runExplain(cmd, []string{"any-entry"})
	require.Error(t, err)
}

func TestLoadTrayEntries_ResolveRootFailure(t *testing.T) {
	cmd := NewTrayCmd()
	cli.SetContext(cmd, nil)
	_, _, err := loadTrayEntries(cmd)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get context")
}

func TestTrayRun_FlagParsingAndProfileLogger(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Flag parsing error when dry-run flag type is invalid
	badFlagCmd := &cobra.Command{Use: "run"}
	badFlagCmd.Flags().Int(cli.FlagDryRun, 1, "invalid type")
	cli.SetContext(badFlagCmd, cli.ContextForProjectRoot(tempDir))
	err := runRun(badFlagCmd, []string{"intake"})
	require.Error(t, err)

	// 2. Valid profile logger coverage
	validCmd := NewTrayCmd()
	ictx := cli.ContextForProjectRoot(tempDir)
	ictx.Profile = "test-profile"
	cli.SetContext(validCmd, ictx)
	out, err := executeCommandWithBuffer(t, validCmd, "run", "intake", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, out, "dry_run: true")
}

func TestTrayRun_PrivilegedMismatchedAuditorKey(t *testing.T) {
	// Generate key A to sign the entry
	keyDirA := t.TempDir()
	privKeyA, privPathA := setupTestAuditorKey(t, filepath.Join(keyDirA, paths.ProjectDataDir, "keystore"), "auditor.priv")
	require.NotEmpty(t, privPathA)

	entry := traypkg.Entry{
		Name:        "secure-entry",
		Description: "Privileged action",
		Argv:        []string{"system", "purge"},
	}
	require.NoError(t, traypkg.SignEntry(&entry, privKeyA, "auditor-a"))

	// Create project with key B in keystore (mismatched)
	projectDir := setupTestProjectWithTrayYAML(t, []traypkg.Entry{entry})
	privKeyB, privPathB := setupTestAuditorKey(t, filepath.Join(projectDir, paths.ProjectDataDir, "keystore"), "auditor.priv")
	require.NotNil(t, privKeyB)
	require.NotEmpty(t, privPathB)

	cmd := NewTrayCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(projectDir))
	_, err := executeCommandWithBuffer(t, cmd, "run", "secure-entry", "--dry-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature verification failed")
}

func TestTrayRun_SubprocessExecutionFailures(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Subprocess non-zero exit code handling
	oldArg0 := os.Args[0]
	t.Cleanup(func() { os.Args[0] = oldArg0 })

	exitProject := setupTestProjectWithTrayYAML(t, []traypkg.Entry{
		{
			Name:        "exit-code-entry",
			Description: "Subprocess exits non-zero",
			Argv:        []string{"-c", "exit 42"},
		},
	})

	os.Args[0] = "/bin/sh"
	cmd := newRunCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(exitProject))
	err := runRun(cmd, []string{"exit-code-entry"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "subprocess exited with code 42")

	// 2. Subprocess non-existent binary execution error
	cmd2 := newRunCmd()
	cli.SetContext(cmd2, cli.ContextForProjectRoot(exitProject))
	os.Args[0] = filepath.Join(tempDir, "nonexistent-binary-path")
	err = runRun(cmd2, []string{"exit-code-entry"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tray run")
}

func TestTraySign_ResolveRootAndManifestErrors(t *testing.T) {
	// 1. ResolveRoot failure when context is missing
	uninitializedCmd := bldr_cli_cmd_v1.NewTraySignCommandBuilder()
	uninitializedCmd.RunE = runSign
	cli.SetContext(uninitializedCmd, nil)
	err := uninitializedCmd.RunE(uninitializedCmd, []string{"any-entry"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get context")

	// 2. Manifest path is an unreadable directory (not a regular file)
	tempDir := t.TempDir()
	zqkDir := filepath.Join(tempDir, paths.ProjectDataDir)
	require.NoError(t, fileutil.MkdirAll(filepath.Join(zqkDir, "tray.yaml"), paths.DirPerm755))

	signCmd := newSignCmd()
	cli.SetContext(signCmd, cli.ContextForProjectRoot(tempDir))
	err = signCmd.RunE(signCmd, []string{"any-entry"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read")
}

func TestTraySign_FlagParsingAndRelativeKeyPath(t *testing.T) {
	entries := []traypkg.Entry{
		{Name: "task-entry", Argv: []string{"system", "purge"}},
	}
	projectRoot := setupTestProjectWithTrayYAML(t, entries)
	keystoreDir := filepath.Join(projectRoot, paths.ProjectDataDir, "keystore")
	privKey, privPath := setupTestAuditorKey(t, keystoreDir, "auditor.priv")
	require.NotNil(t, privKey)
	require.NotEmpty(t, privPath)

	// 1. Flag parsing error when key-path has unexpected type
	badCmd := &cobra.Command{Use: "sign"}
	badCmd.Flags().Int("key-path", 0, "invalid")
	cli.SetContext(badCmd, cli.ContextForProjectRoot(projectRoot))
	err := runSign(badCmd, []string{"task-entry"})
	require.Error(t, err)

	// 2. Relative key path resolution (!filepath.IsAbs(keyPath))
	relKeyPath := filepath.Join(paths.ProjectDataDir, "keystore", "auditor.priv")
	signCmd := newSignCmd()
	cli.SetContext(signCmd, cli.ContextForProjectRoot(projectRoot))
	require.NoError(t, signCmd.Flags().Set("key-path", relKeyPath))

	// Remove auditor.pub if present to verify public key persistence
	pubPath := filepath.Join(keystoreDir, "auditor.pub")
	if fileutil.Exists(pubPath) {
		require.NoError(t, fileutil.Remove(pubPath))
	}

	// Custom security context for accountID
	signCmd.SetContext(pkgctx.WithSecurityContext(context.Background(), &pkgctx.SecurityContext{
		AccountID: "custom-auditor-id-001",
	}))

	err = signCmd.RunE(signCmd, []string{"task-entry"})
	require.NoError(t, err)

	// Verify public key was generated alongside private key
	assert.True(t, fileutil.Exists(pubPath))

	// Verify signed_by reflects custom accountID
	loaded, err := traypkg.Load(projectRoot)
	require.NoError(t, err)
	entry := traypkg.Find(loaded, "task-entry")
	require.NotNil(t, entry)
	assert.Equal(t, "custom-auditor-id-001", entry.SignedBy)
	assert.NotEmpty(t, entry.Signature)
}

func TestTraySign_ManifestWriteFailure(t *testing.T) {
	entries := []traypkg.Entry{
		{Name: "task-entry", Argv: []string{"system", "purge"}},
	}
	projectRoot := setupTestProjectWithTrayYAML(t, entries)
	keystoreDir := filepath.Join(projectRoot, paths.ProjectDataDir, "keystore")
	privKey, privPath := setupTestAuditorKey(t, keystoreDir, "auditor.priv")
	require.NotNil(t, privKey)
	require.NotEmpty(t, privPath)

	signCmd := newSignCmd()
	cli.SetContext(signCmd, cli.ContextForProjectRoot(projectRoot))

	// Make the project directory read-only so rewriting tray.yaml fails
	trayPath := datacell.TrayYAMLPath(projectRoot)
	require.NoError(t, os.Chmod(trayPath, 0444))
	t.Cleanup(func() {
		require.NoError(t, os.Chmod(trayPath, 0644))
	})

	err := signCmd.RunE(signCmd, []string{"task-entry"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "write")
}

func TestTraySign_PersistPublicKeyFailure(t *testing.T) {
	entries := []traypkg.Entry{
		{Name: "task-entry", Argv: []string{"system", "purge"}},
	}
	projectRoot := setupTestProjectWithTrayYAML(t, entries)
	roDir := filepath.Join(projectRoot, "ro-keystore")
	privKey, privPath := setupTestAuditorKey(t, roDir, "auditor.priv")
	require.NotNil(t, privKey)
	require.NotEmpty(t, privPath)

	require.NoError(t, os.Chmod(roDir, 0555))
	t.Cleanup(func() {
		require.NoError(t, os.Chmod(roDir, 0755))
	})

	signCmd := newSignCmd()
	cli.SetContext(signCmd, cli.ContextForProjectRoot(projectRoot))
	require.NoError(t, signCmd.Flags().Set("key-path", privPath))

	err := signCmd.RunE(signCmd, []string{"task-entry"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persist public key")
}
