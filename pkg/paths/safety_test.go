package paths

import (
	"path/filepath"
	"testing"
)

func TestMustNotDestroyProjectRoot(t *testing.T) {
	proj := "/test/project/root"

	tests := []struct {
		name       string
		target     string
		expectFail bool
	}{
		{"safe subfolder", filepath.Join(proj, "foo"), false},
		{"exact match", proj, true},
		{"cleaned exact match", proj + "/", true},
		{"zqk metadata", filepath.Join(proj, ".zqk"), true},
		{"git metadata", filepath.Join(proj, ".git"), true},
		{"zqk subfolder safe", filepath.Join(proj, ".zqk", "worktrees", "ATK-1"), false},
		{"empty project", "", true},
		{"empty target", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MustNotDestroyProjectRoot(proj, tt.target)
			if tt.expectFail && err == nil {
				t.Errorf("expected error for %s, got nil", tt.target)
			}
			if !tt.expectFail && err != nil {
				t.Errorf("expected success for %s, got %v", tt.target, err)
			}
		})
	}
}
