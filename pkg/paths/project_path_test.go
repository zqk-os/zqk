package paths

import (
	"path/filepath"
	"testing"
)

func TestProjectPath_UnderRoot(t *testing.T) {
	root := t.TempDir()
	got, err := ProjectPath(root, ProjectDataDir, "config", "config.yaml")
	if err != nil {
		t.Fatalf("ProjectPath: %v", err)
	}
	want := filepath.Join(root, ProjectDataDir, "config", "config.yaml")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestProjectPath_EscapeRejected(t *testing.T) {
	root := t.TempDir()
	_, err := ProjectPath(root, "..", "etc", "passwd")
	if err != ErrPathEscapesProject {
		t.Errorf("expected ErrPathEscapesProject, got %v", err)
	}
}

func TestUnderProjectRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub", "dir")
	tests := []struct {
		path string
		want bool
	}{
		{root, true},
		{root + string(filepath.Separator), true},
		{filepath.Join(root, ".zqk"), true},
		{sub, true},
		{filepath.Dir(root), false},
		{root + "nope", false},
	}
	for _, tt := range tests {
		got := UnderProjectRoot(root, tt.path)
		if got != tt.want {
			t.Errorf("UnderProjectRoot(%q, %q) = %v, want %v", root, tt.path, got, tt.want)
		}
	}
}
