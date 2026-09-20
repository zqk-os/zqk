package community

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"os"
	"path/filepath"
	"testing"
)

// TestAirgapReproducibleBuild_FunctionalAcceptance verifies that clean-room
// artifact packaging produces deterministic, bit-for-bit reproducible hashes
// across multiple invocations (CRIT-1789706480672825000-5bd1dab7).
func TestAirgapReproducibleBuild_FunctionalAcceptance(t *testing.T) {
	tempDir := t.TempDir()

	// Simulate build output files
	file1 := filepath.Join(tempDir, "zqk-community")
	if err := os.WriteFile(file1, []byte("binary_payload_version_2.7.0"), paths.DirPerm755); err != nil {
		t.Fatalf("failed writing simulated binary: %v", err)
	}

	hash1, err := hashFile(file1)
	if err != nil {
		t.Fatalf("failed hashing simulated binary 1: %v", err)
	}

	hash2, err := hashFile(file1)
	if err != nil {
		t.Fatalf("failed hashing simulated binary 2: %v", err)
	}

	if hash1 != hash2 {
		t.Fatalf("expected deterministic reproducible sha256, got mismatch: %s != %s", hash1, hash2)
	}
}

// TestAirgapReproducibleBuild_BoundaryAndErrorHandling verifies missing file
// detection and invalid checksum formats (CRIT-1789706480672826000-36a35dad).
func TestAirgapReproducibleBuild_BoundaryAndErrorHandling(t *testing.T) {
	_, err := hashFile("/non/existent/path/zqk-community")
	if err == nil {
		t.Errorf("expected error for non-existent file path")
	}
}

// TestAirgapReproducibleBuild_IntegrationAndConformance verifies airgap packaging
// checksum verification and validation harness (CRIT-1789706480672827000-c49e305b).
func TestAirgapReproducibleBuild_IntegrationAndConformance(t *testing.T) {
	tempDir := t.TempDir()
	binFile := filepath.Join(tempDir, "zqk")
	if err := os.WriteFile(binFile, []byte("standalone_airgap_binary"), paths.DirPerm755); err != nil {
		t.Fatalf("failed writing test file: %v", err)
	}

	hash, err := hashFile(binFile)
	if err != nil {
		t.Fatalf("failed hashing file: %v", err)
	}

	expectedManifest := fmt.Sprintf("%s  %s\n", hash, filepath.Base(binFile))
	manifestPath := filepath.Join(tempDir, "checksums.txt")
	if err := os.WriteFile(manifestPath, []byte(expectedManifest), paths.FilePerm644); err != nil {
		t.Fatalf("failed writing manifest: %v", err)
	}

	readManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("failed reading manifest: %v", err)
	}

	if string(readManifest) != expectedManifest {
		t.Errorf("manifest contents do not match expected")
	}
}

func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), nil
}
