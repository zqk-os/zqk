package safepath

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestJoinUnderRoot(t *testing.T) {
	tempDir := t.TempDir()

	tests := []struct {
		name        string
		root        string
		elems       []string
		expectErr   bool
		errContains string
		checkResult func(t *testing.T, result string)
	}{
		{
			name:        "empty root returns error",
			root:        "",
			elems:       []string{"file.txt"},
			expectErr:   true,
			errContains: errEmptyRoot,
		},
		{
			name:      "valid nested path",
			root:      tempDir,
			elems:     []string{"sub", "file.txt"},
			expectErr: false,
			checkResult: func(t *testing.T, result string) {
				expected := filepath.Join(tempDir, "sub", "file.txt")
				if result != expected {
					t.Errorf("expected %q, got %q", expected, result)
				}
			},
		},
		{
			name:      "internal dot dot that stays inside root",
			root:      tempDir,
			elems:     []string{"sub", "nested", "..", "file.txt"},
			expectErr: false,
			checkResult: func(t *testing.T, result string) {
				expected := filepath.Join(tempDir, "sub", "file.txt")
				if result != expected {
					t.Errorf("expected %q, got %q", expected, result)
				}
			},
		},
		{
			name:        "simple escape with parent dir marker",
			root:        tempDir,
			elems:       []string{"..", "escape.txt"},
			expectErr:   true,
			errContains: errPathEscapesRoot,
		},
		{
			name:        "multiple dot dot escape",
			root:        tempDir,
			elems:       []string{"sub", "..", "..", "escape.txt"},
			expectErr:   true,
			errContains: errPathEscapesRoot,
		},
		{
			name:        "deep escape attempt",
			root:        tempDir,
			elems:       []string{"a", "b", "c", "../../../../escape.txt"},
			expectErr:   true,
			errContains: errPathEscapesRoot,
		},
		{
			// TRACK: BLI-CEF-R15-PATH-TRAVERSAL-001 / REQ-CEF-R2-SEC-PATH-TRAVERSAL
			name:        "root relative traversal escape attempt",
			root:        tempDir,
			elems:       []string{"a/b/../../../../escape.txt"},
			expectErr:   true,
			errContains: errPathEscapesRoot,
		},
		{
			name:      "empty elems returns root",
			root:      tempDir,
			elems:     []string{},
			expectErr: false,
			checkResult: func(t *testing.T, result string) {
				absRoot, _ := filepath.Abs(tempDir)
				if result != absRoot {
					t.Errorf("expected %q, got %q", absRoot, result)
				}
			},
		},
		{
			name:      "relative root resolves properly",
			root:      ".",
			elems:     []string{"pkg", "mcp"},
			expectErr: false,
			checkResult: func(t *testing.T, result string) {
				cwd, _ := fileutil.Getwd()
				expected := filepath.Join(cwd, "pkg", "mcp")
				if result != expected {
					t.Errorf("expected %q, got %q", expected, result)
				}
			},
		},
		{
			name:        "relative root escape rejected",
			root:        ".",
			elems:       []string{"..", "sibling"},
			expectErr:   true,
			errContains: errPathEscapesRoot,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JoinUnderRoot(tt.root, tt.elems...)
			if tt.expectErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errContains)
				}
				if tt.errContains != "" && err.Error() != tt.errContains {
					t.Fatalf("expected error %q, got %q", tt.errContains, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if tt.checkResult != nil {
					tt.checkResult(t, got)
				}
			}
		})
	}
}
