package fileutil

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestValidateSafePath(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		path        string
		expectError bool
	}{
		{
			name:        "clean standard relative path",
			path:        "foo/bar/baz.txt",
			expectError: false,
		},
		{
			name:        "clean standard absolute path",
			path:        "/Users/user/project/file.go",
			expectError: false,
		},
		{
			name:        "path with inner hyphens is permitted",
			path:        "my-cool-project/sub-dir/file-name.txt",
			expectError: false,
		},
		{
			name:        "empty path is permitted",
			path:        "",
			expectError: false,
		},
		{
			name:        "dot path is permitted",
			path:        ".",
			expectError: false,
		},
		{
			name:        "dot dot path is permitted",
			path:        "..",
			expectError: false,
		},
		{
			name:        "double hyphen flag path is rejected",
			path:        "--help",
			expectError: true,
		},
		{
			name:        "single hyphen flag path is rejected",
			path:        "-rf",
			expectError: true,
		},
		{
			name:        "nested double hyphen segment is rejected",
			path:        "project/--help/subfile",
			expectError: true,
		},
		{
			name:        "relative prefix with flag segment is rejected",
			path:        "./--help",
			expectError: true,
		},
		{
			name:        "absolute path with flag segment is rejected",
			path:        "/tmp/--version/test",
			expectError: true,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateSafePath(tc.path)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error for path %q, got nil", tc.path)
				}
				if !errors.Is(err, ErrUnsafeFlagPath) {
					t.Fatalf("expected ErrUnsafeFlagPath, got: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error for path %q: %v", tc.path, err)
				}
			}
		})
	}
}

func TestFS_PathSanitization(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()

	// 1. Unsafe flag-like paths are rejected
	unsafePath := filepath.Join(tmpDir, "--unsafe-flag-injection")
	if err := WriteFile(unsafePath, []byte("data"), 0644); !errors.Is(err, ErrUnsafeFlagPath) {
		t.Errorf("WriteFile: expected ErrUnsafeFlagPath, got: %v", err)
	}
	if err := MkdirAll(unsafePath, 0755); !errors.Is(err, ErrUnsafeFlagPath) {
		t.Errorf("MkdirAll: expected ErrUnsafeFlagPath, got: %v", err)
	}
	if err := Chmod(unsafePath, 0600); !errors.Is(err, ErrUnsafeFlagPath) {
		t.Errorf("Chmod: expected ErrUnsafeFlagPath, got: %v", err)
	}
	if _, err := ReadFile(unsafePath); !errors.Is(err, ErrUnsafeFlagPath) {
		t.Errorf("ReadFile: expected ErrUnsafeFlagPath, got: %v", err)
	}

	// 2. Redundant uncleaned path segments are cleaned and work correctly
	targetDir := filepath.Join(tmpDir, "sub", "dir")
	uncleanedDir := filepath.Join(tmpDir, "sub", "other", "..", "dir")
	if err := MkdirAll(uncleanedDir, 0755); err != nil {
		t.Fatalf("MkdirAll failed with uncleaned path: %v", err)
	}
	if !Exists(targetDir) {
		t.Fatalf("targetDir does not exist after MkdirAll with uncleaned path")
	}

	targetFile := filepath.Join(targetDir, "sample.txt")
	uncleanedFile := filepath.Join(targetDir, ".", "nested", "..", "sample.txt")
	if err := WriteFile(uncleanedFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("WriteFile failed with uncleaned path: %v", err)
	}
	if !Exists(targetFile) {
		t.Fatalf("targetFile does not exist after WriteFile with uncleaned path")
	}

	data, err := ReadFile(uncleanedFile)
	if err != nil {
		t.Fatalf("ReadFile failed with uncleaned path: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("ReadFile returned unexpected data: %q", string(data))
	}

	if err := Chmod(uncleanedFile, 0600); err != nil {
		t.Fatalf("Chmod failed with uncleaned path: %v", err)
	}

	// 3. Empty paths preserve native os error semantics (not converted to ".")
	if err := WriteFile("", []byte("empty"), 0644); err == nil {
		t.Errorf("WriteFile(\"\") expected error, got nil")
	}
	if _, err := ReadFile(""); err == nil {
		t.Errorf("ReadFile(\"\") expected error, got nil")
	}
	if err := Chmod("", 0600); err == nil {
		t.Errorf("Chmod(\"\") expected error, got nil")
	}
}
