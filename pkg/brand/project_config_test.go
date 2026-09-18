package brand

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromProject_localWinsOverCommitted(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	committed := "brand:\n  executable_name: zqk\n  product_name: ZQK\n"
	if err := os.WriteFile(filepath.Join(root, "config", "zqk.yaml"), []byte(committed), 0o644); err != nil {
		t.Fatal(err)
	}
	local := "brand:\n  executable_name: zcom\n"
	if err := os.WriteFile(filepath.Join(root, "config", "zqk-local.yaml"), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadFromProject(root)
	if got.ExecutableName != "zcom" {
		t.Fatalf("local should win: got %q", got.ExecutableName)
	}
}

func TestLoadFromProject_committedWhenLocalHasNoBrand(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "zqk.yaml"), []byte("brand:\n  executable_name: zqk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "zqk-local.yaml"), []byte("kernel_state:\n  project_root: \".\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadFromProject(root)
	if got.ExecutableName != "zqk" {
		t.Fatalf("committed brand: got %q", got.ExecutableName)
	}
}

func TestLoadFromProject_ignoresLeftoverWhenCanonicalPresent(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".zqk", "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "config", "zqk.yaml"), []byte("brand:\n  executable_name: zqk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".zqk", "config", "config.yaml"), []byte("brand:\n  executable_name: leftover\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadFromProject(root)
	if got.ExecutableName != "zqk" {
		t.Fatalf("leftover config.yaml must not win: got %q", got.ExecutableName)
	}
}
