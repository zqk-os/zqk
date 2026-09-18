package operational

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestParseFilesystemSnapshotScope(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		in   string
		want FilesystemSnapshotScope
	}{
		{"", FSSnapshotScopeFull},
		{"full", FSSnapshotScopeFull},
		{"Zqk", FSSnapshotScopeZqk},
		{"zqk-and-docs", FSSnapshotScopeZqkAndDocs},
	} {
		got, err := ParseFilesystemSnapshotScope(tc.in)
		if err != nil || got != tc.want {
			t.Fatalf("ParseFilesystemSnapshotScope(%q) = %v, %v want %v, nil", tc.in, got, err, tc.want)
		}
	}
	if _, err := ParseFilesystemSnapshotScope("nope"); err == nil {
		t.Fatal("expected error for invalid scope")
	}
}

func TestRunFilesystemProjectSnapshot_ScopeZqk(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_ = fileutil.MkdirAll(filepath.Join(root, paths.ProjectDataDir, "logs", "d"), paths.DirPerm750)
	_ = fileutil.WriteFile(filepath.Join(root, paths.ProjectDataDir, "logs", "d", "a.jsonl"), []byte("x"), paths.FilePerm600)
	_ = fileutil.MkdirAll(filepath.Join(root, "pkg", "p"), paths.DirPerm750)
	_ = fileutil.WriteFile(filepath.Join(root, "pkg", "p", "z.go"), []byte("p"), paths.FilePerm600)

	full, err := RunFilesystemProjectSnapshot(context.Background(), root, FSSnapshotScopeFull)
	if err != nil {
		t.Fatal(err)
	}
	zqkOnly, err := RunFilesystemProjectSnapshot(context.Background(), root, FSSnapshotScopeZqk)
	if err != nil {
		t.Fatal(err)
	}
	if zqkOnly.TotalFiles >= full.TotalFiles {
		t.Fatalf("zqk scope should count fewer files than full: zqk=%d full=%d", zqkOnly.TotalFiles, full.TotalFiles)
	}
	if zqkOnly.TotalFiles != 1 {
		t.Fatalf("zqk scope TotalFiles=%d want 1", zqkOnly.TotalFiles)
	}
	if zqkOnly.SnapshotScope != "zqk" {
		t.Fatalf("SnapshotScope=%q", zqkOnly.SnapshotScope)
	}
}
