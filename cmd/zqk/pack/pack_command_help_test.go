package packcmd

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCycle101_PackCmd_HelpAndStructure tests the command hierarchy and flag registrations.
func TestCycle101_PackCmd_HelpAndStructure(t *testing.T) {
	cmd := NewPackCmd()
	require.NotNil(t, cmd)

	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"--help"})
	require.NoError(t, cmd.Execute())
	output := buf.String()

	require.Contains(t, output, "init")
	require.Contains(t, output, "seal")
	require.Contains(t, output, "validate")
	require.Contains(t, output, "manifest")
}

// TestCycle101_PackInit_Comprehensive tests scaffold generation with various flag permutations and boundary cases.
func TestCycle101_PackInit_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Scaffold into an explicit subfolder with custom name, version, and description
	targetDir := filepath.Join(tmpDir, "packs", "pck-demo-custom")
	initCmd := newInitCmd()
	var outBuf bytes.Buffer
	initCmd.SetOut(&outBuf)
	initCmd.SetArgs([]string{
		"--name", "pck-demo-custom",
		"--version", "2.4.1",
		"--description", "Custom high-assurance evaluation pack",
		targetDir,
	})
	require.NoError(t, initCmd.Execute())
	require.Contains(t, outBuf.String(), `Successfully scaffolded holonic swarm package "pck-demo-custom"`)

	manifestPath := filepath.Join(targetDir, "swarm.yaml")
	require.True(t, fileutil.Exists(manifestPath))
	data, err := fileutil.ReadFile(manifestPath)
	require.NoError(t, err)
	manifestStr := string(data)
	require.Contains(t, manifestStr, "name: pck-demo-custom")
	require.Contains(t, manifestStr, "version: 2.4.1")
	require.Contains(t, manifestStr, "description: Custom high-assurance evaluation pack")
	require.Contains(t, manifestStr, "Lead Specialist")
	require.Contains(t, manifestStr, "Adversarial Verifier")
	require.Contains(t, manifestStr, "task-evaluate")
	require.Contains(t, manifestStr, "task-verify")

	// Verify sample prompt template exists
	tplPath := filepath.Join(targetDir, "templates", "eval_prompt.yaml")
	require.True(t, fileutil.Exists(tplPath))
	tplBytes, err := fileutil.ReadFile(tplPath)
	require.NoError(t, err)
	require.Contains(t, string(tplBytes), "Evaluate the system against defined requirements")

	// 2. Collision failure on re-init
	initCmdCollision := newInitCmd()
	initCmdCollision.SetArgs([]string{
		"--name", "pck-demo-custom",
		targetDir,
	})
	err = initCmdCollision.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")

	// 3. Name derived from base folder when name flag is omitted
	packDerivedDir := filepath.Join(tmpDir, "derived-pack-name")
	initCmdDerived := newInitCmd()
	var outBufDerived bytes.Buffer
	initCmdDerived.SetOut(&outBufDerived)
	initCmdDerived.SetArgs([]string{packDerivedDir})
	require.NoError(t, initCmdDerived.Execute())
	require.Contains(t, outBufDerived.String(), `Successfully scaffolded holonic swarm package "derived-pack-name"`)

	// 4. Invalid SemVer version format
	initCmdBadVer := newInitCmd()
	initCmdBadVer.SetArgs([]string{"--version", "invalid.version.format", filepath.Join(tmpDir, "bad-ver-pack")})
	err = initCmdBadVer.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid package version")

	// 5. Pre-existing templates and membranes folders should succeed idempotently
	preExistingDir := filepath.Join(tmpDir, "pre-existing-subdirs")
	require.NoError(t, fileutil.MkdirAll(filepath.Join(preExistingDir, "templates"), paths.DirPerm755))
	require.NoError(t, fileutil.MkdirAll(filepath.Join(preExistingDir, "membranes"), paths.DirPerm755))
	initCmdPreExisting := newInitCmd()
	var outBufPreExisting bytes.Buffer
	initCmdPreExisting.SetOut(&outBufPreExisting)
	initCmdPreExisting.SetArgs([]string{preExistingDir})
	require.NoError(t, initCmdPreExisting.Execute())
	require.Contains(t, outBufPreExisting.String(), "Successfully scaffolded")
}

// TestCycle101_PackSeal_EdgeCasesAndOptions tests sealing under various key formats and error situations.
func TestCycle101_PackSeal_EdgeCasesAndOptions(t *testing.T) {
	tmpDir := t.TempDir()
	packDir := filepath.Join(tmpDir, "seal-suite-pack")

	// Scaffold pack first
	initCmd := newInitCmd()
	initCmd.SetArgs([]string{packDir})
	require.NoError(t, initCmd.Execute())

	// 1. Seal with default generated ephemeral key and custom signer ID
	sealCmdEphemeral := newSealCmd()
	var sealBufEphemeral bytes.Buffer
	sealCmdEphemeral.SetOut(&sealBufEphemeral)
	pubOutFile := filepath.Join(tmpDir, "ephemeral_pub.key")
	sealCmdEphemeral.SetArgs([]string{
		"--signer-id", "architect@example.com",
		"--pubkey-out", pubOutFile,
		packDir,
	})
	require.NoError(t, sealCmdEphemeral.Execute())
	sealOutput := sealBufEphemeral.String()
	require.Contains(t, sealOutput, "Cryptographically sealed pack")
	require.Contains(t, sealOutput, "Signer:  architect@example.com")
	require.Contains(t, sealOutput, "Digest:")
	require.Contains(t, sealOutput, "Pubkey:")
	require.True(t, fileutil.Exists(pubOutFile))

	// 2. Validate the sealed pack with the exported public key
	valCmd := newValidateCmd()
	var valBuf bytes.Buffer
	valCmd.SetOut(&valBuf)
	valCmd.SetArgs([]string{"--key", pubOutFile, packDir})
	require.NoError(t, valCmd.Execute())
	require.Contains(t, valBuf.String(), "is valid and SEALED")

	// 3. Error when writing pubkey to an invalid/inaccessible directory
	sealCmdBadPubOut := newSealCmd()
	sealCmdBadPubOut.SetArgs([]string{
		"--pubkey-out", filepath.Join(tmpDir, "non_existent_folder", "sub", "pub.key"),
		packDir,
	})
	err := sealCmdBadPubOut.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to write public key")

	// 4. Error when sealing non-existent directory
	sealCmdMissingDir := newSealCmd()
	sealCmdMissingDir.SetArgs([]string{filepath.Join(tmpDir, "nonexistent-pack-dir")})
	err = sealCmdMissingDir.Execute()
	require.Error(t, err)

	// 5. Sealing with whitespace-padded hex private key
	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	paddedHexKey := "  \n\t" + hex.EncodeToString(privKey) + "\n "
	paddedKeyFile := filepath.Join(tmpDir, "padded_priv.hex")
	require.NoError(t, fileutil.WriteFile(paddedKeyFile, []byte(paddedHexKey), paths.FilePerm644))

	sealCmdPadded := newSealCmd()
	var sealBufPadded bytes.Buffer
	sealCmdPadded.SetOut(&sealBufPadded)
	sealCmdPadded.SetArgs([]string{
		"--key", paddedKeyFile,
		packDir,
	})
	require.NoError(t, sealCmdPadded.Execute())
	require.Contains(t, sealBufPadded.String(), "Cryptographically sealed pack")
}

// TestCycle101_PackValidate_TamperingAndKeyMismatches tests cryptographic verification invariants.
func TestCycle101_PackValidate_TamperingAndKeyMismatches(t *testing.T) {
	tmpDir := t.TempDir()
	packDir := filepath.Join(tmpDir, "val-suite-pack")

	// Scaffold pack
	initCmd := newInitCmd()
	initCmd.SetArgs([]string{"--name", "pck-demo-validate", packDir})
	require.NoError(t, initCmd.Execute())

	// Validate unsealed pack prints warning
	valCmdUnsealed := newValidateCmd()
	var unsealedBuf bytes.Buffer
	valCmdUnsealed.SetOut(&unsealedBuf)
	valCmdUnsealed.SetArgs([]string{packDir})
	require.NoError(t, valCmdUnsealed.Execute())
	require.Contains(t, unsealedBuf.String(), "is semantically valid but UNSEALED")

	// Generate two separate keypairs
	pub1, priv1, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	pub2, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	key1File := filepath.Join(tmpDir, "key1.priv")
	require.NoError(t, fileutil.WriteFile(key1File, priv1, paths.FilePerm644))

	pub1File := filepath.Join(tmpDir, "pub1.hex")
	require.NoError(t, fileutil.WriteFile(pub1File, []byte(hex.EncodeToString(pub1)), paths.FilePerm644))

	pub2File := filepath.Join(tmpDir, "pub2.hex")
	require.NoError(t, fileutil.WriteFile(pub2File, []byte(hex.EncodeToString(pub2)), paths.FilePerm644))

	// Seal with key1
	sealCmd := newSealCmd()
	sealCmd.SetArgs([]string{"--key", key1File, packDir})
	require.NoError(t, sealCmd.Execute())

	// Validate with wrong key (pub2) -> fails integrity verification
	valCmdWrongKey := newValidateCmd()
	valCmdWrongKey.SetArgs([]string{"--key", pub2File, packDir})
	err = valCmdWrongKey.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "integrity verification failed")

	// Validate with correct key (pub1) -> succeeds
	valCmdCorrectKey := newValidateCmd()
	var correctBuf bytes.Buffer
	valCmdCorrectKey.SetOut(&correctBuf)
	valCmdCorrectKey.SetArgs([]string{"--key", pub1File, packDir})
	require.NoError(t, valCmdCorrectKey.Execute())
	require.Contains(t, correctBuf.String(), "is valid and SEALED")

	// Tamper an internal file
	tplPath := filepath.Join(packDir, "templates", "eval_prompt.yaml")
	require.NoError(t, fileutil.WriteFile(tplPath, []byte("prompt: adversarial-mutation\n"), paths.FilePerm644))

	// Validate tampered pack -> fails
	valCmdTampered := newValidateCmd()
	valCmdTampered.SetArgs([]string{"--key", pub1File, packDir})
	err = valCmdTampered.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "integrity verification failed")
}

// TestCycle101_PackManifest_FormattingAndErrorHandling tests manifest generation and output routing.
func TestCycle101_PackManifest_FormattingAndErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. Stdout generation using alias 'draft'
	rootCmd := NewPackCmd()
	var stdoutBuf bytes.Buffer
	rootCmd.SetOut(&stdoutBuf)
	rootCmd.SetArgs([]string{
		"draft",
		"--name", "pck-demo-stdout",
		"--version", "1.5.0",
		"--description", "Manifest stdout streaming verification",
		"--author", "craftsman@zqk.dev",
		"--license", "MIT",
		"--cell-type", "neuron",
		"--goal-metric", "latency_p99",
		"--goal-target", "< 15ms",
	})
	require.NoError(t, rootCmd.Execute())
	draftStr := stdoutBuf.String()
	require.Contains(t, draftStr, "name: pck-demo-stdout")
	require.Contains(t, draftStr, "version: 1.5.0")
	require.Contains(t, draftStr, "craftsman@zqk.dev")
	require.Contains(t, draftStr, "MIT")

	// 2. File output writing to invalid path
	badOutCmd := newManifestCmd()
	badOutCmd.SetArgs([]string{
		"--output", filepath.Join(tmpDir, "missing-dir", "no-perm", "out.yaml"),
	})
	err := badOutCmd.Execute()
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to write manifest to")
}
