package fileutil

import (
	"errors"
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
			name:        "dot dot path is rejected",
			path:        "..",
			expectError: true,
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
