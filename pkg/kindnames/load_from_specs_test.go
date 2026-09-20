package kindnames_test

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/kindnames"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadKindNamesFromSpecsDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := fileutil.WriteSecureFile(filepath.Join(dir, "foo.yaml"), []byte("ontology: foo_kind\n")); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteSecureFile(filepath.Join(dir, "bar.yaml"), []byte("ontology: bar_kind\n")); err != nil {
		t.Fatal(err)
	}
	got, err := kindnames.LoadKindNamesFromSpecsDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 kinds, got %d (%v)", len(got), got)
	}
	if _, ok := got["foo_kind"]; !ok {
		t.Fatalf("missing foo_kind: %v", got)
	}
	if _, ok := got["bar_kind"]; !ok {
		t.Fatalf("missing bar_kind: %v", got)
	}
}

func TestLoadKindNamesFromSpecsDir_emptyDirErrors(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	_, err := kindnames.LoadKindNamesFromSpecsDir(dir)
	if err == nil {
		t.Fatal("expected error for no ontology values")
	}
}
