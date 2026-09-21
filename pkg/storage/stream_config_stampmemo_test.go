package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func writeHVStampMoved(t *testing.T, path, body string) {
	t.Helper()
	if err := fileutil.WriteFile(path, []byte(body), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(2 * time.Second)
	if err := fileutil.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func TestMemoizedHighVolumeKindMaps_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), paths.HighVolumeKindsConfigFile)
	writeHVStampMoved(t, path, `
kinds:
  - kind: audit_event
    storage: stream
`)
	first := memoizedHighVolumeKindMaps(path)
	if !first.stream["audit_event"] {
		t.Fatalf("got stream %#v", first.stream)
	}
	first.stream["audit_event"] = false
	writeHVStampMoved(t, path, `
kinds:
  - kind: audit_event
    storage: stream
  - kind: zqk_session
    storage: stream
`)
	second := memoizedHighVolumeKindMaps(path)
	if !second.stream["zqk_session"] {
		t.Fatalf("expected reload after stamp move, got %#v", second.stream)
	}
	if first.stream["audit_event"] {
		t.Fatal("clone should isolate caller mutation from later loads")
	}
}
