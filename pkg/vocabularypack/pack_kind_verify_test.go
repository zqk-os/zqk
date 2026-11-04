// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package vocabularypack

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestConstantsAndPackInfo(t *testing.T) {
	if Name != "vocabulary" {
		t.Fatalf("expected Name to be %q, got %q", "vocabulary", Name)
	}
	if SpecDir != "packs/vocabulary/specs" {
		t.Fatalf("expected SpecDir to be %q, got %q", "packs/vocabulary/specs", SpecDir)
	}
	if LifecycleDir != "packs/vocabulary/lifecycles" {
		t.Fatalf("expected LifecycleDir to be %q, got %q", "packs/vocabulary/lifecycles", LifecycleDir)
	}
	if OmitBuildTag != "zqk_omit_vocabularypack" {
		t.Fatalf("expected OmitBuildTag to be %q, got %q", "zqk_omit_vocabularypack", OmitBuildTag)
	}

	info := Info()
	if info.Name != Name {
		t.Fatalf("expected info.Name %q, got %q", Name, info.Name)
	}
	if info.SpecDir != SpecDir {
		t.Fatalf("expected info.SpecDir %q, got %q", SpecDir, info.SpecDir)
	}
	if info.LifecycleDir != LifecycleDir {
		t.Fatalf("expected info.LifecycleDir %q, got %q", LifecycleDir, info.LifecycleDir)
	}
	if info.OmitBuildTag != OmitBuildTag {
		t.Fatalf("expected info.OmitBuildTag %q, got %q", OmitBuildTag, info.OmitBuildTag)
	}
	expectedKinds := []string{
		"glossary_term",
		"glossary_term_relation",
		"import_tracking",
		"vocabulary_scheme",
	}
	if len(info.Kinds) != len(expectedKinds) {
		t.Fatalf("expected %d kinds, got %d", len(expectedKinds), len(info.Kinds))
	}
	for i, k := range expectedKinds {
		if info.Kinds[i] != k {
			t.Fatalf("expected kind index %d to be %q, got %q", i, k, info.Kinds[i])
		}
	}
}

func TestKindsAndHasKind(t *testing.T) {
	kinds := Kinds()
	expectedKinds := []string{
		"glossary_term",
		"glossary_term_relation",
		"import_tracking",
		"vocabulary_scheme",
	}

	if len(kinds) != len(expectedKinds) {
		t.Fatalf("expected %d kinds, got %d", len(expectedKinds), len(kinds))
	}

	seen := make(map[string]bool)
	for i, kind := range kinds {
		if kind == "" {
			t.Fatalf("expected non-empty kind at index %d", i)
		}
		if kind != expectedKinds[i] {
			t.Fatalf("expected kind %q at index %d, got %q", expectedKinds[i], i, kind)
		}
		if seen[kind] {
			t.Fatalf("duplicate kind %q found at index %d", kind, i)
		}
		seen[kind] = true

		if !HasKind(kind) {
			t.Fatalf("expected HasKind(%q) to be true", kind)
		}
	}

	// Negative cases
	if HasKind("unknown_kind") {
		t.Fatal("expected HasKind(\"unknown_kind\") to be false")
	}
	if HasKind("") {
		t.Fatal("expected HasKind(\"\") to be false")
	}
	if HasKind("GLOSSARY_TERM") {
		t.Fatal("expected HasKind(\"GLOSSARY_TERM\") to be false")
	}
	if HasKind("vocabulary") {
		t.Fatal("expected HasKind(\"vocabulary\") to be false")
	}
}

func TestValidateKind(t *testing.T) {
	for _, kind := range Kinds() {
		if err := ValidateKind(kind); err != nil {
			t.Fatalf("expected ValidateKind(%q) to succeed, got %v", kind, err)
		}
	}

	err := ValidateKind("invalid_kind")
	if err == nil {
		t.Fatal("expected ValidateKind to return error for invalid kind")
	}
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("expected error wrapping ErrUnknownKind, got %v", err)
	}
}

func TestSpecPath(t *testing.T) {
	for _, kind := range Kinds() {
		// BaseDir empty
		rel, err := SpecPath("", kind)
		if err != nil {
			t.Fatalf("unexpected SpecPath error for %q: %v", kind, err)
		}
		expectedRel := filepath.Join(SpecDir, kind+".yaml")
		if rel != expectedRel {
			t.Fatalf("expected %q, got %q", expectedRel, rel)
		}

		// BaseDir non-empty
		base := "/tmp/vocabulary_testroot"
		full, err := SpecPath(base, kind)
		if err != nil {
			t.Fatalf("unexpected SpecPath error with baseDir for %q: %v", kind, err)
		}
		expectedFull := filepath.Join(base, expectedRel)
		if full != expectedFull {
			t.Fatalf("expected %q, got %q", expectedFull, full)
		}
	}

	// Invalid kind
	if _, err := SpecPath("", "invalid"); err == nil {
		t.Fatal("expected error for invalid kind in SpecPath")
	}
}

func TestLifecyclePath(t *testing.T) {
	for _, kind := range Kinds() {
		// BaseDir empty
		rel, err := LifecyclePath("", kind)
		if err != nil {
			t.Fatalf("unexpected LifecyclePath error for %q: %v", kind, err)
		}
		expectedRel := filepath.Join(LifecycleDir, kind+"_lifecycle.yaml")
		if rel != expectedRel {
			t.Fatalf("expected %q, got %q", expectedRel, rel)
		}

		// BaseDir non-empty
		base := "/tmp/vocabulary_testroot"
		full, err := LifecyclePath(base, kind)
		if err != nil {
			t.Fatalf("unexpected LifecyclePath error with baseDir for %q: %v", kind, err)
		}
		expectedFull := filepath.Join(base, expectedRel)
		if full != expectedFull {
			t.Fatalf("expected %q, got %q", expectedFull, full)
		}
	}

	// Invalid kind
	if _, err := LifecyclePath("", "invalid"); err == nil {
		t.Fatal("expected error for invalid kind in LifecyclePath")
	}
}

func TestVerifyKindFiles(t *testing.T) {
	tempDir := t.TempDir()

	// 1. Invalid kind
	if err := VerifyKindFiles(tempDir, []string{"unsupported"}); err == nil {
		t.Fatal("expected error verifying unsupported kind")
	}

	// 2. Missing spec file
	if err := VerifyKindFiles(tempDir, []string{"glossary_term"}); err == nil {
		t.Fatal("expected error when spec file is missing")
	}

	// 3. Create spec file but missing lifecycle file
	specFile := filepath.Join(tempDir, SpecDir, "glossary_term.yaml")
	if err := fileutil.MkdirAll(filepath.Dir(specFile), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fileutil.WriteFile(specFile, []byte("test spec content"), 0644); err != nil {
		t.Fatalf("write spec failed: %v", err)
	}
	if err := VerifyKindFiles(tempDir, []string{"glossary_term"}); err == nil {
		t.Fatal("expected error when lifecycle file is missing")
	}

	// 4. Create lifecycle file -> success
	lifecycleFile := filepath.Join(tempDir, LifecycleDir, "glossary_term_lifecycle.yaml")
	if err := fileutil.MkdirAll(filepath.Dir(lifecycleFile), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fileutil.WriteFile(lifecycleFile, []byte("test lifecycle content"), 0644); err != nil {
		t.Fatalf("write lifecycle failed: %v", err)
	}
	if err := VerifyKindFiles(tempDir, []string{"glossary_term"}); err != nil {
		t.Fatalf("expected VerifyKindFiles to succeed, got %v", err)
	}
}

func TestVerifyFiles(t *testing.T) {
	// Verify against the real repository
	root, err := ResolveModuleRoot("")
	if err != nil {
		t.Fatalf("failed to resolve module root: %v", err)
	}
	if err := VerifyFiles(root); err != nil {
		t.Fatalf("expected VerifyFiles on real repo to succeed, got %v", err)
	}

	// Verify on empty temp dir fails
	emptyDir := t.TempDir()
	if err := VerifyFiles(emptyDir); err == nil {
		t.Fatal("expected VerifyFiles on empty dir to fail")
	}
}

func TestResolveModuleRoot(t *testing.T) {
	// 1. Empty string resolves cwd repository root
	root, err := ResolveModuleRoot("")
	if err != nil {
		t.Fatalf("ResolveModuleRoot(\"\") error: %v", err)
	}
	if _, err := fileutil.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("resolved root does not contain go.mod: %v", err)
	}

	// 2. Explicit path to repository root
	root2, err := ResolveModuleRoot(root)
	if err != nil {
		t.Fatalf("ResolveModuleRoot(root) error: %v", err)
	}
	if root != root2 {
		t.Fatalf("expected %q, got %q", root, root2)
	}

	// 3. Subdirectory in repository
	subDir := filepath.Join(root, "pkg", "vocabularypack")
	root3, err := ResolveModuleRoot(subDir)
	if err != nil {
		t.Fatalf("ResolveModuleRoot(subDir) error: %v", err)
	}
	if root != root3 {
		t.Fatalf("expected %q, got %q", root, root3)
	}

	// 4. Directory without go.mod returns error
	tempDir := t.TempDir()
	if _, err := ResolveModuleRoot(tempDir); err == nil {
		t.Fatal("expected error resolving root from directory without go.mod")
	}

	// 5. Simulated getwd error
	origGetwd := getwd
	defer func() { getwd = origGetwd }()
	getwd = func() (string, error) {
		return "", errors.New("simulated working directory error")
	}
	if _, err := ResolveModuleRoot(""); err == nil {
		t.Fatal("expected error when getwd fails")
	}
}

func TestVerifyPackInRepo(t *testing.T) {
	// 1. In repo root or empty string
	if err := VerifyPackInRepo(""); err != nil {
		t.Fatalf("VerifyPackInRepo(\"\") error: %v", err)
	}

	// 2. Directory without go.mod
	tempDir := t.TempDir()
	if err := VerifyPackInRepo(tempDir); err == nil {
		t.Fatal("expected error verifying pack in non-repo dir")
	}

	// 3. Directory with go.mod but missing pack specs
	tempWithMod := t.TempDir()
	goModContent := []byte("module github.com/zqk-os/zqk\n\ngo 1.24\n")
	if err := fileutil.WriteFile(filepath.Join(tempWithMod, "go.mod"), goModContent, 0644); err != nil {
		t.Fatalf("failed to write go.mod: %v", err)
	}
	if err := VerifyPackInRepo(tempWithMod); err == nil {
		t.Fatal("expected error verifying pack when pack directories are missing")
	}
}

func TestFilesystemConsistency(t *testing.T) {
	root, err := ResolveModuleRoot("")
	if err != nil {
		t.Fatalf("ResolveModuleRoot failed: %v", err)
	}

	// Verify each kind has its corresponding spec and lifecycle files physically present
	for _, kind := range Kinds() {
		specPath := filepath.Join(root, SpecDir, kind+".yaml")
		if _, err := fileutil.Stat(specPath); err != nil {
			t.Fatalf("expected spec file for kind %q at %s: %v", kind, specPath, err)
		}

		lifecyclePath := filepath.Join(root, LifecycleDir, kind+"_lifecycle.yaml")
		if _, err := fileutil.Stat(lifecyclePath); err != nil {
			t.Fatalf("expected lifecycle file for kind %q at %s: %v", kind, lifecyclePath, err)
		}
	}
}

func TestConcurrencySafety(t *testing.T) {
	root, err := ResolveModuleRoot("")
	if err != nil {
		t.Fatalf("ResolveModuleRoot failed: %v", err)
	}

	const workerCount = 16
	var wg sync.WaitGroup
	errCh := make(chan error, workerCount*10)
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		goroutinelabels.NewGoroutine("test_vocabularypack_concurrency", "validating thread safety").StartSimple(func() {
			defer wg.Done()
			if len(Kinds()) != 4 {
				errCh <- errors.New("unexpected kinds length")
			}
			for _, k := range Kinds() {
				if !HasKind(k) {
					errCh <- errors.New("expected HasKind " + k)
				}
				if valErr := ValidateKind(k); valErr != nil {
					errCh <- valErr
				}
			}
			if HasKind("unknown") {
				errCh <- errors.New("unexpected HasKind unknown")
			}
			if valErr := ValidateKind("nonexistent"); valErr == nil {
				errCh <- errors.New("expected error for nonexistent")
			}
			info := Info()
			if info.Name != Name {
				errCh <- errors.New("unexpected pack name")
			}
			if _, specErr := SpecPath("", "glossary_term"); specErr != nil {
				errCh <- specErr
			}
			if _, specErr := SpecPath(root, "glossary_term"); specErr != nil {
				errCh <- specErr
			}
			if _, lcErr := LifecyclePath("", "glossary_term"); lcErr != nil {
				errCh <- lcErr
			}
			if _, lcErr := LifecyclePath(root, "glossary_term"); lcErr != nil {
				errCh <- lcErr
			}
			if vfErr := VerifyFiles(root); vfErr != nil {
				errCh <- vfErr
			}
			if _, resErr := ResolveModuleRoot(""); resErr != nil {
				errCh <- resErr
			}
			if vprErr := VerifyPackInRepo(""); vprErr != nil {
				errCh <- vprErr
			}
		})
	}

	wg.Wait()
	close(errCh)
	for workerErr := range errCh {
		if workerErr != nil {
			t.Fatalf("concurrency worker error: %v", workerErr)
		}
	}
}
