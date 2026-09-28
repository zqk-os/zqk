package vet

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckRawGoroutines(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Valid file using goroutinelabels (no raw go statements)
	validPkgDir := filepath.Join(tempDir, "pkg", "valid")
	if err := os.MkdirAll(validPkgDir, 0o755); err != nil {
		t.Fatalf("mkdir valid: %v", err)
	}
	validCode := `package valid

import "fmt"

func DoWork() {
	fmt.Println("doing work")
}
`
	if err := os.WriteFile(filepath.Join(validPkgDir, "valid.go"), []byte(validCode), 0o644); err != nil {
		t.Fatalf("write valid.go: %v", err)
	}

	// 2. Invalid file with raw go func()
	invalidPkgDir := filepath.Join(tempDir, "pkg", "invalid")
	if err := os.MkdirAll(invalidPkgDir, 0o755); err != nil {
		t.Fatalf("mkdir invalid: %v", err)
	}
	invalidCode := `package invalid

func BadWork() {
	go func() {
		// unmanaged goroutine
	}()
}
`
	if err := os.WriteFile(filepath.Join(invalidPkgDir, "bad.go"), []byte(invalidCode), 0o644); err != nil {
		t.Fatalf("write bad.go: %v", err)
	}

	// 3. Test file with raw go statement (exempt)
	testCode := `package valid

import "testing"

func TestSomething(t *testing.T) {
	go func() {
		// allowed in tests
	}()
}
`
	if err := os.WriteFile(filepath.Join(validPkgDir, "valid_test.go"), []byte(testCode), 0o644); err != nil {
		t.Fatalf("write valid_test.go: %v", err)
	}

	// 4. Goroutinelabels implementation file (exempt)
	labelDir := filepath.Join(tempDir, "pkg", "goroutinelabels")
	if err := os.MkdirAll(labelDir, 0o755); err != nil {
		t.Fatalf("mkdir goroutinelabels: %v", err)
	}
	labelCode := `package goroutinelabels

func StartInternal() {
	go func() {
		// primitive wrapper
	}()
}
`
	if err := os.WriteFile(filepath.Join(labelDir, "primitive.go"), []byte(labelCode), 0o644); err != nil {
		t.Fatalf("write primitive.go: %v", err)
	}

	cfg := DefaultConfig()
	cfg.Hygiene.GoScanDirs = []string{"pkg"}

	findings, err := CheckRawGoroutines(tempDir, nil, cfg)
	if err != nil {
		t.Fatalf("CheckRawGoroutines failed: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected exactly 1 finding, got %d: %+v", len(findings), findings)
	}

	f := findings[0]
	if f.CheckID != "hygiene/raw_goroutine" {
		t.Errorf("expected CheckID 'hygiene/raw_goroutine', got %s", f.CheckID)
	}
	if f.Severity != SeverityError {
		t.Errorf("expected SeverityError, got %s", f.Severity)
	}
	if f.File != "pkg/invalid/bad.go" {
		t.Errorf("expected file 'pkg/invalid/bad.go', got %s", f.File)
	}
	if f.Line != 4 {
		t.Errorf("expected line 4, got %d", f.Line)
	}
}
