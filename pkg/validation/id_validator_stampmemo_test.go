package validation

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestParseSpecFile_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "stampmemo_id.yaml")
	write := func(prefix string) {
		t.Helper()
		body := "ontology: stampmemo_id_kind\nid_prefixes: [" + prefix + "]\n"
		if err := fileutil.WriteFile(path, []byte(body), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}
	write("AAA-")
	v := NewIDValidator(dir)
	first, err := v.parseSpecFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if first == nil || len(first.Prefixes) == 0 || first.Prefixes[0] != "AAA-" {
		t.Fatalf("got %#v", first)
	}
	write("BBB-")
	later := time.Now().Add(2 * time.Second)
	if err := fileutil.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	second, err := v.parseSpecFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if second == nil || len(second.Prefixes) == 0 || second.Prefixes[0] != "BBB-" {
		t.Fatalf("expected reload after stamp move, got %#v", second)
	}
}
