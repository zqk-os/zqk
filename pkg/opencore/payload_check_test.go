package opencore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPayloadCheck_DetectsSensitivePatterns(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a file that contains a sensitive pattern
	sensitiveContent := []byte("api_key = \"sk-1234567890abcdef\"\npassword = \"secret123\"")
	err := os.WriteFile(filepath.Join(tmpDir, "config.env"), sensitiveContent, 0600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report, err := PayloadCheck(tmpDir, PayloadCheckOptions{})
	if err != nil {
		t.Fatalf("PayloadCheck returned error: %v", err)
	}

	if len(report.Violations) == 0 {
		t.Fatal("expected at least one violation for sensitive patterns, got none")
	}
}

func TestPayloadCheck_AllowsSafeContent(t *testing.T) {
	tmpDir := t.TempDir()

	safeContent := []byte("name = \"MyProject\"\nversion = \"1.0.0\"\ndescription = \"A safe open-core project\"")
	err := os.WriteFile(filepath.Join(tmpDir, "README.md"), safeContent, 0600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report, err := PayloadCheck(tmpDir, PayloadCheckOptions{})
	if err != nil {
		t.Fatalf("PayloadCheck returned error: %v", err)
	}

	if len(report.Violations) > 0 {
		t.Errorf("expected no violations for safe content, got: %+v", report.Violations)
	}
}

func TestPayloadCheck_IgnoresExcludedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	sensitiveContent := []byte("api_key = \"sk-1234567890abcdef\"")
	err := os.WriteFile(filepath.Join(tmpDir, ".env"), sensitiveContent, 0600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report, err := PayloadCheck(tmpDir, PayloadCheckOptions{
		ExcludedPatterns: []string{"*.env"},
	})
	if err != nil {
		t.Fatalf("PayloadCheck returned error: %v", err)
	}

	for _, v := range report.Violations {
		if filepath.Base(v.Path) == ".env" {
			t.Errorf("expected .env to be excluded, but found violation in %s", v.Path)
		}
	}
}

func TestPayloadCheck_RejectsBinaryInRelease(t *testing.T) {
	tmpDir := t.TempDir()

	// Write a file with binary content (null bytes)
	binaryContent := []byte{0x7f, 0x45, 0x4c, 0x46, 0x02, 0x01, 0x01, 0x00, 0x00}
	err := os.WriteFile(filepath.Join(tmpDir, "libfoo.so"), binaryContent, 0600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report, err := PayloadCheck(tmpDir, PayloadCheckOptions{
		CheckBinaries: true,
	})
	if err != nil {
		t.Fatalf("PayloadCheck returned error: %v", err)
	}

	foundBinaryViolation := false
	for _, v := range report.Violations {
		if v.Category == "binary-detect" {
			foundBinaryViolation = true
			break
		}
	}
	if !foundBinaryViolation {
		t.Error("expected binary-detect violation for .so file")
	}
}

func TestPayloadCheck_SummaryReturnsTotalCounts(t *testing.T) {
	tmpDir := t.TempDir()

	sensitiveContent := []byte("api_key = \"sk-1234567890abcdef\"")
	err := os.WriteFile(filepath.Join(tmpDir, "config.env"), sensitiveContent, 0600)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	report, err := PayloadCheck(tmpDir, PayloadCheckOptions{})
	if err != nil {
		t.Fatalf("PayloadCheck returned error: %v", err)
	}

	if report.TotalViolations != 1 {
		t.Errorf("expected total violations = 1, got %d", report.TotalViolations)
	}
}

func TestPayloadCheck_EmptyDirReturnsCleanReport(t *testing.T) {
	tmpDir := t.TempDir()

	report, err := PayloadCheck(tmpDir, PayloadCheckOptions{})
	if err != nil {
		t.Fatalf("PayloadCheck returned error: %v", err)
	}

	if report.TotalViolations != 0 {
		t.Errorf("expected 0 violations for empty dir, got %d", report.TotalViolations)
	}
}
