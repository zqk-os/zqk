package loader

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestLoadLoaderTimeoutConfig_reloadsWhenStampMoves(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "zqk.yaml")
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
	write("component_loaders:\n  id_patterns:\n    wait_for_completion_seconds: 2\n")
	first, err := LoadLoaderTimeoutConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if first["id_patterns"].WaitForCompletion != 2*time.Second {
		t.Fatalf("got %#v", first["id_patterns"])
	}
	got := first["id_patterns"]
	got.WaitForCompletion = 99 * time.Second
	first["id_patterns"] = got
	write("component_loaders:\n  id_patterns:\n    wait_for_completion_seconds: 8\n")
	second, err := LoadLoaderTimeoutConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if second["id_patterns"].WaitForCompletion != 8*time.Second {
		t.Fatalf("expected reload after stamp move, got %#v", second["id_patterns"])
	}
}
