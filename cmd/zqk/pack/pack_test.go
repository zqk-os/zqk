package packcmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPackCLI_InitSealValidate(t *testing.T) {
	tmpDir := t.TempDir()
	packDir := filepath.Join(tmpDir, "my-pack")

	// 1. Test pack init
	initCmd := newInitCmd()
	initBuf := new(bytes.Buffer)
	initCmd.SetOut(initBuf)
	initCmd.SetArgs([]string{"--name", "custom-eval", "--version", "1.2.0", packDir})

	if err := initCmd.Execute(); err != nil {
		t.Fatalf("pack init failed: %v", err)
	}

	if !strings.Contains(initBuf.String(), "Successfully scaffolded") {
		t.Errorf("unexpected output from pack init: %s", initBuf.String())
	}

	manifestPath := filepath.Join(packDir, "swarm.yaml")
	if _, err := fileutil.Stat(manifestPath); err != nil {
		t.Fatalf("swarm.yaml not created: %v", err)
	}

	// 2. Test pack seal
	pubKeyPath := filepath.Join(tmpDir, "pub.key")
	sealCmd := newSealCmd()
	sealBuf := new(bytes.Buffer)
	sealCmd.SetOut(sealBuf)
	sealCmd.SetArgs([]string{"--pubkey-out", pubKeyPath, packDir})

	if err := sealCmd.Execute(); err != nil {
		t.Fatalf("pack seal failed: %v", err)
	}

	if !strings.Contains(sealBuf.String(), "Cryptographically sealed pack") {
		t.Errorf("unexpected output from pack seal: %s", sealBuf.String())
	}

	// 3. Test pack validate
	validateCmd := newValidateCmd()
	valBuf := new(bytes.Buffer)
	validateCmd.SetOut(valBuf)
	validateCmd.SetArgs([]string{"--key", pubKeyPath, packDir})

	if err := validateCmd.Execute(); err != nil {
		t.Fatalf("pack validate failed: %v", err)
	}

	if !strings.Contains(valBuf.String(), "is valid and SEALED") {
		t.Errorf("unexpected output from pack validate: %s", valBuf.String())
	}

	// 4. Adversarial: Tamper a template and test validate fails closed
	tplPath := filepath.Join(packDir, "templates", "eval_prompt.yaml")
	if err := fileutil.WriteFile(tplPath, []byte("prompt: tampered content\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to tamper template: %v", err)
	}

	validateCmdTampered := newValidateCmd()
	validateCmdTampered.SetArgs([]string{"--key", pubKeyPath, packDir})
	if err := validateCmdTampered.Execute(); err == nil {
		t.Fatal("expected pack validate to fail closed on tampered pack, got nil")
	}
}

func TestPackCLI_Manifest(t *testing.T) {
	manifestCmd := newManifestCmd()
	buf := new(bytes.Buffer)
	manifestCmd.SetOut(buf)
	manifestCmd.SetArgs([]string{"--name", "cli-draft-test", "--description", "CLI test swarm", "-o", "-"})

	if err := manifestCmd.Execute(); err != nil {
		t.Fatalf("pack manifest failed: %v", err)
	}

	out := buf.String()
	for _, needle := range []string{
		"name: cli-draft-test",
		"version: 1.0.0",
		"description: CLI test swarm",
		"agents:",
		"tasks:",
	} {
		if !strings.Contains(out, needle) {
			t.Fatalf("pack manifest stdout missing %q:\n%s", needle, out)
		}
	}
}
