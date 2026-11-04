// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package releasepack

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestConstantsAndPackInfo(t *testing.T) {
	if Name != "release" {
		t.Fatalf("expected Name to be %q, got %q", "release", Name)
	}
	if SpecDir != "packs/release/specs" {
		t.Fatalf("expected SpecDir to be %q, got %q", "packs/release/specs", SpecDir)
	}
	if LifecycleDir != "packs/release/lifecycles" {
		t.Fatalf("expected LifecycleDir to be %q, got %q", "packs/release/lifecycles", LifecycleDir)
	}
	if OmitBuildTag != "zqk_omit_releasepack" {
		t.Fatalf("expected OmitBuildTag to be %q, got %q", "zqk_omit_releasepack", OmitBuildTag)
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
	if len(info.Kinds) != 1 || info.Kinds[0] != "release" {
		t.Fatalf("expected info.Kinds [release], got %v", info.Kinds)
	}
}

func TestKindsAndHasKind(t *testing.T) {
	kinds := Kinds()
	if len(kinds) != 1 || kinds[0] != "release" {
		t.Fatalf("expected Kinds() to return [release], got %v", kinds)
	}

	if !HasKind("release") {
		t.Fatal("expected HasKind(\"release\") to be true")
	}
	if HasKind("goal") {
		t.Fatal("expected HasKind(\"goal\") to be false")
	}
	if HasKind("") {
		t.Fatal("expected HasKind(\"\") to be false")
	}
	if HasKind("RELEASE") {
		t.Fatal("expected HasKind(\"RELEASE\") to be false")
	}
}

func TestValidateKind(t *testing.T) {
	if err := ValidateKind("release"); err != nil {
		t.Fatalf("expected ValidateKind(\"release\") to succeed, got %v", err)
	}

	err := ValidateKind("unknown_kind")
	if err == nil {
		t.Fatal("expected ValidateKind to return error for unknown kind")
	}
	if !errors.Is(err, ErrUnknownKind) {
		t.Fatalf("expected error wrapping ErrUnknownKind, got %v", err)
	}
}

func TestSpecPath(t *testing.T) {
	// BaseDir empty
	rel, err := SpecPath("", "release")
	if err != nil {
		t.Fatalf("unexpected SpecPath error: %v", err)
	}
	expectedRel := filepath.Join(SpecDir, "release.yaml")
	if rel != expectedRel {
		t.Fatalf("expected %q, got %q", expectedRel, rel)
	}

	// BaseDir non-empty
	base := "/tmp/testroot"
	full, err := SpecPath(base, "release")
	if err != nil {
		t.Fatalf("unexpected SpecPath error: %v", err)
	}
	expectedFull := filepath.Join(base, expectedRel)
	if full != expectedFull {
		t.Fatalf("expected %q, got %q", expectedFull, full)
	}

	// Invalid kind
	if _, err := SpecPath("", "invalid"); err == nil {
		t.Fatal("expected error for invalid kind in SpecPath")
	}
}

func TestLifecyclePath(t *testing.T) {
	// BaseDir empty
	rel, err := LifecyclePath("", "release")
	if err != nil {
		t.Fatalf("unexpected LifecyclePath error: %v", err)
	}
	expectedRel := filepath.Join(LifecycleDir, "release_lifecycle.yaml")
	if rel != expectedRel {
		t.Fatalf("expected %q, got %q", expectedRel, rel)
	}

	// BaseDir non-empty
	base := "/tmp/testroot"
	full, err := LifecyclePath(base, "release")
	if err != nil {
		t.Fatalf("unexpected LifecyclePath error: %v", err)
	}
	expectedFull := filepath.Join(base, expectedRel)
	if full != expectedFull {
		t.Fatalf("expected %q, got %q", expectedFull, full)
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
	if err := VerifyKindFiles(tempDir, []string{"release"}); err == nil {
		t.Fatal("expected error when spec file is missing")
	}

	// 3. Create spec file but missing lifecycle file
	specFile := filepath.Join(tempDir, SpecDir, "release.yaml")
	if err := fileutil.MkdirAll(filepath.Dir(specFile), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fileutil.WriteFile(specFile, []byte("test spec content"), 0644); err != nil {
		t.Fatalf("write spec failed: %v", err)
	}
	if err := VerifyKindFiles(tempDir, []string{"release"}); err == nil {
		t.Fatal("expected error when lifecycle file is missing")
	}

	// 4. Create lifecycle file -> success
	lifecycleFile := filepath.Join(tempDir, LifecycleDir, "release_lifecycle.yaml")
	if err := fileutil.MkdirAll(filepath.Dir(lifecycleFile), 0755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := fileutil.WriteFile(lifecycleFile, []byte("test lifecycle content"), 0644); err != nil {
		t.Fatalf("write lifecycle failed: %v", err)
	}
	if err := VerifyKindFiles(tempDir, []string{"release"}); err != nil {
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
	subDir := filepath.Join(root, "pkg", "releasepack")
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
		goroutinelabels.NewGoroutine("test_releasepack_concurrency", "validating thread safety").StartSimple(func() {
			defer wg.Done()
			if len(Kinds()) != 1 {
				errCh <- errors.New("unexpected kinds length")
			}
			if !HasKind("release") {
				errCh <- errors.New("expected HasKind release")
			}
			if HasKind("unknown") {
				errCh <- errors.New("unexpected HasKind unknown")
			}
			if valErr := ValidateKind("release"); valErr != nil {
				errCh <- valErr
			}
			if valErr := ValidateKind("nonexistent"); valErr == nil {
				errCh <- errors.New("expected error for nonexistent")
			}
			info := Info()
			if info.Name != Name {
				errCh <- errors.New("unexpected pack name")
			}
			if _, specErr := SpecPath("", "release"); specErr != nil {
				errCh <- specErr
			}
			if _, specErr := SpecPath(root, "release"); specErr != nil {
				errCh <- specErr
			}
			if _, lcErr := LifecyclePath("", "release"); lcErr != nil {
				errCh <- lcErr
			}
			if _, lcErr := LifecyclePath(root, "release"); lcErr != nil {
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
