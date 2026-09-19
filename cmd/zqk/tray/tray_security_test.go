package tray

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	traypkg "github.com/zqk-os/zqk/pkg/tray"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func setupTestProjectWithTrayYAML(t *testing.T, entries []traypkg.Entry) string {
	t.Helper()
	tmpDir := t.TempDir()
	zqkDir := filepath.Join(tmpDir, ".zqk")
	if err := os.MkdirAll(zqkDir, 0o755); err != nil {
		t.Fatalf("failed to create .zqk dir: %v", err)
	}

	cfg := traypkg.Config{
		SchemaRef: "https://zqk.dev/schemas/tray_config.schema.json",
		Version:   1,
		Entries:   entries,
	}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&cfg); err != nil {
		t.Fatalf("failed to encode tray config: %v", err)
	}

	trayPath := datacell.TrayYAMLPath(tmpDir)
	if err := fileutil.WriteFile(trayPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("failed to write tray.yaml: %v", err)
	}

	return tmpDir
}

func executeTrayCmd(projectRoot string, args ...string) (string, error) {
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

func TestIndirectExecutionBlocksOverrideWithoutSignature(t *testing.T) {
	entries := []traypkg.Entry{
		{
			Name:        "dangerous-override",
			Description: "Run command with override flag",
			Argv:        []string{"object", "promote", "--override", "--reason-code", "bypass reasons here"},
		},
	}
	projectRoot := setupTestProjectWithTrayYAML(t, entries)

	// Run dangerous entry without signature
	out, err := executeTrayCmd(projectRoot, "run", "dangerous-override", "--dry-run")
	if err == nil {
		t.Fatalf("expected access denied error, got success. Output:\n%s", out)
	}

	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("expected error to contain 'access denied', got: %v", err)
	}
	if !strings.Contains(err.Error(), "--override") {
		t.Errorf("expected error to mention '--override', got: %v", err)
	}
	if !strings.Contains(err.Error(), "not cryptographically signed") {
		t.Errorf("expected error to mention 'not cryptographically signed', got: %v", err)
	}
}

func TestSigningAndRunningEntry_Success(t *testing.T) {
	entries := []traypkg.Entry{
		{
			Name:        "purge-cache-cmd",
			Description: "Hard purge cache with force",
			Argv:        []string{"cache", "clear", "--force"},
		},
	}
	projectRoot := setupTestProjectWithTrayYAML(t, entries)

	// First verify running without sign fails
	_, err := executeTrayCmd(projectRoot, "run", "purge-cache-cmd", "--dry-run")
	if err == nil {
		t.Fatal("expected failure before signing, got success")
	}

	// Sign the entry
	signOut, err := executeTrayCmd(projectRoot, "sign", "purge-cache-cmd")
	if err != nil {
		t.Fatalf("tray sign failed: %v\nOutput: %s", err, signOut)
	}

	if !strings.Contains(signOut, "signed") {
		t.Errorf("expected sign output to contain 'signed', got: %s", signOut)
	}

	// Verify tray.yaml now has signature and signed_by
	loadedEntries, err := traypkg.Load(projectRoot)
	if err != nil {
		t.Fatalf("failed to load tray manifest: %v", err)
	}
	e := traypkg.Find(loadedEntries, "purge-cache-cmd")
	if e == nil {
		t.Fatal("entry purge-cache-cmd not found after signing")
	}
	if e.Signature == "" {
		t.Fatal("entry signature is empty after signing")
	}
	if e.SignedBy == "" {
		t.Fatal("entry signed_by is empty after signing")
	}

	// Running with dry-run should now succeed
	runOut, err := executeTrayCmd(projectRoot, "run", "purge-cache-cmd", "--dry-run")
	if err != nil {
		t.Fatalf("tray run failed for signed entry: %v\nOutput: %s", err, runOut)
	}

	if !strings.Contains(runOut, "dry_run: true") {
		t.Errorf("expected dry_run output, got: %s", runOut)
	}
}

func TestModifyingSignedEntry_FailsClosed(t *testing.T) {
	entries := []traypkg.Entry{
		{
			Name:        "secure-cmd",
			Description: "Secure command",
			Argv:        []string{"cache", "clear", "--force"},
		},
	}
	projectRoot := setupTestProjectWithTrayYAML(t, entries)

	// Sign the entry
	_, err := executeTrayCmd(projectRoot, "sign", "secure-cmd")
	if err != nil {
		t.Fatalf("tray sign failed: %v", err)
	}

	// Now tamper with the entry in .zqk/tray.yaml (change argv from cache clear to object delete)
	trayPath := datacell.TrayYAMLPath(projectRoot)
	data, err := fileutil.ReadFile(trayPath)
	if err != nil {
		t.Fatalf("read %s: %v", trayPath, err)
	}

	var cfg traypkg.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal %s: %v", trayPath, err)
	}

	// Tamper argv while keeping original signature
	cfg.Entries[0].Argv = []string{"object", "delete", "policy", "--all", "--force"}

	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&cfg); err != nil {
		t.Fatalf("encode tampered config: %v", err)
	}
	if err := fileutil.WriteFile(trayPath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write tampered %s: %v", trayPath, err)
	}

	// Run tampered entry; must fail closed!
	runOut, err := executeTrayCmd(projectRoot, "run", "secure-cmd", "--dry-run")
	if err == nil {
		t.Fatalf("expected verification failure for tampered entry, got success. Output:\n%s", runOut)
	}

	if !strings.Contains(err.Error(), "signature verification failed") {
		t.Errorf("expected error to contain 'signature verification failed', got: %v", err)
	}
}
