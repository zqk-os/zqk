// BLI-STARTER-COMMUNITY-027 / PRI-STARTER-COMMUNITY-027 coverage elevation
package diskusage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFormatBytes_NegativeAndLarge(t *testing.T) {
	t.Parallel()
	if got := FormatBytes(-5); got != "0B" {
		t.Fatalf("neg: %q", got)
	}
	if got := FormatBytes(10 * 1024); got != "10KiB" {
		t.Fatalf("ge10: %q", got)
	}
}

func TestParseSize_Edges(t *testing.T) {
	t.Parallel()
	if n, err := ParseSize(""); err != nil || n != 0 {
		t.Fatalf("empty: %d %v", n, err)
	}
	if n, err := ParseSize("  2T "); err != nil || n != 2*1024*1024*1024*1024 {
		t.Fatalf("2T: %d %v", n, err)
	}
	if _, err := ParseSize("1X"); err == nil {
		t.Fatal("invalid letter")
	}
	if _, err := ParseSize("-1"); err == nil {
		t.Fatal("negative")
	}
	if _, err := ParseSize("GiB"); err == nil {
		t.Fatal("suffix only")
	}
}

func TestScan_IncludeFilesLimitAndSymlink(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	must := func(rel string, size int) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		if err := fileutil.WriteFile(p, make([]byte, size), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}
	must("keep.bin", 200)
	must("nested/x.bin", 50)
	if err := os.Symlink(filepath.Join(root, "keep.bin"), filepath.Join(root, "link.bin")); err != nil {
		t.Fatal(err)
	}

	res, err := Scan(root, Options{MaxDepth: 1, IncludeFiles: true, Limit: 4, MinBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	clamped, err := Scan(root, Options{MaxDepth: -1})
	if err != nil {
		t.Fatal(err)
	}
	if clamped.MaxDepth != 0 {
		t.Fatalf("neg maxdepth should clamp to 0, got %d", clamped.MaxDepth)
	}
	if len(res.Entries) > 4 {
		t.Fatalf("limit: %+v", res.Entries)
	}
	if res.MinHuman == "" && res.MinBytes > 0 {
		t.Fatal("MinHuman")
	}
	foundFile := false
	for _, e := range res.Entries {
		if !e.IsDir {
			foundFile = true
		}
	}
	if !foundFile {
		t.Fatalf("expected a file entry: %+v", res.Entries)
	}
	capped, err := Scan(root, Options{MaxDepth: 2, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(capped.Entries) != 1 {
		t.Fatalf("limit 1: %+v", capped.Entries)
	}
}

func TestPathDepthAndDisplay(t *testing.T) {
	t.Parallel()
	if pathDepth("/a", "/a") != 0 {
		t.Fatal("root depth")
	}
	if pathDepth("/a", "/a/b/c") != 2 {
		t.Fatal("nested")
	}
	if pathDepth("/a", "/b") != -1 {
		t.Fatal("outside")
	}
	if displayPath("/a", "/a") != "." {
		t.Fatal("display root")
	}
	if displayPath("/a", "/a/b") != "b" {
		t.Fatal("display child")
	}
}
