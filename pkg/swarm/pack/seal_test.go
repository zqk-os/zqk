package pack

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestSealAndVerifyPack_Success(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "swarm.yaml")
	if err := EnsureSampleSwarm(manifestPath); err != nil {
		t.Fatalf("EnsureSampleSwarm failed: %v", err)
	}

	// Create a sample template file in the pack
	templatesDir := filepath.Join(tmpDir, "templates")
	if err := fileutil.MkdirAll(templatesDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}
	templateFile := filepath.Join(templatesDir, "eval.yaml")
	if err := fileutil.WriteFile(templateFile, []byte("prompt: test evaluation prompt\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write template file: %v", err)
	}

	// Seal the pack
	integrity, err := SealPack(tmpDir, priv, "signer@zqk-os.com")
	if err != nil {
		t.Fatalf("SealPack failed: %v", err)
	}

	if integrity.Algorithm != "ed25519" {
		t.Errorf("expected algorithm ed25519, got %s", integrity.Algorithm)
	}
	if integrity.SignerID != "signer@zqk-os.com" {
		t.Errorf("expected signer_id signer@zqk-os.com, got %s", integrity.SignerID)
	}

	// Verify the pack
	verifiedPkg, err := VerifyPackIntegrity(tmpDir, pub)
	if err != nil {
		t.Fatalf("VerifyPackIntegrity failed: %v", err)
	}

	if verifiedPkg.Name != "sample-refactor-swarm" {
		t.Errorf("expected pack name sample-refactor-swarm, got %s", verifiedPkg.Name)
	}
}

func TestVerifyPack_TamperTemplateFailsClosed(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "swarm.yaml")
	if err := EnsureSampleSwarm(manifestPath); err != nil {
		t.Fatalf("EnsureSampleSwarm failed: %v", err)
	}

	templatesDir := filepath.Join(tmpDir, "templates")
	if err := fileutil.MkdirAll(templatesDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create templates dir: %v", err)
	}
	templateFile := filepath.Join(templatesDir, "eval.yaml")
	if err := fileutil.WriteFile(templateFile, []byte("prompt: original content\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write template file: %v", err)
	}

	// Seal
	if _, err := SealPack(tmpDir, priv, "signer@zqk-os.com"); err != nil {
		t.Fatalf("SealPack failed: %v", err)
	}

	// Tamper template file by modifying one byte
	if err := fileutil.WriteFile(templateFile, []byte("prompt: modified content\n"), paths.FilePerm644); err != nil {
		t.Fatalf("failed to tamper template file: %v", err)
	}

	// Verify must fail closed with ErrDigestMismatch
	_, err = VerifyPackIntegrity(tmpDir, pub)
	if err == nil {
		t.Fatal("expected failure on tampered template, got nil")
	}
	if !errors.Is(err, ErrDigestMismatch) {
		t.Errorf("expected ErrDigestMismatch, got %v", err)
	}
}

func TestVerifyPack_TamperManifestFailsClosed(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ed25519 key: %v", err)
	}

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "swarm.yaml")
	if err := EnsureSampleSwarm(manifestPath); err != nil {
		t.Fatalf("EnsureSampleSwarm failed: %v", err)
	}

	// Seal
	if _, err := SealPack(tmpDir, priv, "signer@zqk-os.com"); err != nil {
		t.Fatalf("SealPack failed: %v", err)
	}

	// Tamper manifest name without resealing
	tamperedBytes, err := fileutil.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}
	tamperedStr := strings.Replace(string(tamperedBytes), "sample-refactor-swarm", "tampered-swarm", 1)
	if err := fileutil.WriteFile(manifestPath, []byte(tamperedStr), paths.FilePerm644); err != nil {
		t.Fatalf("failed to write tampered manifest: %v", err)
	}

	// Verify must fail closed
	_, err = VerifyPackIntegrity(tmpDir, pub)
	if err == nil {
		t.Fatal("expected failure on tampered manifest, got nil")
	}
	if !errors.Is(err, ErrDigestMismatch) {
		t.Errorf("expected ErrDigestMismatch, got %v", err)
	}
}

func TestVerifyPack_InvalidSignatureFailsClosed(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)

	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "swarm.yaml")
	_ = EnsureSampleSwarm(manifestPath)

	// Seal with priv key
	_, err := SealPack(tmpDir, priv, "signer@zqk-os.com")
	if err != nil {
		t.Fatalf("SealPack failed: %v", err)
	}

	// Verify with a different public key
	_, err = VerifyPackIntegrity(tmpDir, otherPub)
	if err == nil {
		t.Fatal("expected failure when verifying with wrong public key, got nil")
	}
	if !errors.Is(err, ErrInvalidSignature) {
		t.Errorf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestVerifyPack_UnsealedFailsClosed(t *testing.T) {
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "swarm.yaml")
	_ = EnsureSampleSwarm(manifestPath)

	// Verify unsealed
	_, err := VerifyPackIntegrity(tmpDir, pub)
	if err == nil {
		t.Fatal("expected ErrUnsealedPack, got nil")
	}
	if !errors.Is(err, ErrUnsealedPack) {
		t.Errorf("expected ErrUnsealedPack, got %v", err)
	}
}
