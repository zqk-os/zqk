// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package appledouble

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"sync"
	"testing"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestIsSidecarFileName_Comprehensive(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		input    string
		expected bool
	}{
		{name: "empty string", input: "", expected: false},
		{name: "exact sidecar prefix only", input: "._", expected: true},
		{name: "standard sidecar file", input: "._repair_command.yaml", expected: true},
		{name: "sidecar with relative dir", input: "foo/bar/._baz.json", expected: true},
		{name: "sidecar with absolute path", input: "/var/tmp/._manifest.xml", expected: true},
		{name: "sidecar uppercase extension", input: "._SPEC.YAML", expected: true},
		{name: "regular file", input: "repair_command.yaml", expected: false},
		{name: "dot git directory file", input: ".git", expected: false},
		{name: "dot gitignore file", input: ".gitignore", expected: false},
		{name: "dot zqk directory file", input: ".zqk", expected: false},
		{name: "dot DS_Store file", input: ".DS_Store", expected: false},
		{name: "middle dot-underscore", input: "foo._bar.yaml", expected: false},
		{name: "trailing dot-underscore", input: "something._", expected: false},
		{name: "single dot only", input: ".", expected: false},
		{name: "double dot only", input: "..", expected: false},
		{name: "single underscore only", input: "_", expected: false},
		{name: "double underscore prefix", input: "__init__.py", expected: false},
		{name: "dot without underscore", input: ".hidden", expected: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IsSidecarFileName(tc.input)
			if got != tc.expected {
				t.Fatalf("IsSidecarFileName(%q) = %v, want %v", tc.input, got, tc.expected)
			}
			gotPolicy := SkipNameInReadDir(tc.input)
			if gotPolicy != tc.expected {
				t.Fatalf("SkipNameInReadDir(%q) = %v, want %v", tc.input, gotPolicy, tc.expected)
			}
		})
	}
}

func TestPathHasSidecarSegment_Comprehensive(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		path     string
		expected bool
	}{
		{name: "empty string", path: "", expected: false},
		{name: "single root slash", path: "/", expected: false},
		{name: "multiple root slashes", path: "///", expected: false},
		{name: "current directory dot", path: ".", expected: false},
		{name: "parent directory double dot", path: "..", expected: false},
		{name: "relative regular path", path: "cli_specs/system/repair_command.yaml", expected: false},
		{name: "absolute regular path", path: "/usr/local/bin/zqk", expected: false},
		{name: "leading sidecar segment", path: "._foo/bar/baz.yaml", expected: true},
		{name: "middle sidecar segment", path: "cli_specs/._system/repair_command.yaml", expected: true},
		{name: "trailing sidecar file segment", path: "cli_specs/system/._repair_command.yaml", expected: true},
		{name: "deeply nested sidecar segment", path: "a/b/c/d/._e/f/g.json", expected: true},
		{name: "windows backslash regular path", path: `foo\bar\baz.yaml`, expected: false},
		{name: "windows backslash sidecar segment", path: `foo\._bar\baz.yaml`, expected: true},
		{name: "consecutive slashes with sidecar", path: "a//._b///c", expected: true},
		{name: "consecutive slashes regular path", path: "a///b///c", expected: false},
		{name: "trailing slash on sidecar segment", path: "a/b/._c/", expected: true},
		{name: "trailing slash on regular path", path: "a/b/c/", expected: false},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := PathHasSidecarSegment(tc.path)
			if got != tc.expected {
				t.Fatalf("PathHasSidecarSegment(%q) = %v, want %v", tc.path, got, tc.expected)
			}
			gotSkip := SkipPathInTreeWalk(tc.path)
			if gotSkip != tc.expected {
				t.Fatalf("SkipPathInTreeWalk(%q) = %v, want %v", tc.path, gotSkip, tc.expected)
			}
		})
	}
}

func TestSkipDirOrSidecar_Matrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		isDir    bool
		path     string
		expected bool
	}{
		{name: "directory with normal path", isDir: true, path: "pkg/storage", expected: true},
		{name: "directory with sidecar path", isDir: true, path: "pkg/._storage", expected: true},
		{name: "file with normal path", isDir: false, path: "pkg/storage/store.go", expected: false},
		{name: "file with sidecar basename", isDir: false, path: "pkg/storage/._store.go", expected: true},
		{name: "file in sidecar parent dir", isDir: false, path: "pkg/._storage/store.go", expected: true},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := SkipDirOrSidecar(tc.isDir, tc.path)
			if got != tc.expected {
				t.Fatalf("SkipDirOrSidecar(%v, %q) = %v, want %v", tc.isDir, tc.path, got, tc.expected)
			}
		})
	}
}

func TestReadDirFiltering_Hermetic(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	createHermeticTestFiles(t, tempDir, []string{
		"config.yaml",
		"._config.yaml",
		"manifest.json",
		"._manifest.json",
		"subfolder",
		"._subfolder",
	})

	entries, err := fileutil.ReadDir(tempDir)
	if err != nil {
		t.Fatalf("fileutil.ReadDir failed: %v", err)
	}

	var accepted []string
	for _, entry := range entries {
		if SkipNameInReadDir(entry.Name()) {
			continue
		}
		accepted = append(accepted, entry.Name())
	}

	expectedMap := map[string]bool{
		"config.yaml":   true,
		"manifest.json": true,
		"subfolder":     true,
	}

	if len(accepted) != len(expectedMap) {
		t.Fatalf("expected %d entries, got %d: %v", len(expectedMap), len(accepted), accepted)
	}

	for _, name := range accepted {
		if !expectedMap[name] {
			t.Fatalf("unexpected entry accepted: %s", name)
		}
	}
}

func TestTreeWalkFiltering_Hermetic(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	createHermeticTreeStructure(t, tempDir)

	var visitedRegular []string
	var detectedSidecars []string

	walkErr := filepath.WalkDir(tempDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if SkipPathInTreeWalk(path) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(tempDir, path)
		if relErr != nil {
			return relErr
		}
		normRel := filepath.ToSlash(rel)

		if SkipPathInTreeWalk(path) {
			detectedSidecars = append(detectedSidecars, normRel)
			return nil
		}
		visitedRegular = append(visitedRegular, normRel)
		return nil
	})

	if walkErr != nil {
		t.Fatalf("filepath.WalkDir encountered unexpected error: %v", walkErr)
	}

	expectedSidecars := map[string]bool{
		"._root.yaml":         true,
		".zqk/._process.yaml": true,
		"sub/._child.yaml":    true,
	}

	if len(detectedSidecars) != len(expectedSidecars) {
		t.Fatalf("expected %d detected sidecars, got %d: %v", len(expectedSidecars), len(detectedSidecars), detectedSidecars)
	}
	for _, sidecar := range detectedSidecars {
		if !expectedSidecars[sidecar] {
			t.Fatalf("unexpected sidecar detected: %s", sidecar)
		}
	}

	expectedAccepted := map[string]bool{
		".zqk/process.yaml": true,
		"root.yaml":         true,
		"sub/child.yaml":    true,
	}

	if len(visitedRegular) != len(expectedAccepted) {
		t.Fatalf("expected %d visited files, got %d: %v", len(expectedAccepted), len(visitedRegular), visitedRegular)
	}

	for _, rel := range visitedRegular {
		if !expectedAccepted[rel] {
			t.Fatalf("unexpected regular file visited: %s", rel)
		}
	}
}

func TestConcurrencySafety(t *testing.T) {
	t.Parallel()

	const workerCount = 16
	const iterations = 200

	testPaths := []string{
		"regular/file.yaml",
		"regular/._sidecar.yaml",
		"._dir/file.yaml",
		`windows\path\file.yaml`,
		`windows\._path\file.yaml`,
		"deep/nested/path/to/spec.yaml",
		"deep/nested/path/to/._spec.yaml",
	}

	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		workerID := i
		builder := goroutinelabels.NewGoroutine("appledouble_test", fmt.Sprintf("concurrency worker %d", workerID))
		builder.StartSimple(func() {
			defer wg.Done()
			for iter := 0; iter < iterations; iter++ {
				for _, p := range testPaths {
					resSegment := PathHasSidecarSegment(p)
					resName := IsSidecarFileName(p)
					resSkipDir := SkipDirOrSidecar(iter%2 == 0, p)
					resSkipName := SkipNameInReadDir(p)
					resSkipPath := SkipPathInTreeWalk(p)

					if resName != resSkipName {
						t.Errorf("mismatch: IsSidecarFileName(%q) != SkipNameInReadDir", p)
					}
					if resSegment != resSkipPath {
						t.Errorf("mismatch: PathHasSidecarSegment(%q) != SkipPathInTreeWalk", p)
					}
					if iter%2 == 0 && !resSkipDir {
						t.Errorf("expected isDir=true to always return true, got false for %q", p)
					}
				}
			}
		})
	}

	wg.Wait()
}

func createHermeticTestFiles(t *testing.T, baseDir string, names []string) {
	t.Helper()
	for _, name := range names {
		fullPath := filepath.Join(baseDir, name)
		if name == "subfolder" || name == "._subfolder" {
			if err := fileutil.MkdirAll(fullPath, 0750); err != nil {
				t.Fatalf("failed to create directory %q: %v", fullPath, err)
			}
			continue
		}
		if err := fileutil.WriteFile(fullPath, []byte("payload"), 0600); err != nil {
			t.Fatalf("failed to write file %q: %v", fullPath, err)
		}
	}
}

func createHermeticTreeStructure(t *testing.T, root string) {
	t.Helper()

	dirs := []string{
		filepath.Join(root, ".zqk"),
		filepath.Join(root, "sub"),
		filepath.Join(root, "._sidecardir"),
	}
	for _, d := range dirs {
		if err := fileutil.MkdirAll(d, 0750); err != nil {
			t.Fatalf("failed to create dir %q: %v", d, err)
		}
	}

	files := map[string]string{
		filepath.Join(root, "root.yaml"):                   "root content",
		filepath.Join(root, "._root.yaml"):                 "apple double sidecar",
		filepath.Join(root, ".zqk", "process.yaml"):        "zqk process config",
		filepath.Join(root, ".zqk", "._process.yaml"):      "apple double in dot dir",
		filepath.Join(root, "sub", "child.yaml"):           "sub child content",
		filepath.Join(root, "sub", "._child.yaml"):         "sub child sidecar",
		filepath.Join(root, "._sidecardir", "spec.yaml"):   "content inside sidecar dir",
		filepath.Join(root, "._sidecardir", "._spec.yaml"): "sidecar inside sidecar dir",
	}

	for fPath, content := range files {
		if err := fileutil.WriteFile(fPath, []byte(content), 0600); err != nil {
			t.Fatalf("failed to create file %q: %v", fPath, err)
		}
	}
}
