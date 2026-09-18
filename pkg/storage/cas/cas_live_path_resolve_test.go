package cas

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestResolveLiveCASFilePath_FindsCurrentHashAfterStaleCachePath(t *testing.T) {
	tmp := t.TempDir()
	kind := "backlog_item"
	dirName := objects.GetDirectoryFromKind(kind)
	if dirName == "" {
		t.Fatal("expected directory for backlog_item")
	}
	kindDir := datacell.CellCASPrimaryDir(tmp, dirName)
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	id := "BLI-RESOLVE-001"
	liveHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	livePath := filepath.Join(kindDir, liveHash+".yaml")
	body := []byte("id: " + id + "\nkind: backlog_item\nstatus: planned\n")
	if err := fileutil.WriteFile(livePath, body, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	indexPath := filepath.Join(kindDir, "."+kind+".index")
	indexBody := []byte(`{"version":"1.0","kind":"` + kind + `","mappings":{"` + id + `":"` + liveHash + `"}}` + "\n")
	if err := fileutil.WriteFile(indexPath, indexBody, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	stalePath := filepath.Join(kindDir, "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.yaml")
	if !CachePathNeedsCASResolve(stalePath) {
		t.Fatal("expected missing stale path to need resolve")
	}

	got, ok := ResolveLiveCASFilePath(tmp, kind, id)
	if !ok {
		t.Fatal("expected resolve to succeed")
	}
	if got != livePath {
		t.Fatalf("got %q want %q", got, livePath)
	}
}

func TestResolveLiveCASFilePath_MissingObject(t *testing.T) {
	tmp := t.TempDir()
	kind := "backlog_item"
	dirName := objects.GetDirectoryFromKind(kind)
	kindDir := datacell.CellCASPrimaryDir(tmp, dirName)
	_ = fileutil.MkdirAll(kindDir, paths.DirPerm755)
	_ = fileutil.WriteFile(filepath.Join(kindDir, "."+kind+".index"), []byte(`{"`+objects.FieldKeyVersion+`":"1.0","`+objects.FieldKeyKind+`":"`+kind+`","mappings":{}}`+"\n"), paths.FilePerm644)

	if _, ok := ResolveLiveCASFilePath(tmp, kind, "BLI-GONE"); ok {
		t.Fatal("expected missing object to fail resolve")
	}
}
