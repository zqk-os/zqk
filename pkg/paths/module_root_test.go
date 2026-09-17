package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModuleRootFromPath_findsGoMod(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, goModFileName), []byte("module example.test\n"), FilePerm644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "pkg", "specbuilder", "profile_builders")
	if err := os.MkdirAll(nested, DirPerm755); err != nil {
		t.Fatal(err)
	}
	got, err := ModuleRootFromPath(nested)
	if err != nil {
		t.Fatalf("ModuleRootFromPath: %v", err)
	}
	want, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestObjectsFieldKeysGoPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	got, err := ObjectsFieldKeysGoPath(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, PkgDir, ObjectsPackageDir, FieldKeysGoFile)
	wantAbs, err := filepath.Abs(want)
	if err != nil {
		t.Fatal(err)
	}
	if got != wantAbs {
		t.Fatalf("got %q want %q", got, wantAbs)
	}
}

func TestModuleImportPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	goMod := `module example.test/foo

go 1.22
`
	if err := os.WriteFile(filepath.Join(root, goModFileName), []byte(goMod), FilePerm644); err != nil {
		t.Fatal(err)
	}
	got, err := ModuleImportPath(root)
	if err != nil {
		t.Fatalf("ModuleImportPath: %v", err)
	}
	if got != "example.test/foo" {
		t.Fatalf("got %q want example.test/foo", got)
	}
}

func TestSpecbuilderPackageImportPath(t *testing.T) {
	t.Parallel()
	got := SpecbuilderPackageImportPath("example.test/mod", "routing_builders")
	want := "example.test/mod/pkg/specbuilder/routing_builders"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
