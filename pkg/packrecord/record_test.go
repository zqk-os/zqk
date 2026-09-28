package packrecord

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestInstallRecordsFormalSpec(t *testing.T) {
	src := writePack(t, "widget-pack", "widget")
	root := t.TempDir()

	got, err := Install(src, root)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "widget-pack" || len(got.Kinds) != 1 || got.Kinds[0] != "widget" {
		t.Fatalf("manifest = %+v", got)
	}
	recorded := filepath.Join(recordDir(root, "widget-pack"), specSubdir, "widget.yaml")
	data, err := os.ReadFile(recorded)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "ontology: widget") {
		t.Fatalf("recorded spec = %s", data)
	}
	if !rootListed(filepath.Join(recordDir(root, "widget-pack"), specSubdir)) {
		t.Fatal("recorded spec dir was not registered")
	}
}

func TestInstallRejectsMismatchedOntology(t *testing.T) {
	src := writePack(t, "widget-pack", "widget")
	if err := os.WriteFile(filepath.Join(src, specSubdir, "widget.yaml"), []byte("ontology: other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(src, t.TempDir()); err == nil {
		t.Fatal("expected ontology mismatch")
	}
}

func TestLoadRegistersRecordedPack(t *testing.T) {
	src := writePack(t, "widget-pack", "widget")
	root := t.TempDir()
	if _, err := Install(src, root); err != nil {
		t.Fatal(err)
	}
	objects.RemoveSpecRoot(filepath.Join(recordDir(root, "widget-pack"), specSubdir))

	if err := Load(root); err != nil {
		t.Fatal(err)
	}
	if !rootListed(filepath.Join(recordDir(root, "widget-pack"), specSubdir)) {
		t.Fatal("load did not register the recorded spec dir")
	}
}

func writePack(t *testing.T, name, kind string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, specSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, lifecycleSubdir), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "name: " + name + "\nkinds:\n  - " + kind + "\n"
	if err := os.WriteFile(filepath.Join(dir, manifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := "ontology: " + kind + "\ndescription: uploaded kind\n"
	if err := os.WriteFile(filepath.Join(dir, specSubdir, kind+".yaml"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	life := "name: " + kind + "\nstatuses:\n  - name: originated\n"
	if err := os.WriteFile(filepath.Join(dir, lifecycleSubdir, kind+"_lifecycle.yaml"), []byte(life), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func rootListed(dir string) bool {
	for _, root := range objects.ExtraSpecRoots() {
		if root == dir {
			return true
		}
	}
	return false
}
