package community

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestProviderDiscovery_StaticFloorInvariant(t *testing.T) {
	manifest := &ProviderManifest{
		ID:               "zqk.provider.test",
		Name:             "Test Provider",
		Version:          "1.0.0",
		MinEngineVersion: "0.3.6",
		Capabilities:     []string{"telemetry", "export"},
	}

	if err := manifest.Validate(); err != nil {
		t.Fatalf("expected valid manifest, got error: %v", err)
	}

	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}

	parsed, err := ParseManifest(data)
	if err != nil {
		t.Fatalf("expected ParseManifest to succeed, got %v", err)
	}
	if parsed.ID != manifest.ID {
		t.Errorf("expected id %s, got %s", manifest.ID, parsed.ID)
	}
	if parsed.Name != manifest.Name {
		t.Errorf("expected name %s, got %s", manifest.Name, parsed.Name)
	}
	if len(parsed.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(parsed.Capabilities))
	}
}

func TestProviderDiscovery_OperationalDynamicProof(t *testing.T) {
	ctx := context.Background()
	tempDir := t.TempDir()

	pluginDir1 := filepath.Join(tempDir, "provider-alpha")
	if err := fileutil.MkdirAll(pluginDir1, 0755); err != nil {
		t.Fatalf("failed to create plugin dir: %v", err)
	}

	payload := []byte("binary-plugin-content-payload-alpha")
	hasher := sha256.New()
	hasher.Write(payload)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	m1 := ProviderManifest{
		ID:               "zqk.provider.alpha",
		Name:             "Alpha Provider",
		Version:          "0.1.0",
		MinEngineVersion: "0.3.6",
		Capabilities:     []string{"adapter.storage"},
		ChecksumSHA256:   checksum,
	}
	data1, _ := json.Marshal(m1)
	if err := fileutil.WriteFile(filepath.Join(pluginDir1, "provider.json"), data1, 0644); err != nil {
		t.Fatalf("failed to write provider.json: %v", err)
	}

	scanner := NewProviderDiscoveryScanner(tempDir)
	discovered, err := scanner.Scan(ctx)
	if err != nil {
		t.Fatalf("expected scan to succeed, got %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 discovered provider, got %d", len(discovered))
	}
	if discovered[0].ID != "zqk.provider.alpha" {
		t.Errorf("expected id zqk.provider.alpha, got %s", discovered[0].ID)
	}

	// Verify integrity check
	if err := VerifyManifestIntegrity(discovered[0], payload); err != nil {
		t.Fatalf("expected integrity verification to pass, got: %v", err)
	}

	// Test ScanSingleDirectory
	single, err := scanner.ScanSingleDirectory(pluginDir1)
	if err != nil {
		t.Fatalf("expected ScanSingleDirectory to pass, got: %v", err)
	}
	if single.ID != "zqk.provider.alpha" {
		t.Errorf("expected id zqk.provider.alpha, got: %s", single.ID)
	}
}

func TestProviderDiscovery_NegativeBoundaryAndErrorRecovery(t *testing.T) {
	// Case 1: Empty manifest payload
	if _, err := ParseManifest([]byte{}); !errors.Is(err, ErrManifestInvalid) {
		t.Errorf("expected ErrManifestInvalid for empty payload, got %v", err)
	}

	// Case 2: Malformed JSON
	if _, err := ParseManifest([]byte(`{not-json}`)); !errors.Is(err, ErrManifestInvalid) {
		t.Errorf("expected ErrManifestInvalid for malformed JSON, got %v", err)
	}

	// Case 3: Missing required fields
	mMissing := &ProviderManifest{
		ID: "valid.id",
		// missing Name, Version, Capabilities
	}
	if err := mMissing.Validate(); !errors.Is(err, ErrManifestInvalid) {
		t.Errorf("expected ErrManifestInvalid for missing name, got %v", err)
	}

	// Case 4: Invalid ID characters
	mBadID := &ProviderManifest{
		ID:           "bad ID with spaces!",
		Name:         "Name",
		Version:      "1.0.0",
		Capabilities: []string{"test"},
	}
	if err := mBadID.Validate(); !errors.Is(err, ErrManifestInvalid) {
		t.Errorf("expected ErrManifestInvalid for invalid ID, got %v", err)
	}

	// Case 5: Empty capabilities
	mNoCaps := &ProviderManifest{
		ID:           "test.id",
		Name:         "Name",
		Version:      "1.0.0",
		Capabilities: []string{},
	}
	if err := mNoCaps.Validate(); !errors.Is(err, ErrManifestInvalid) {
		t.Errorf("expected ErrManifestInvalid for empty capabilities, got %v", err)
	}

	// Case 6: Checksum mismatch
	mChecksum := &ProviderManifest{
		ID:             "test.id",
		Name:           "Name",
		Version:        "1.0.0",
		Capabilities:   []string{"test"},
		ChecksumSHA256: "0000000000000000000000000000000000000000000000000000000000000000",
	}
	if err := VerifyManifestIntegrity(mChecksum, []byte("tampered-content")); !errors.Is(err, ErrIntegrityMismatch) {
		t.Errorf("expected ErrIntegrityMismatch, got %v", err)
	}

	// Case 7: Non-existent directory scanning
	scanner := NewProviderDiscoveryScanner("/tmp/path/that/does/not/exist/ever")
	discovered, err := scanner.Scan(context.Background())
	if err != nil {
		t.Fatalf("expected non-existent dir scan to return empty list without error, got: %v", err)
	}
	if len(discovered) != 0 {
		t.Errorf("expected 0 discovered, got %d", len(discovered))
	}
}

func TestProviderDiscovery_DocumentationVerification(t *testing.T) {
	adrPath := filepath.Join("..", "..", "docs", "architecture", "ADR_COMMUNITY_PROVIDER_DISCOVERY_AND_MANIFEST_VERIFICATION_V036.md")
	data, err := fileutil.ReadFile(adrPath)
	if err != nil {
		t.Fatalf("expected ADR-0040 to exist at %s, got: %v", adrPath, err)
	}
	content := string(data)
	if len(content) == 0 {
		t.Fatalf("ADR-0040 content cannot be empty")
	}
	if len(content) > 8000 {
		t.Errorf("ADR-0040 exceeds POL-DOC-001 character limit (8000), size: %d", len(content))
	}
}
