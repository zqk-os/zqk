package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadBlockingCheckConfig_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), paths.BlockingCheckConfigFile)
	write := func(body string) {
		t.Helper()
		if err := fileutil.WriteFile(path, []byte(body), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
		later := time.Now().Add(2 * time.Second)
		if err := fileutil.Chtimes(path, later, later); err != nil {
			t.Fatal(err)
		}
	}
	write("bypass_kinds:\n  - audit_event\n")
	first, err := LoadBlockingCheckConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := first.GetBypassKinds(); len(got) != 1 || got[0] != "audit_event" {
		t.Fatalf("got %#v", got)
	}
	first.BypassKinds[0] = "mutated"
	write("bypass_kinds:\n  - audit_event\n  - scheduler_job\n")
	second, err := LoadBlockingCheckConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.GetBypassKinds(); len(got) != 2 {
		t.Fatalf("expected reload after stamp move, got %#v", got)
	}
}
