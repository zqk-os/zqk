package paths

import (
	"path/filepath"
	"testing"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestIndexYAMLNames_skipsDotAndAppleDouble(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "kernel")
	if err := fileutil.EnsureDir(nested); err != nil {
		t.Fatal(err)
	}
	account := filepath.Join(dir, "account.yaml")
	role := filepath.Join(nested, "role.yaml")
	if err := fileutil.WriteStandardFile(account, []byte("ontology: account\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(role, []byte("ontology: role\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(filepath.Join(dir, "._account.yaml"), []byte("garbage\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(filepath.Join(dir, ".hidden.yaml"), []byte("hidden\n")); err != nil {
		t.Fatal(err)
	}

	idx := IndexYAMLNames(dir)
	if _, ok := idx["._account.yaml"]; ok {
		t.Fatal("AppleDouble sidecar must not be indexed")
	}
	if _, ok := idx[".hidden.yaml"]; ok {
		t.Fatal("dotfile YAML must not be indexed")
	}
	if idx["account.yaml"] != account {
		t.Fatalf("account.yaml got %q", idx["account.yaml"])
	}
	if idx["account"] != account {
		t.Fatalf("ontology account got %q", idx["account"])
	}
	if idx["role.yaml"] != role {
		t.Fatalf("role.yaml got %q", idx["role.yaml"])
	}
}
