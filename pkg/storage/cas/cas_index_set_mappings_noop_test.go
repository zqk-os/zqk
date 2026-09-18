package cas_test

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"testing"
)

func TestStringMapsEqual(t *testing.T) {
	t.Parallel()
	if !filecas.StringMapsEqual(nil, nil) || !filecas.StringMapsEqual(nil, map[string]string{}) {
		t.Fatal("nil/empty should equal")
	}
	if filecas.StringMapsEqual(map[string]string{"a": "1"}, map[string]string{"a": "2"}) {
		t.Fatal("different values")
	}
}

func TestSetMappings_NoOpSkipsRewrite(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "id_index.json")
	idx := &filecas.IDIndex{
		Version:  "1",
		Kind:     "criteria",
		FilePath: indexPath,
		Mappings: map[string]string{"CRIT-1": "abc"},
	}
	if err := idx.Save(); err != nil {
		t.Fatalf("seed Save: %v", err)
	}
	info1, err := fileutil.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := idx.SetMappings(map[string]string{"CRIT-1": "abc"}, nil); err != nil {
		t.Fatal(err)
	}
	info2, err := fileutil.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Fatalf("no-op SetMappings rewrote index (mtime %v -> %v)", info1.ModTime(), info2.ModTime())
	}
}

func TestSetMappings_UpdatesWhenChanged(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "id_index.json")
	idx := &filecas.IDIndex{
		Version:  "1",
		Kind:     "criteria",
		FilePath: indexPath,
		Mappings: map[string]string{"CRIT-1": "abc"},
	}
	if err := idx.SetMappings(map[string]string{"CRIT-1": "abc"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := idx.SetMappings(map[string]string{"CRIT-1": "def"}, nil); err != nil {
		t.Fatal(err)
	}
	h, err := idx.GetHash("CRIT-1")
	if err != nil || h != "def" {
		t.Fatalf("hash=%q err=%v", h, err)
	}
}
