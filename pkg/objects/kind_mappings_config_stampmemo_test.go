package objects

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func writeKindMappingsStampMoved(t *testing.T, path, body string) {
	t.Helper()
	if err := fileutil.WriteFile(path, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := fileutil.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func TestLoadKindMappingsConfig_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), paths.KindMappingsConfigFile)
	writeKindMappingsStampMoved(t, path, `
backends:
  default:
    kind_to_directory:
      backlog_item: backlog
`)
	first, err := LoadKindMappingsConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if first.Backends[backendDefaultKey].KindToDirectory["backlog_item"] != "backlog" {
		t.Fatalf("got %#v", first.Backends)
	}
	first.Backends[backendDefaultKey].KindToDirectory["backlog_item"] = "mutated"
	writeKindMappingsStampMoved(t, path, `
backends:
  default:
    kind_to_directory:
      backlog_item: backlog_items
`)
	second, err := LoadKindMappingsConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if second.Backends[backendDefaultKey].KindToDirectory["backlog_item"] != "backlog_items" {
		t.Fatalf("expected reload after stamp move, got %#v", second.Backends)
	}
	if first.Backends[backendDefaultKey].KindToDirectory["backlog_item"] != "mutated" {
		t.Fatal("clone should isolate caller mutation from later loads")
	}
}
