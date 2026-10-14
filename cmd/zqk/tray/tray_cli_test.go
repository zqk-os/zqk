package tray

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/paths"
	traypkg "github.com/zqk-os/zqk/pkg/tray"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation/qa"
)

func executeTrayCommandWithContext(t *testing.T, projectRoot string, args ...string) (string, error) {
	t.Helper()
	cmd := NewTrayCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cli.SetContext(cmd, cli.ContextForProjectRoot(projectRoot))
	cmd.SetContext(pkgctx.WithCommandOutputWriter(context.Background(), &buf))
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestTrayListCommand(t *testing.T) {
	tempDir := t.TempDir()

	// 1. List with default embedded entries
	out, err := executeTrayCommandWithContext(t, tempDir, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "intake")

	// 2. List with custom tray.yaml
	entries := []traypkg.Entry{
		{Name: "custom-job", Description: "Run custom job", Argv: []string{"job", "run", "custom"}},
		{Name: "custom-query", Description: "Query graph", Argv: []string{"query", "graph"}},
	}
	customDir := setupTestProjectWithTrayYAML(t, entries)
	out2, err := executeTrayCommandWithContext(t, customDir, "list")
	require.NoError(t, err)
	assert.Contains(t, out2, "custom-job")
	assert.Contains(t, out2, "custom-query")

	// 3. List with JSON format
	out3, err := executeTrayCommandWithContext(t, customDir, "list", "-f", "json")
	require.NoError(t, err)
	assert.Contains(t, out3, `"custom-job"`)
}

func TestTrayShowCommand(t *testing.T) {
	entries := []traypkg.Entry{
		{Name: "scheduler-check", Description: "Check scheduler status", Argv: []string{"scheduler", "status"}},
	}
	customDir := setupTestProjectWithTrayYAML(t, entries)

	// 1. Show existing entry
	out, err := executeTrayCommandWithContext(t, customDir, "show", "scheduler-check")
	require.NoError(t, err)
	assert.Contains(t, out, "scheduler-check")
	assert.Contains(t, out, "Check scheduler status")

	// 2. Show existing entry in JSON format
	outJSON, err := executeTrayCommandWithContext(t, customDir, "show", "scheduler-check", "-f", "json")
	require.NoError(t, err)
	assert.Contains(t, outJSON, `"scheduler-check"`)
	assert.Contains(t, outJSON, `"argv"`)

	// 3. Show non-existent entry returns error
	_, err = executeTrayCommandWithContext(t, customDir, "show", "non-existent-entry")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tray entry")
}

func TestTrayExplainCommand(t *testing.T) {
	entries := []traypkg.Entry{
		{Name: "inspect-system", Description: "Inspect full system", Argv: []string{"system", "check", "--deep"}},
	}
	customDir := setupTestProjectWithTrayYAML(t, entries)

	// 1. Explain existing entry
	out, err := executeTrayCommandWithContext(t, customDir, "explain", "inspect-system")
	require.NoError(t, err)
	assert.Contains(t, out, "system check --deep")

	// 2. Explain in JSON format
	outJSON, err := executeTrayCommandWithContext(t, customDir, "explain", "inspect-system", "-f", "json")
	require.NoError(t, err)
	assert.Contains(t, outJSON, `"command_line"`)

	// 3. Explain unknown entry
	_, err = executeTrayCommandWithContext(t, customDir, "explain", "unknown-entry")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tray entry")
}

func TestTrayRunErrorsAndBoundaries(t *testing.T) {
	entries := []traypkg.Entry{
		{Name: "basic-echo", Description: "Simple run", Argv: []string{"status"}},
		{Name: "restricted-entry", Description: "Privileged run", Argv: []string{"system", "purge"}},
	}
	customDir := setupTestProjectWithTrayYAML(t, entries)

	// 1. Run unknown entry
	_, err := executeTrayCommandWithContext(t, customDir, "run", "no-such-entry", "--dry-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tray entry")

	// 2. Dry run with appended restricted argument rejection
	_, err = executeTrayCommandWithContext(t, customDir, "run", "basic-echo", "--dry-run", "--", "system", "purge")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
	assert.Contains(t, err.Error(), "contain restricted flag/command")

	// 3. Run unsigned privileged entry rejection
	_, err = executeTrayCommandWithContext(t, customDir, "run", "restricted-entry", "--dry-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "access denied")
	assert.Contains(t, err.Error(), "not cryptographically signed")

	// 4. Run privileged entry with missing auditor key
	entriesWithSig := []traypkg.Entry{
		{
			Name:        "signed-privileged",
			Description: "Signed privileged action",
			Argv:        []string{"system", "purge"},
			SignedBy:    "auditor",
			Signature:   "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff",
		},
	}
	missingKeyDir := setupTestProjectWithTrayYAML(t, entriesWithSig)
	_, err = executeTrayCommandWithContext(t, missingKeyDir, "run", "signed-privileged", "--dry-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load auditor key")

	// 5. Run privileged entry with invalid signature
	keyDir := t.TempDir()
	keystoreDir := filepath.Join(keyDir, paths.ProjectDataDir, "keystore")
	require.NoError(t, fileutil.MkdirAll(keystoreDir, paths.DirPerm755))

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pubBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	require.NoError(t, err)
	pubPem := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	require.NoError(t, fileutil.WriteFile(filepath.Join(keystoreDir, "auditor.pub"), pubPem, paths.FilePerm644))

	cfg := traypkg.Config{
		SchemaRef: "https://zqk.dev/schemas/tray_config.schema.json",
		Version:   1,
		Entries:   entriesWithSig,
	}
	cfgBytes, err := yaml.Marshal(&cfg)
	require.NoError(t, err)
	require.NoError(t, fileutil.WriteFile(datacell.TrayYAMLPath(keyDir), cfgBytes, paths.FilePerm644))

	_, err = executeTrayCommandWithContext(t, keyDir, "run", "signed-privileged", "--dry-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "signature verification failed")
}

func TestResolveAuditorPublicKeyVariants(t *testing.T) {
	tempDir := t.TempDir()
	keystoreDir := filepath.Join(tempDir, paths.ProjectDataDir, "keystore")
	require.NoError(t, fileutil.MkdirAll(keystoreDir, paths.DirPerm755))

	// 1. Neither pub nor priv exists
	_, err := resolveAuditorPublicKey(tempDir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "auditor public key not found")

	// 2. auditor.pub with raw hex string (>= 64 chars)
	rawHexKey := strings.Repeat("ab", 64)
	pubPath := filepath.Join(keystoreDir, "auditor.pub")
	require.NoError(t, fileutil.WriteFile(pubPath, []byte(rawHexKey+"\n"), paths.FilePerm644))
	hexKey, err := resolveAuditorPublicKey(tempDir)
	require.NoError(t, err)
	assert.Equal(t, rawHexKey, hexKey)
	_ = fileutil.Remove(pubPath)

	// 3. auditor.pub with PEM PKIX encoded public key
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	pubBytes, err := x509.MarshalPKIXPublicKey(&privKey.PublicKey)
	require.NoError(t, err)
	pemData := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	require.NoError(t, fileutil.WriteFile(pubPath, pemData, paths.FilePerm644))
	hexKey2, err := resolveAuditorPublicKey(tempDir)
	require.NoError(t, err)
	expectedHex := fmt.Sprintf("%064x%064x", privKey.PublicKey.X, privKey.PublicKey.Y)
	assert.Equal(t, expectedHex, hexKey2)
	_ = fileutil.Remove(pubPath)

	// 4. auditor.priv with EC PRIVATE KEY (SEC1)
	privPath := filepath.Join(keystoreDir, "auditor.priv")
	sec1Bytes, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	sec1Pem := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: sec1Bytes})
	require.NoError(t, fileutil.WriteFile(privPath, sec1Pem, paths.FilePerm600))
	hexKey3, err := resolveAuditorPublicKey(tempDir)
	require.NoError(t, err)
	assert.Equal(t, expectedHex, hexKey3)
	_ = fileutil.Remove(privPath)

	// 5. auditor.priv with PKCS8 PRIVATE KEY
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	require.NoError(t, err)
	pkcs8Pem := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8Bytes})
	require.NoError(t, fileutil.WriteFile(privPath, pkcs8Pem, paths.FilePerm600))
	hexKey4, err := resolveAuditorPublicKey(tempDir)
	require.NoError(t, err)
	assert.Equal(t, expectedHex, hexKey4)
	_ = fileutil.Remove(privPath)

	// 6. Corrupted auditor.priv
	require.NoError(t, fileutil.WriteFile(privPath, []byte("NOT A VALID PEM"), paths.FilePerm600))
	_, err = resolveAuditorPublicKey(tempDir)
	require.Error(t, err)
}

func TestTraySignErrorPaths(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Sign when tray.yaml does not exist
	_, err := executeTrayCommandWithContext(t, tempDir, "sign", "my-entry")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not exist")

	// 2. Sign when tray.yaml has corrupt syntax
	require.NoError(t, fileutil.MkdirAll(filepath.Join(tempDir, paths.ProjectDataDir), paths.DirPerm755))
	require.NoError(t, fileutil.WriteFile(datacell.TrayYAMLPath(tempDir), []byte("invalid: [yaml"), paths.FilePerm644))
	_, err = executeTrayCommandWithContext(t, tempDir, "sign", "my-entry")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parse")

	// 3. Sign when entry is not in tray.yaml
	entries := []traypkg.Entry{
		{Name: "existing-entry", Argv: []string{"status"}},
	}
	customDir := setupTestProjectWithTrayYAML(t, entries)
	_, err = executeTrayCommandWithContext(t, customDir, "sign", "missing-entry")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found in")

	// 4. Sign with invalid key path
	_, err = executeTrayCommandWithContext(t, customDir, "sign", "existing-entry", "--key-path", "/no/such/key.priv")
	require.Error(t, err)

	// 5. Successful signing flow with generated auditor key
	keystoreDir := filepath.Join(customDir, paths.ProjectDataDir, "keystore")
	require.NoError(t, fileutil.MkdirAll(keystoreDir, paths.DirPerm755))
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	privBytes, err := x509.MarshalECPrivateKey(privKey)
	require.NoError(t, err)
	privPem := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
	privPath := filepath.Join(keystoreDir, "auditor.priv")
	require.NoError(t, fileutil.WriteFile(privPath, privPem, paths.FilePerm600))

	out, err := executeTrayCommandWithContext(t, customDir, "sign", "existing-entry", "-f", "json")
	require.NoError(t, err)
	assert.Contains(t, out, `"status": "signed"`)
	assert.Contains(t, out, `"name": "existing-entry"`)

	// Verify the matching .pub was created and signature is present in tray.yaml
	pubPath := filepath.Join(keystoreDir, "auditor.pub")
	assert.True(t, fileutil.Exists(pubPath))

	data, err := fileutil.ReadFile(datacell.TrayYAMLPath(customDir))
	require.NoError(t, err)
	var updatedCfg traypkg.Config
	require.NoError(t, yaml.Unmarshal(data, &updatedCfg))
	require.Len(t, updatedCfg.Entries, 1)
	assert.NotEmpty(t, updatedCfg.Entries[0].Signature)
	assert.Equal(t, qa.AuditorAccountID, updatedCfg.Entries[0].SignedBy)

	// Dry run the now-signed entry with privileged argv
	updatedCfg.Entries[0].Argv = []string{"system", "purge"}
	// Resign with the correct argv
	require.NoError(t, traypkg.SignEntry(&updatedCfg.Entries[0], privKey, qa.AuditorAccountID))
	buf, _ := yaml.Marshal(&updatedCfg)
	require.NoError(t, fileutil.WriteFile(datacell.TrayYAMLPath(customDir), buf, paths.FilePerm644))

	runOut, err := executeTrayCommandWithContext(t, customDir, "run", "existing-entry", "--dry-run")
	require.NoError(t, err)
	assert.Contains(t, runOut, "dry_run: true")
}

func TestFindTrayEntryMissing(t *testing.T) {
	cmd := NewTrayCmd()
	cli.SetContext(cmd, cli.ContextForProjectRoot(t.TempDir()))
	_, _, err := findTrayEntry(cmd, "completely-unknown-entry")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown tray entry")
}

func TestTrayRun_ActualSubprocess(t *testing.T) {
	entries := []traypkg.Entry{
		{Name: "help-entry", Description: "Show help", Argv: []string{"--help"}},
	}
	customDir := setupTestProjectWithTrayYAML(t, entries)
	_, err := executeTrayCommandWithContext(t, customDir, "run", "help-entry")
	require.NoError(t, err)
}
