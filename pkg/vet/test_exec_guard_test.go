package vet

import (
	"os"
	"path/filepath"
	"testing"
)

// TestProhibitNakedExecCommand verifies CRIT-TST-SUBPROCESS-STATIC-GUARD / TST-TST-SUBPROCESS-STATIC-GUARD
func TestProhibitNakedExecCommand(t *testing.T) {
	tempDir := t.TempDir()
	pkgDir := filepath.Join(tempDir, "pkg", "sample")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatalf("failed to create pkg dir: %v", err)
	}

	// 1. Write an unmanaged test file that calls exec.Command directly
	unmanagedTest := `package sample

import (
	"os/exec"
	"testing"
)

func TestUnmanaged(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	_ = cmd.Run()
}
`
	if err := os.WriteFile(filepath.Join(pkgDir, "unmanaged_test.go"), []byte(unmanagedTest), 0o644); err != nil {
		t.Fatalf("failed to write unmanaged test: %v", err)
	}

	// 2. Write a managed test file that uses ManagedCommand
	managedTest := `package sample

import (
	"context"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/testkit"
)

func TestManaged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := testkit.ManagedCommand(t, ctx, "sleep", "1")
	_ = cmd.Run()
}
`
	if err := os.WriteFile(filepath.Join(pkgDir, "managed_test.go"), []byte(managedTest), 0o644); err != nil {
		t.Fatalf("failed to write managed test: %v", err)
	}

	cfg := &GatesConfig{
		Hygiene: HygieneConfig{
			GoScanDirs: []string{"pkg"},
		},
	}

	findings, err := CheckTestSubprocessHygiene(tempDir, cfg)
	if err != nil {
		t.Fatalf("CheckTestSubprocessHygiene returned error: %v", err)
	}

	if len(findings) != 1 {
		t.Fatalf("expected exactly 1 finding for unmanaged exec, got %d: %+v", len(findings), findings)
	}

	f := findings[0]
	if f.CheckID != "hygiene/unmanaged_test_exec" {
		t.Errorf("expected check_id hygiene/unmanaged_test_exec, got %s", f.CheckID)
	}
	if !filepath.IsAbs(f.File) && f.File != filepath.Join("pkg", "sample", "unmanaged_test.go") {
		t.Errorf("unexpected file in finding: %s", f.File)
	}
}
