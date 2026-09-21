package internal

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadYAMLDoc_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.yaml")
	write := func(body string) {
		t.Helper()
		if err := fileutil.WriteFile(path, []byte(body), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}
	write("object_type: one\n")
	first, err := loadYAMLDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := first["object_type"].(string); got != "one" {
		t.Fatalf("got %#v", first)
	}
	write("object_type: two\n")
	later := time.Now().Add(2 * time.Second)
	if err := fileutil.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	second, err := loadYAMLDoc(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := second["object_type"].(string); got != "two" {
		t.Fatalf("expected reload after stamp move, got %#v", second)
	}
}
