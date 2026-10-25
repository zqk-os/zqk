// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package systempeel

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func writeMockGoFile(t *testing.T, fullPath string, lines int) {
	t.Helper()
	var sb strings.Builder
	for i := 0; i < lines; i++ {
		if i > 0 {
			sb.WriteString("\n")
		}
		if i == 0 {
			sb.WriteString("package testpkg")
		} else {
			sb.WriteString(fmt.Sprintf("// line %d commentary", i))
		}
	}
	if err := fileutil.WriteStandardFile(fullPath, []byte(sb.String())); err != nil {
		t.Fatalf("failed to write mock file %s: %v", fullPath, err)
	}
}

func TestScanGoFiles_ComprehensiveDirectoryTraversalAndFiltering(t *testing.T) {
	tempDir := t.TempDir()
	pkgRel := filepath.Join("cmd", "zqk", "system")
	pkgFull := filepath.Join(tempDir, pkgRel)

	if err := fileutil.EnsureDir(pkgFull); err != nil {
		t.Fatalf("failed to create pkg dir: %v", err)
	}

	writeMockGoFile(t, filepath.Join(pkgFull, "cmd_alpha.go"), 15)
	writeMockGoFile(t, filepath.Join(pkgFull, "cmd_beta.go"), 25)

	// Non-Go files that must be skipped by filter
	nonGoFile1 := filepath.Join(pkgFull, "README.md")
	if err := fileutil.WriteStandardFile(nonGoFile1, []byte("# Docs\n")); err != nil {
		t.Fatalf("failed to write non-go file: %v", err)
	}
	nonGoFile2 := filepath.Join(pkgFull, "cmd_alpha.go.bak")
	if err := fileutil.WriteStandardFile(nonGoFile2, []byte("backup\n")); err != nil {
		t.Fatalf("failed to write backup file: %v", err)
	}

	// Subdirectory that must be skipped because entry.IsDir() is true
	nestedDir := filepath.Join(pkgFull, "nested")
	if err := fileutil.EnsureDir(nestedDir); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}
	writeMockGoFile(t, filepath.Join(nestedDir, "nested.go"), 10)

	inspector := NewSystemInspector(tempDir)
	files, err := inspector.scanGoFiles(pkgRel)
	if err != nil {
		t.Fatalf("scanGoFiles failed unexpectedly: %v", err)
	}

	if len(files) != 2 {
		t.Fatalf("expected exactly 2 Go files scanned, got %d", len(files))
	}

	foundAlpha := false
	foundBeta := false
	for _, f := range files {
		if f.baseName == "cmd_alpha.go" {
			foundAlpha = true
			if f.loc != 15 {
				t.Errorf("expected 15 LOC for alpha, got %d", f.loc)
			}
		}
		if f.baseName == "cmd_beta.go" {
			foundBeta = true
			if f.loc != 25 {
				t.Errorf("expected 25 LOC for beta, got %d", f.loc)
			}
		}
	}

	if !foundAlpha || !foundBeta {
		t.Errorf("expected both alpha and beta go files to be detected: alpha=%v, beta=%v", foundAlpha, foundBeta)
	}
}

func TestScanGoFiles_EmptyDirectory(t *testing.T) {
	tempDir := t.TempDir()
	emptyPkgRel := filepath.Join("pkg", "empty")
	if err := fileutil.EnsureDir(filepath.Join(tempDir, emptyPkgRel)); err != nil {
		t.Fatalf("failed to create empty dir: %v", err)
	}

	inspector := NewSystemInspector(tempDir)
	files, err := inspector.scanGoFiles(emptyPkgRel)
	if err != nil {
		t.Fatalf("expected empty directory scan to succeed, got: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 files, got %d", len(files))
	}
}

func TestScanGoFiles_ErrorBoundaries(t *testing.T) {
	tempDir := t.TempDir()
	inspector := NewSystemInspector(tempDir)

	// Non-existent directory error path
	if _, err := inspector.scanGoFiles("nonexistent/package/dir"); err == nil {
		t.Fatal("expected error for non-existent package dir, got nil")
	}

	// Target package path is a regular file instead of a directory
	filePath := filepath.Join(tempDir, "file_as_pkg.go")
	if err := fileutil.WriteStandardFile(filePath, []byte("package foo\n")); err != nil {
		t.Fatalf("failed to create regular file: %v", err)
	}
	if _, err := inspector.scanGoFiles("file_as_pkg.go"); err == nil {
		t.Fatal("expected error when pkgRelPath is a file rather than directory")
	}

	// Unreadable file branch
	pkgRel := filepath.Join("cmd", "unreadable_pkg")
	pkgFull := filepath.Join(tempDir, pkgRel)
	if err := fileutil.EnsureDir(pkgFull); err != nil {
		t.Fatalf("failed to create unreadable_pkg dir: %v", err)
	}
	targetUnreadable := filepath.Join(pkgFull, "locked.go")
	if err := fileutil.WriteStandardFile(targetUnreadable, []byte("package foo\n")); err != nil {
		t.Fatalf("failed to write locked file: %v", err)
	}

	if err := os.Chmod(targetUnreadable, 0000); err != nil {
		t.Fatalf("failed to chmod 0000: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(targetUnreadable, 0644); err != nil {
			t.Logf("cleanup chmod failed: %v", err)
		}
	})

	_, err := inspector.scanGoFiles(pkgRel)
	if err == nil {
		t.Fatal("expected error when reading unreadable file")
	}
	if !strings.Contains(err.Error(), "failed to read file") {
		t.Errorf("expected 'failed to read file' in error, got: %v", err)
	}
}

func TestInspectSystemPackage_MultiFileAndLargeThreshold(t *testing.T) {
	tempDir := t.TempDir()
	pkgRel := filepath.Join("cmd", "zqk", "modular")
	pkgFull := filepath.Join(tempDir, pkgRel)
	if err := fileutil.EnsureDir(pkgFull); err != nil {
		t.Fatalf("failed to create pkg dir: %v", err)
	}

	writeMockGoFile(t, filepath.Join(pkgFull, "small.go"), 50)
	writeMockGoFile(t, filepath.Join(pkgFull, "huge_one.go"), 1200)
	writeMockGoFile(t, filepath.Join(pkgFull, "huge_two.go"), 1050)

	inspector := NewSystemInspector(tempDir)
	meta, err := inspector.InspectSystemPackage(pkgRel)
	if err != nil {
		t.Fatalf("InspectSystemPackage failed: %v", err)
	}

	if meta.Name != "modular" {
		t.Errorf("expected package name 'modular', got %q", meta.Name)
	}
	if meta.PackagePath != pkgRel {
		t.Errorf("expected package path %q, got %q", pkgRel, meta.PackagePath)
	}
	if meta.LinesOfCode != 2300 {
		t.Errorf("expected 2300 total LOC, got %d", meta.LinesOfCode)
	}
	if len(meta.Subcommands) != 3 {
		t.Errorf("expected 3 subcommands, got %d", len(meta.Subcommands))
	}
	if len(meta.LargeFiles) != 2 {
		t.Fatalf("expected 2 large files exceeding 1000 LOC, got %d", len(meta.LargeFiles))
	}

	// Verify descending sorting of large files
	if meta.LargeFiles[0].LinesOfCode < meta.LargeFiles[1].LinesOfCode {
		t.Errorf("expected descending sort: %d should be >= %d",
			meta.LargeFiles[0].LinesOfCode, meta.LargeFiles[1].LinesOfCode)
	}
	if meta.LargeFiles[0].BaseName != "huge_one.go" {
		t.Errorf("expected largest file to be huge_one.go, got %s", meta.LargeFiles[0].BaseName)
	}
}

func TestInspectSystemPackage_ScanErrorPropagation(t *testing.T) {
	inspector := NewSystemInspector(t.TempDir())
	meta, err := inspector.InspectSystemPackage("nonexistent/missing/dir")
	if err == nil {
		t.Fatal("expected error for nonexistent package, got nil")
	}
	if meta != nil {
		t.Fatal("expected nil metadata on scan error")
	}
}

func TestFindFilesExceedingThreshold_SortingAndBoundary(t *testing.T) {
	tempDir := t.TempDir()
	pkgRel := filepath.Join("cmd", "zqk", "threshold")
	pkgFull := filepath.Join(tempDir, pkgRel)
	if err := fileutil.EnsureDir(pkgFull); err != nil {
		t.Fatalf("failed to create pkg dir: %v", err)
	}

	writeMockGoFile(t, filepath.Join(pkgFull, "low.go"), 20)
	writeMockGoFile(t, filepath.Join(pkgFull, "mid.go"), 60)
	writeMockGoFile(t, filepath.Join(pkgFull, "high.go"), 100)

	inspector := NewSystemInspector(tempDir)

	// Threshold at 50 LOC: mid.go and high.go should match
	reports, err := inspector.FindFilesExceedingThreshold(pkgRel, 50)
	if err != nil {
		t.Fatalf("FindFilesExceedingThreshold failed: %v", err)
	}
	if len(reports) != 2 {
		t.Fatalf("expected 2 reports exceeding threshold 50, got %d", len(reports))
	}
	if reports[0].LinesOfCode != 100 || reports[1].LinesOfCode != 60 {
		t.Errorf("expected sorted [100, 60], got [%d, %d]",
			reports[0].LinesOfCode, reports[1].LinesOfCode)
	}

	// Threshold at 200 LOC: no files should match
	reportsEmpty, err := inspector.FindFilesExceedingThreshold(pkgRel, 200)
	if err != nil {
		t.Fatalf("FindFilesExceedingThreshold failed: %v", err)
	}
	if len(reportsEmpty) != 0 {
		t.Errorf("expected 0 reports exceeding threshold 200, got %d", len(reportsEmpty))
	}

	// Error path: nonexistent package directory
	if _, err := inspector.FindFilesExceedingThreshold("missing/dir", 10); err == nil {
		t.Fatal("expected error for missing directory in FindFilesExceedingThreshold")
	}
}

func TestPeeledComponentRegistry_IsolatedOperations(t *testing.T) {
	reg := &PeeledComponentRegistry{
		components: make(map[string]*PeeledCommandMetadata),
	}

	// Verify Get on empty registry
	if item, exists := reg.Get("missing"); exists || item != nil {
		t.Fatalf("expected missing item to return (nil, false), got (%v, %v)", item, exists)
	}

	// Register items in non-alphabetical order to test sorting in List()
	reg.Register("zeta_service", &PeeledCommandMetadata{Name: "zeta_service", LinesOfCode: 300})
	reg.Register("alpha_service", &PeeledCommandMetadata{Name: "alpha_service", LinesOfCode: 100})
	reg.Register("mu_service", &PeeledCommandMetadata{Name: "mu_service", LinesOfCode: 200})

	// Test Get on existing item
	item, exists := reg.Get("alpha_service")
	if !exists || item == nil {
		t.Fatal("expected alpha_service to exist in registry")
	}
	if item.LinesOfCode != 100 {
		t.Errorf("expected 100 LOC, got %d", item.LinesOfCode)
	}

	// Test List sorting order
	list := reg.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 items in list, got %d", len(list))
	}
	if list[0].Name != "alpha_service" || list[1].Name != "mu_service" || list[2].Name != "zeta_service" {
		t.Errorf("expected sorted order [alpha_service, mu_service, zeta_service], got [%s, %s, %s]",
			list[0].Name, list[1].Name, list[2].Name)
	}
}

func TestAttachPeeledCommand_NilSafeties(t *testing.T) {
	validParent := &cobra.Command{Use: "parent"}
	validChild := &cobra.Command{Use: "child"}

	// Both nil
	AttachPeeledCommand(nil, nil)

	// Nil parent, valid child
	AttachPeeledCommand(nil, validChild)

	// Valid parent, nil child
	AttachPeeledCommand(validParent, nil)
	if len(validParent.Commands()) != 0 {
		t.Errorf("expected 0 commands attached when child is nil, got %d", len(validParent.Commands()))
	}

	// Both valid
	AttachPeeledCommand(validParent, validChild)
	if len(validParent.Commands()) != 1 {
		t.Fatalf("expected 1 command attached, got %d", len(validParent.Commands()))
	}
	if validParent.Commands()[0].Use != "child" {
		t.Errorf("expected attached child command 'child', got %s", validParent.Commands()[0].Use)
	}
}

func TestSystemInspector_ConcurrentInspection(t *testing.T) {
	tempDir := t.TempDir()
	pkgRel := filepath.Join("cmd", "zqk", "concurrent")
	pkgFull := filepath.Join(tempDir, pkgRel)
	if err := fileutil.EnsureDir(pkgFull); err != nil {
		t.Fatalf("failed to create pkg dir: %v", err)
	}

	for i := 0; i < 5; i++ {
		writeMockGoFile(t, filepath.Join(pkgFull, fmt.Sprintf("cmd_%d.go", i)), 50*(i+1))
	}

	inspector := NewSystemInspector(tempDir)
	reg := &PeeledComponentRegistry{
		components: make(map[string]*PeeledCommandMetadata),
	}

	var wg sync.WaitGroup
	numWorkers := 10

	for i := 0; i < numWorkers; i++ {
		workerID := i
		wg.Add(1)
		goroutinelabels.NewGoroutine(
			fmt.Sprintf("test_systempeel_worker_%d", workerID),
			"concurrent inspection and registry verification",
		).StartSimple(func() {
			defer wg.Done()

			meta, err := inspector.InspectSystemPackage(pkgRel)
			if err != nil {
				t.Errorf("worker %d inspect failed: %v", workerID, err)
				return
			}
			if meta.LinesOfCode <= 0 {
				t.Errorf("worker %d expected positive LOC, got %d", workerID, meta.LinesOfCode)
				return
			}

			reports, err := inspector.FindFilesExceedingThreshold(pkgRel, 100)
			if err != nil {
				t.Errorf("worker %d find exceeding threshold failed: %v", workerID, err)
				return
			}
			if len(reports) < 3 {
				t.Errorf("worker %d expected at least 3 reports, got %d", workerID, len(reports))
				return
			}

			regName := fmt.Sprintf("service_%d", workerID)
			reg.Register(regName, meta)
			if retrieved, ok := reg.Get(regName); !ok || retrieved == nil {
				t.Errorf("worker %d failed to retrieve registered service", workerID)
				return
			}
		})
	}

	wg.Wait()

	if len(reg.List()) != numWorkers {
		t.Errorf("expected %d registered components in registry, got %d", numWorkers, len(reg.List()))
	}
}
