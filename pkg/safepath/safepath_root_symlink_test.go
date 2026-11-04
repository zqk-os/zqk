// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 ZQK Authors. All rights reserved.
// Use of this source code is governed by an Apache-2.0 license.

package safepath

import (
	"path/filepath"
	"strings"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestCycle175_EmptyRootValidation verifies that empty root inputs fail closed.
func TestCycle175_EmptyRootValidation(t *testing.T) {
	testCases := []struct {
		name  string
		root  string
		elems []string
	}{
		{name: "empty-root-no-elems", root: "", elems: nil},
		{name: "empty-root-empty-elems", root: "", elems: []string{""}},
		{name: "empty-root-single-elem", root: "", elems: []string{"data.json"}},
		{name: "empty-root-nested-elems", root: "", elems: []string{"nested", "dir", "file.txt"}},
		{name: "empty-root-traversal-elem", root: "", elems: []string{"..", "etc", "passwd"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := JoinUnderRoot(tc.root, tc.elems...)
			if err == nil {
				t.Fatalf("JoinUnderRoot(%q, %v) succeeded unexpectedly, got %q", tc.root, tc.elems, result)
			}
			if result != "" {
				t.Fatalf("JoinUnderRoot(%q, %v) returned non-empty path on error: %q", tc.root, tc.elems, result)
			}
			if err.Error() != errEmptyRoot {
				t.Fatalf("JoinUnderRoot error = %q; want %q", err.Error(), errEmptyRoot)
			}
		})
	}
}

// TestCycle175_StandardValidJoins validates standard subpath confinement within root boundaries.
func TestCycle175_StandardValidJoins(t *testing.T) {
	tempRoot := t.TempDir()

	testCases := []struct {
		name     string
		elems    []string
		expected string
	}{
		{
			name:     "no-elems-returns-root",
			elems:    nil,
			expected: tempRoot,
		},
		{
			name:     "single-file",
			elems:    []string{"config.yaml"},
			expected: filepath.Join(tempRoot, "config.yaml"),
		},
		{
			name:     "nested-subdirectories",
			elems:    []string{"var", "log", "zqk", "audit.log"},
			expected: filepath.Join(tempRoot, "var", "log", "zqk", "audit.log"),
		},
		{
			name:     "redundant-separators",
			elems:    []string{"var///data////storage", "meta.db"},
			expected: filepath.Join(tempRoot, "var", "data", "storage", "meta.db"),
		},
		{
			name:     "current-dir-markers",
			elems:    []string{".", "a", ".", "b", "c.txt"},
			expected: filepath.Join(tempRoot, "a", "b", "c.txt"),
		},
		{
			name:     "legitimate-internal-backtrack",
			elems:    []string{"pkg", "test", "..", "core", "runtime.go"},
			expected: filepath.Join(tempRoot, "pkg", "core", "runtime.go"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := JoinUnderRoot(tempRoot, tc.elems...)
			if err != nil {
				t.Fatalf("JoinUnderRoot(%q, %v) failed: %v", tempRoot, tc.elems, err)
			}
			if got != tc.expected {
				t.Fatalf("JoinUnderRoot got %q; want %q", got, tc.expected)
			}
		})
	}
}

// TestCycle175_TrailingSlashesClean verifies root and element trailing slash normalization.
func TestCycle175_TrailingSlashesClean(t *testing.T) {
	tempRoot := t.TempDir()
	rootWithSlashes := tempRoot + string(fileutil.PathSeparator) + string(fileutil.PathSeparator)

	got, err := JoinUnderRoot(rootWithSlashes, "sub/", "file.txt/")
	if err != nil {
		t.Fatalf("JoinUnderRoot trailing slash failed: %v", err)
	}

	expected := filepath.Join(tempRoot, "sub", "file.txt")
	if got != expected {
		t.Fatalf("JoinUnderRoot got %q; want %q", got, expected)
	}
}

// TestCycle175_RelativeRootResolution ensures relative roots are converted to absolute paths.
func TestCycle175_RelativeRootResolution(t *testing.T) {
	cwd, err := fileutil.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	testCases := []struct {
		name     string
		root     string
		elems    []string
		wantPath string
	}{
		{
			name:     "relative-dot",
			root:     ".",
			elems:    []string{"pkg", "safepath"},
			wantPath: filepath.Join(cwd, "pkg", "safepath"),
		},
		{
			name:     "relative-subpath",
			root:     filepath.Join(".", "pkg"),
			elems:    []string{"safepath", "join.go"},
			wantPath: filepath.Join(cwd, "pkg", "safepath", "join.go"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, rErr := JoinUnderRoot(tc.root, tc.elems...)
			if rErr != nil {
				t.Fatalf("JoinUnderRoot(%q, %v) returned unexpected error: %v", tc.root, tc.elems, rErr)
			}
			if got != tc.wantPath {
				t.Fatalf("JoinUnderRoot got %q; want %q", got, tc.wantPath)
			}
		})
	}
}

// TestCycle175_LexicalTraversalPrevention validates rejection of parent directory escapes.
func TestCycle175_LexicalTraversalPrevention(t *testing.T) {
	tempRoot := t.TempDir()

	testCases := []struct {
		name  string
		elems []string
	}{
		{name: "single-parent-marker", elems: []string{".."}},
		{name: "parent-marker-with-file", elems: []string{"..", "escape.txt"}},
		{name: "subfolder-double-parent", elems: []string{"sub", "..", ".."}},
		{name: "nested-deep-escape", elems: []string{"a", "b", "c", "..", "..", "..", ".."}},
		{name: "direct-path-traversal-string", elems: []string{"../../../../etc/shadow"}},
		{name: "mixed-separators-escape", elems: []string{"folder/../../escaped"}},
		{name: "leading-slash-traversal", elems: []string{"/../../root-escape"}},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := JoinUnderRoot(tempRoot, tc.elems...)
			if err == nil {
				t.Fatalf("expected error for traversal %v, got %q", tc.elems, got)
			}
			if err.Error() != errPathEscapesRoot {
				t.Fatalf("expected error %q, got %q", errPathEscapesRoot, err.Error())
			}
			if got != "" {
				t.Fatalf("expected empty string on error, got %q", got)
			}
		})
	}
}

// TestCycle175_SymlinkWithinRootAllowed verifies that symlinks pointing inside root are permitted.
func TestCycle175_SymlinkWithinRootAllowed(t *testing.T) {
	root := t.TempDir()
	targetDir := filepath.Join(root, "target_dir")
	if err := fileutil.MkdirAll(targetDir, 0755); err != nil {
		t.Fatalf("failed to create target directory: %v", err)
	}

	targetFile := filepath.Join(targetDir, "data.txt")
	if err := fileutil.WriteFile(targetFile, []byte("contained payload"), 0644); err != nil {
		t.Fatalf("failed to write target file: %v", err)
	}

	// Symlink to file inside root
	fileSymlink := filepath.Join(root, "file_link.txt")
	if err := fileutil.Symlink(targetFile, fileSymlink); err != nil {
		t.Fatalf("failed to create file symlink: %v", err)
	}

	gotFile, err := JoinUnderRoot(root, "file_link.txt")
	if err != nil {
		t.Fatalf("JoinUnderRoot failed for contained file symlink: %v", err)
	}
	if gotFile != fileSymlink {
		t.Fatalf("JoinUnderRoot returned %q; want %q", gotFile, fileSymlink)
	}

	// Symlink to directory inside root
	dirSymlink := filepath.Join(root, "dir_link")
	if err := fileutil.Symlink(targetDir, dirSymlink); err != nil {
		t.Fatalf("failed to create directory symlink: %v", err)
	}

	gotDirChild, err := JoinUnderRoot(root, "dir_link", "data.txt")
	if err != nil {
		t.Fatalf("JoinUnderRoot failed for contained directory symlink child: %v", err)
	}
	expectedChild := filepath.Join(root, "dir_link", "data.txt")
	if gotDirChild != expectedChild {
		t.Fatalf("JoinUnderRoot returned %q; want %q", gotDirChild, expectedChild)
	}
}

// TestCycle175_SymlinkOutsideRootBlocked verifies symlink target escape defenses (F-SEC-006).
func TestCycle175_SymlinkOutsideRootBlocked(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	outsideFile := filepath.Join(outside, "secret.key")
	if err := fileutil.WriteFile(outsideFile, []byte("sensitive-data"), 0600); err != nil {
		t.Fatalf("failed to write outside file: %v", err)
	}

	// Direct symlink to outside file
	evilFileLink := filepath.Join(root, "evil_file.key")
	if err := fileutil.Symlink(outsideFile, evilFileLink); err != nil {
		t.Fatalf("failed to create evil file symlink: %v", err)
	}

	got, err := JoinUnderRoot(root, "evil_file.key")
	if err == nil {
		t.Fatalf("expected error for symlink pointing outside root, got %q", got)
	}
	if err.Error() != errPathEscapesRoot {
		t.Fatalf("expected error %q, got %q", errPathEscapesRoot, err.Error())
	}

	// Symlink to outside directory
	evilDirLink := filepath.Join(root, "evil_dir")
	if err := fileutil.Symlink(outside, evilDirLink); err != nil {
		t.Fatalf("failed to create evil dir symlink: %v", err)
	}

	gotChild, err := JoinUnderRoot(root, "evil_dir", "secret.key")
	if err == nil {
		t.Fatalf("expected error for symlink traversing outside directory, got %q", gotChild)
	}
	if err.Error() != errPathEscapesRoot {
		t.Fatalf("expected error %q, got %q", errPathEscapesRoot, err.Error())
	}
}

// TestCycle175_ChainedSymlinkEscape verifies multi-hop symlink escape detection.
func TestCycle175_ChainedSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	outsideTarget := filepath.Join(outside, "target.txt")
	if err := fileutil.WriteFile(outsideTarget, []byte("secret"), 0644); err != nil {
		t.Fatalf("failed to write target file: %v", err)
	}

	hop2 := filepath.Join(root, "hop2.txt")
	if err := fileutil.Symlink(outsideTarget, hop2); err != nil {
		t.Fatalf("failed to create hop2 symlink: %v", err)
	}

	hop1 := filepath.Join(root, "hop1.txt")
	if err := fileutil.Symlink(hop2, hop1); err != nil {
		t.Fatalf("failed to create hop1 symlink: %v", err)
	}

	got, err := JoinUnderRoot(root, "hop1.txt")
	if err == nil {
		t.Fatalf("expected error for chained symlink escape, got %q", got)
	}
	if err.Error() != errPathEscapesRoot {
		t.Fatalf("expected error %q, got %q", errPathEscapesRoot, err.Error())
	}
}

// TestCycle175_BrokenSymlinkHandling verifies that non-existent symlink targets are safely handled.
func TestCycle175_BrokenSymlinkHandling(t *testing.T) {
	root := t.TempDir()
	brokenLink := filepath.Join(root, "broken.txt")
	nonExistentTarget := filepath.Join(root, "does_not_exist.txt")

	if err := fileutil.Symlink(nonExistentTarget, brokenLink); err != nil {
		t.Fatalf("failed to create broken symlink: %v", err)
	}

	// Lexically within root, but target doesn't exist on disk
	got, err := JoinUnderRoot(root, "broken.txt")
	if err != nil {
		t.Fatalf("JoinUnderRoot failed on broken symlink within root: %v", err)
	}
	if got != brokenLink {
		t.Fatalf("JoinUnderRoot returned %q; want %q", got, brokenLink)
	}
}

// TestCycle175_SymlinkRootResolution verifies behavior when root directory itself is a symlink.
func TestCycle175_SymlinkRootResolution(t *testing.T) {
	realRoot := t.TempDir()
	linkParent := t.TempDir()
	symRoot := filepath.Join(linkParent, "sym_root")

	if err := fileutil.Symlink(realRoot, symRoot); err != nil {
		t.Fatalf("failed to create symlink root: %v", err)
	}

	// Case 1: normal child inside symlink root
	got, err := JoinUnderRoot(symRoot, "nested", "file.txt")
	if err != nil {
		t.Fatalf("JoinUnderRoot through symlink root failed: %v", err)
	}
	expected := filepath.Join(symRoot, "nested", "file.txt")
	if got != expected {
		t.Fatalf("JoinUnderRoot returned %q; want %q", got, expected)
	}

	// Case 2: child escaping symlink root lexically
	_, err = JoinUnderRoot(symRoot, "..", "escaped.txt")
	if err == nil {
		t.Fatalf("expected error escaping symlink root")
	}
	if err.Error() != errPathEscapesRoot {
		t.Fatalf("expected error %q, got %q", errPathEscapesRoot, err.Error())
	}
}

// TestCycle175_AbsoluteChildElements verifies that absolute subpath elements remain rooted.
func TestCycle175_AbsoluteChildElements(t *testing.T) {
	tempRoot := t.TempDir()

	// filepath.Join(root, "/absolute/path") treats "/absolute/path" relative to root on unix
	got, err := JoinUnderRoot(tempRoot, "/opt/data/file.txt")
	if err != nil {
		t.Fatalf("JoinUnderRoot absolute elem failed: %v", err)
	}

	cleanRel := filepath.Clean("/opt/data/file.txt")
	cleanRel = strings.TrimPrefix(cleanRel, string(fileutil.PathSeparator))
	expected := filepath.Join(tempRoot, cleanRel)
	if got != expected {
		t.Fatalf("JoinUnderRoot got %q; want %q", got, expected)
	}
}
