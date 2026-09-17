package detector

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestBinaryDetector_GetPath(t *testing.T) {
	t.Parallel()
	detector := NewBinaryDetector()

	// Test when binary is not found
	_, err := detector.GetPath()
	if err == nil {
		t.Skip("Binary found in PATH - skipping test")
	}

	// Test with custom search path
	testDir := t.TempDir()
	testBinary := filepath.Join(testDir, "zqk-migrate")

	// Create a dummy binary
	if err := fileutil.WriteFile(testBinary, []byte("#!/bin/sh\necho 'test'"), paths.DirPerm755); err != nil {
		t.Fatalf("Failed to create test binary: %v", err)
	}

	detector = NewBinaryDetector().WithSearchPaths([]string{testDir})
	path, err := detector.GetPath()
	if err != nil {
		t.Fatalf("Expected to find binary, got error: %v", err)
	}
	if path != testBinary {
		t.Errorf("Expected path %s, got %s", testBinary, path)
	}
}

func TestGetBinaryHash(t *testing.T) {
	t.Parallel()
	// Create a test file
	testFile := filepath.Join(t.TempDir(), "test.bin")
	testData := []byte("test data for hashing")
	if err := fileutil.WriteFile(testFile, testData, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	hash1, err := GetBinaryHash(testFile)
	if err != nil {
		t.Fatalf("Failed to calculate hash: %v", err)
	}

	// Calculate again - should be the same
	hash2, err := GetBinaryHash(testFile)
	if err != nil {
		t.Fatalf("Failed to calculate hash: %v", err)
	}

	if hash1 != hash2 {
		t.Errorf("Hash should be deterministic, got %s and %s", hash1, hash2)
	}

	// Modify file - hash should change
	if err := fileutil.WriteFile(testFile, []byte("modified data"), paths.FilePerm644); err != nil {
		t.Fatalf("Failed to modify test file: %v", err)
	}

	hash3, err := GetBinaryHash(testFile)
	if err != nil {
		t.Fatalf("Failed to calculate hash: %v", err)
	}

	if hash1 == hash3 {
		t.Error("Hash should change when file is modified")
	}
}

func TestVerifyBinaryIntegrity(t *testing.T) {
	t.Parallel()
	// Create a test file
	testFile := filepath.Join(t.TempDir(), "test.bin")
	testData := []byte("test data")
	if err := fileutil.WriteFile(testFile, testData, paths.FilePerm644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Get correct hash
	correctHash, err := GetBinaryHash(testFile)
	if err != nil {
		t.Fatalf("Failed to calculate hash: %v", err)
	}

	// Verify with correct hash - should succeed
	if err := VerifyBinaryIntegrity(testFile, correctHash); err != nil {
		t.Errorf("Verification should succeed with correct hash: %v", err)
	}

	// Verify with wrong hash - should fail
	wrongHash := "0000000000000000000000000000000000000000000000000000000000000000"
	if err := VerifyBinaryIntegrity(testFile, wrongHash); err == nil {
		t.Error("Verification should fail with wrong hash")
	}
}

func TestBinaryDetector_GetVersion(t *testing.T) {
	t.Parallel()
	// This test requires the actual binary or a mock
	// Skip if binary not available
	detector := NewBinaryDetector()
	path, err := detector.GetPath()
	if err != nil {
		t.Skip("Binary not available - skipping version test")
	}

	version, err := detector.GetVersion(path)
	if err != nil {
		t.Logf("Could not get version (binary may not support --version): %v", err)
		return
	}

	if version == emptyValue {
		t.Error("Version should not be empty")
	}
}

func TestBinaryDetector_IsTrustedSigner(t *testing.T) {
	t.Parallel()
	detector := NewBinaryDetector().WithTrustedSigners([]string{
		"Developer ID Application: ZQK",
		"Developer ID Application: Test",
	})

	tests := []struct {
		name     string
		identity string
		expected bool
	}{
		{
			name:     "Trusted signer",
			identity: "Developer ID Application: ZQK (TEAM_ID)",
			expected: true,
		},
		{
			name:     "Another trusted signer",
			identity: "Developer ID Application: Test (TEAM_ID)",
			expected: true,
		},
		{
			name:     "Untrusted signer",
			identity: "Developer ID Application: Untrusted (TEAM_ID)",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detector.isTrustedSigner(tt.identity)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestSupportsMigration(t *testing.T) {
	t.Parallel()
	// This will check if binary is available
	// Result depends on whether binary is installed
	available := SupportsMigration()
	t.Logf("Migration support available: %v", available)
}
