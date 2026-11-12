package vet

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCheckImperativeBlankValidation(t *testing.T) {
	tempDir := t.TempDir()

	srcDir := filepath.Join(tempDir, "pkg", "sample")
	if err := fileutil.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create package dir: %v", err)
	}

	code := `package sample

import (
	"errors"
	"fmt"
	"strings"
)

func ProceduralCheck(val string) error {
	if strings.TrimSpace(val) == "" {
		return fmt.Errorf("val cannot be empty")
	}
	return nil
}

func ProceduralLenCheck(val string) error {
	if len(strings.TrimSpace(val)) == 0 {
		return errors.New("val is blank")
	}
	return nil
}

func ValidFunctionalCheck(val string) error {
	// Not an anti-pattern
	if strings.TrimSpace(val) == "" {
		println("just printing, not returning error")
	}
	return nil
}
`
	srcFile := filepath.Join(srcDir, "sample.go")
	if err := fileutil.WriteFile(srcFile, []byte(code), 0644); err != nil {
		t.Fatalf("failed to write test source: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Hygiene.GoScanDirs = []string{"pkg/"}
	findings, err := CheckImperativeBlankValidation(tempDir, []string{"pkg/sample/sample.go"}, cfg)
	if err != nil {
		t.Fatalf("unexpected error running CheckImperativeBlankValidation: %v", err)
	}

	if len(findings) != 2 {
		t.Fatalf("expected 2 findings for imperative blank checks, got %d findings: %+v", len(findings), findings)
	}

	for _, f := range findings {
		if f.CheckID != "hygiene/anti_pattern_blank_check" {
			t.Errorf("expected CheckID hygiene/anti_pattern_blank_check, got %s", f.CheckID)
		}
		if f.Severity != SeverityWarn {
			t.Errorf("expected severity warn, got %v", f.Severity)
		}
	}
}
