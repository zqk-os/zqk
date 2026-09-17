package validation

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestReadLiveCASBlobFromIndex(t *testing.T) {
	dir := t.TempDir()
	kind := "question"
	id := "QUE-LIVE-1"
	hash := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	body := []byte("id: " + id + "\nkind: question\nstatus: open\n")
	if err := fileutil.WriteFile(filepath.Join(dir, hash+".yaml"), body, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	idx := []byte(`{"version":"1.0","kind":"` + kind + `","mappings":{"` + id + `":"` + hash + `"}}` + "\n")
	if err := fileutil.WriteFile(filepath.Join(dir, "."+kind+".index"), idx, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	path, data, ok := readLiveCASBlobFromIndex(dir, kind, id)
	if !ok {
		t.Fatal("expected ok")
	}
	if filepath.Base(path) != hash+".yaml" {
		t.Fatalf("path=%s", path)
	}
	if string(data) != string(body) {
		t.Fatalf("body mismatch")
	}
	if _, _, ok := readLiveCASBlobFromIndex(dir, kind, "QUE-MISSING"); ok {
		t.Fatal("expected miss")
	}
}
