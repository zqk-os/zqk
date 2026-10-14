package packcmd

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestPackCmd_Hierarchy(t *testing.T) {
	cmd := NewPackCmd()
	require.NotNil(t, cmd)
	require.Equal(t, "pack", cmd.Use)

	subcommands := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		subcommands[sub.Name()] = true
	}
	require.True(t, subcommands["init"], "init subcommand missing")
	require.True(t, subcommands["seal"], "seal subcommand missing")
	require.True(t, subcommands["validate"], "validate subcommand missing")
	require.True(t, subcommands["manifest"], "manifest subcommand missing")

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())
	require.Contains(t, buf.String(), "pack")
}

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

func TestPackInit_ValidationAndErrors(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Invalid semver
	initCmd := newInitCmd()
	initCmd.SetArgs([]string{"--version", "not-a-valid-semver", filepath.Join(tmpDir, "p1")})
	err := initCmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid package version")

	// 2. Default name when name is empty and targetDir is '.'
	packSubDir := filepath.Join(tmpDir, "default-named-pack")
	require.NoError(t, fileutil.MkdirAll(packSubDir, paths.DirPerm755))
	t.Chdir(packSubDir)

	initCmd2 := newInitCmd()
	initBuf2 := new(bytes.Buffer)
	initCmd2.SetOut(initBuf2)
	initCmd2.SetArgs([]string{"."})
	require.NoError(t, initCmd2.Execute())
	require.Contains(t, initBuf2.String(), "custom-swarm-pack")

	// 3. Collision: already existing swarm.yaml
	initCmd3 := newInitCmd()
	initCmd3.SetArgs([]string{"."})
	err = initCmd3.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")

	// 4. Invalid target directory (ValidateSafePath flags injection)
	initCmd4 := newInitCmd()
	initCmd4.SetArgs([]string{"--name", "flag-inj", "./-unsafe-path"})
	err = initCmd4.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid target directory")
}

func TestPackSeal_KeyOptionsAndErrors(t *testing.T) {
	tmpDir := t.TempDir()
	packDir := filepath.Join(tmpDir, "seal-pack")

	initCmd := newInitCmd()
	initCmd.SetArgs([]string{packDir})
	require.NoError(t, initCmd.Execute())

	// Non-existent key file
	sealCmdNonExistent := newSealCmd()
	sealCmdNonExistent.SetArgs([]string{"--key", filepath.Join(tmpDir, "nonexistent.key"), packDir})
	err := sealCmdNonExistent.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read private key")

	// Invalid private key content (too short)
	badKeyPath := filepath.Join(tmpDir, "bad.key")
	require.NoError(t, fileutil.WriteFile(badKeyPath, []byte("short-corrupted-key"), paths.FilePerm644))
	sealCmdBad := newSealCmd()
	sealCmdBad.SetArgs([]string{"--key", badKeyPath, packDir})
	err = sealCmdBad.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid private key")

	// Valid hex-encoded private key
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	hexKeyPath := filepath.Join(tmpDir, "priv_hex.key")
	require.NoError(t, fileutil.WriteFile(hexKeyPath, []byte(hex.EncodeToString(privKey)), paths.FilePerm644))

	pubOutPath := filepath.Join(tmpDir, "pub_out.hex")
	sealCmdHex := newSealCmd()
	sealBufHex := new(bytes.Buffer)
	sealCmdHex.SetOut(sealBufHex)
	sealCmdHex.SetArgs([]string{"--key", hexKeyPath, "--pubkey-out", pubOutPath, packDir})
	require.NoError(t, sealCmdHex.Execute())
	require.Contains(t, sealBufHex.String(), "Cryptographically sealed pack")

	// Valid raw 64-byte private key on another pack
	packDirRaw := filepath.Join(tmpDir, "seal-pack-raw")
	initCmdRaw := newInitCmd()
	initCmdRaw.SetArgs([]string{packDirRaw})
	require.NoError(t, initCmdRaw.Execute())

	rawKeyPath := filepath.Join(tmpDir, "priv_raw.key")
	require.NoError(t, fileutil.WriteFile(rawKeyPath, privKey, paths.FilePerm644))
	sealCmdRaw := newSealCmd()
	sealCmdRaw.SetArgs([]string{"--key", rawKeyPath, packDirRaw})
	require.NoError(t, sealCmdRaw.Execute())
}

func TestPackValidate_UnsealedAndKeyErrors(t *testing.T) {
	tmpDir := t.TempDir()
	packDir := filepath.Join(tmpDir, "unsealed-pack")

	// Scaffold unsealed pack
	initCmd := newInitCmd()
	initCmd.SetArgs([]string{packDir})
	require.NoError(t, initCmd.Execute())

	// Validate unsealed pack (Integrity == nil)
	valCmd := newValidateCmd()
	valBuf := new(bytes.Buffer)
	valCmd.SetOut(valBuf)
	valCmd.SetArgs([]string{packDir})
	require.NoError(t, valCmd.Execute())
	require.Contains(t, valBuf.String(), "is semantically valid but UNSEALED")

	// Missing swarm.yaml
	emptyDir := filepath.Join(tmpDir, "empty-dir")
	require.NoError(t, fileutil.MkdirAll(emptyDir, paths.DirPerm755))
	valCmdMissing := newValidateCmd()
	valCmdMissing.SetArgs([]string{emptyDir})
	err := valCmdMissing.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "package validation failed")

	// Now seal the pack
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	hexPrivKeyPath := filepath.Join(tmpDir, "seal_priv.hex")
	require.NoError(t, fileutil.WriteFile(hexPrivKeyPath, []byte(hex.EncodeToString(privKey)), paths.FilePerm644))
	sealCmd := newSealCmd()
	sealCmd.SetArgs([]string{"--key", hexPrivKeyPath, packDir})
	require.NoError(t, sealCmd.Execute())

	// Validate with non-existent public key file
	valCmdNoKey := newValidateCmd()
	valCmdNoKey.SetArgs([]string{"--key", filepath.Join(tmpDir, "nonexistent_pub.key"), packDir})
	err = valCmdNoKey.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read public key")

	// Validate with invalid public key (wrong length)
	badPubPath := filepath.Join(tmpDir, "bad_pub.key")
	require.NoError(t, fileutil.WriteFile(badPubPath, []byte("short-bad-pub"), paths.FilePerm644))
	valCmdBadKey := newValidateCmd()
	valCmdBadKey.SetArgs([]string{"--key", badPubPath, packDir})
	err = valCmdBadKey.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid public key")

	// Validate with raw 32-byte public key
	rawPubPath := filepath.Join(tmpDir, "raw_pub.key")
	require.NoError(t, fileutil.WriteFile(rawPubPath, pubKey, paths.FilePerm644))
	valCmdRawPub := newValidateCmd()
	valBufRawPub := new(bytes.Buffer)
	valCmdRawPub.SetOut(valBufRawPub)
	valCmdRawPub.SetArgs([]string{"--key", rawPubPath, packDir})
	require.NoError(t, valCmdRawPub.Execute())
	require.Contains(t, valBufRawPub.String(), "is valid and SEALED")

	// Validate with hex-encoded 64-char public key
	hexPubPath := filepath.Join(tmpDir, "hex_pub.key")
	require.NoError(t, fileutil.WriteFile(hexPubPath, []byte(hex.EncodeToString(pubKey)), paths.FilePerm644))
	valCmdHexPub := newValidateCmd()
	valBufHexPub := new(bytes.Buffer)
	valCmdHexPub.SetOut(valBufHexPub)
	valCmdHexPub.SetArgs([]string{"--key", hexPubPath, packDir})
	require.NoError(t, valCmdHexPub.Execute())
	require.Contains(t, valBufHexPub.String(), "is valid and SEALED")
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

func TestPackManifest_FileWriteAndOptions(t *testing.T) {
	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "output_manifest.yaml")

	manifestCmd := newManifestCmd()
	manifestBuf := new(bytes.Buffer)
	manifestCmd.SetOut(manifestBuf)
	manifestCmd.SetArgs([]string{
		"--name", "custom-written-swarm",
		"--version", "2.1.0",
		"--description", "Written to file test",
		"--author", "craftsman",
		"--license", "MIT",
		"--cell-type", "neuron",
		"--goal-metric", "coverage",
		"--goal-target", "90%",
		"-o", outPath,
	})
	require.NoError(t, manifestCmd.Execute())
	require.Contains(t, manifestBuf.String(), "Successfully wrote swarm manifest")

	data, err := fileutil.ReadFile(outPath)
	require.NoError(t, err)
	content := string(data)
	require.Contains(t, content, "name: custom-written-swarm")
	require.Contains(t, content, "version: 2.1.0")
	require.Contains(t, content, "author: craftsman")
	require.Contains(t, content, "license: MIT")

	// Error case: invalid semver
	manifestCmdErr := newManifestCmd()
	manifestCmdErr.SetArgs([]string{"--version", "invalid-semver"})
	err = manifestCmdErr.Execute()
	require.Error(t, err)
}

func TestPackCLI_BuiltinSwarmPacks(t *testing.T) {
	packs := []string{
		"code-eval",
		"greenfield-genesis",
		"legacy-refactor",
		"swarm-hitl",
		"rbac-recovery",
	}

	repoRoot, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatalf("failed to get repo root: %v", err)
	}

	for _, packName := range packs {
		packDir := filepath.Join(repoRoot, "packs", packName)
		validateCmd := newValidateCmd()
		buf := new(bytes.Buffer)
		validateCmd.SetOut(buf)
		validateCmd.SetArgs([]string{packDir})

		if err := validateCmd.Execute(); err != nil {
			t.Errorf("builtin pack %s validation failed: %v", packName, err)
		}

		if !strings.Contains(buf.String(), "is valid and SEALED") {
			t.Errorf("builtin pack %s output unexpected: %s", packName, buf.String())
		}
	}
}
