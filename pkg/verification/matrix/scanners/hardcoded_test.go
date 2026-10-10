package scanners_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/verification/matrix"
	"github.com/zqk-os/zqk/pkg/verification/matrix/scanners"
)

func TestHardcodedLogicScanner_Comprehensive(t *testing.T) {
	tmpDir := t.TempDir()
	scanner := scanners.NewHardcodedLogicScanner()
	ctx := context.Background()

	// 1. Non-existent file error path
	missingEntry := &matrix.FileEntry{Path: "nonexistent.go", Class: matrix.ClassGoProd}
	resMissing, err := scanner.Run(ctx, tmpDir, missingEntry)
	if err == nil {
		t.Errorf("expected error for non-existent file, got nil")
	}
	if resMissing.Status != matrix.CheckStatusFailed {
		t.Errorf("expected failure status, got %v", resMissing.Status)
	}

	// 2. Secret detection
	secretFile := filepath.Join(tmpDir, "secret.go")
	secretContent := "package secret\nvar token = \"Bearer fake_bearer_token_12345678901234567890\"\n"
	if err := fileutil.WriteFile(secretFile, []byte(secretContent), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("failed to write secret file: %v", err)
	}
	resSecret, err := scanner.Run(ctx, tmpDir, &matrix.FileEntry{Path: "secret.go", Class: matrix.ClassGoProd})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resSecret.Status != matrix.CheckStatusFailed {
		t.Errorf("expected secret detection to fail check, got %v", resSecret.Status)
	}

	// 3. IP detection (non-test file)
	ipFile := filepath.Join(tmpDir, "ip.go")
	ipContent := "package network\nvar remote = \"192.168.1.100\"\n"
	if err := fileutil.WriteFile(ipFile, []byte(ipContent), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("failed to write ip file: %v", err)
	}
	resIP, err := scanner.Run(ctx, tmpDir, &matrix.FileEntry{Path: "ip.go", Class: matrix.ClassGoProd})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// IP produces warning, so status can still be passed if no error severity findings
	if len(resIP.Findings) == 0 {
		t.Errorf("expected finding for hardcoded IP, got 0")
	}

	// 4. Docs and test files allow certain patterns without error
	docFile := filepath.Join(tmpDir, "guide.md")
	docContent := "# Guide\nRun with /Users/test/dir and 192.168.1.1\n"
	if err := fileutil.WriteFile(docFile, []byte(docContent), fileutil.StandardFilePerm); err != nil {
		t.Fatalf("failed to write doc file: %v", err)
	}
	resDoc, err := scanner.Run(ctx, tmpDir, &matrix.FileEntry{Path: "guide.md", Class: matrix.ClassDocs})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resDoc.Status != matrix.CheckStatusPassed {
		t.Errorf("expected docs to pass, got %v: %v", resDoc.Status, resDoc.Findings)
	}
}
