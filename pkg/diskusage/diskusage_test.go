package diskusage

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFormatBytes(t *testing.T) {
	cases := []struct {
		n    int64
		want string
	}{
		{0, "0B"},
		{512, "512B"},
		{1024, "1.0KiB"},
		{1536, "1.5KiB"},
		{1024 * 1024, "1.0MiB"},
		{1024 * 1024 * 1024, "1.0GiB"},
	}
	for _, tc := range cases {
		if got := FormatBytes(tc.n); got != tc.want {
			t.Errorf("FormatBytes(%d)=%q want %q", tc.n, got, tc.want)
		}
	}
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"1024", 1024},
		{"1K", 1024},
		{"1M", 1024 * 1024},
		{"1G", 1024 * 1024 * 1024},
		{"1.5G", int64(1.5 * 1024 * 1024 * 1024)},
		{"500MiB", 500 * 1024 * 1024},
	}
	for _, tc := range cases {
		got, err := ParseSize(tc.in)
		if err != nil {
			t.Fatalf("ParseSize(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("ParseSize(%q)=%d want %d", tc.in, got, tc.want)
		}
	}
	if _, err := ParseSize("nope"); err == nil {
		t.Fatal("expected error for invalid size")
	}
}

func TestScanDepthAndMin(t *testing.T) {
	root := t.TempDir()
	mustMk := func(rel string, size int) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := fileutil.MkdirAll(filepath.Dir(p), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		if err := fileutil.WriteFile(p, make([]byte, size), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}
	mustMk("a/big.bin", 2*1024*1024) // 2MiB
	mustMk("a/b/small.bin", 100)
	mustMk("c/tiny.bin", 10)

	res, err := Scan(root, Options{MaxDepth: 2, MinBytes: 1024 * 1024})
	if err != nil {
		t.Fatal(err)
	}
	if res.TotalBytesRoot < 2*1024*1024 {
		t.Fatalf("total too small: %d", res.TotalBytesRoot)
	}
	// Expect root and a/ (and maybe a/b if over min — a/b is only 100 bytes, filtered).
	found := map[string]bool{}
	for _, e := range res.Entries {
		found[e.Path] = true
		if !e.IsDir {
			t.Errorf("unexpected file entry %s", e.Path)
		}
	}
	if !found["."] {
		t.Fatalf("missing root entry: %+v", res.Entries)
	}
	if !found["a"] {
		t.Fatalf("missing a/: %+v", res.Entries)
	}
	if found["a/b"] {
		t.Fatalf("a/b should be under min: %+v", res.Entries)
	}
	if found["c"] {
		t.Fatalf("c should be under min: %+v", res.Entries)
	}
}
