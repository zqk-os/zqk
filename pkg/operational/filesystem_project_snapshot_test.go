package operational

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRunFilesystemProjectSnapshot_Buckets(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_ = fileutil.MkdirAll(filepath.Join(root, paths.ProjectDataDir, "logs", "a"), paths.DirPerm750)
	_ = fileutil.WriteFile(filepath.Join(root, paths.ProjectDataDir, "logs", "a", "x.jsonl"), []byte("hi"), paths.FilePerm600)
	_ = fileutil.MkdirAll(filepath.Join(root, "docs"), paths.DirPerm750)
	_ = fileutil.WriteFile(filepath.Join(root, "docs", "readme.md"), []byte("doc"), paths.FilePerm600)
	_ = fileutil.MkdirAll(filepath.Join(root, "pkg", "x"), paths.DirPerm750)
	_ = fileutil.WriteFile(filepath.Join(root, "pkg", "x", "y.go"), []byte("package x"), paths.FilePerm600)

	ctx := context.Background()
	snap, err := RunFilesystemProjectSnapshot(ctx, root, FSSnapshotScopeFull)
	if err != nil {
		t.Fatal(err)
	}
	if snap.TotalFiles != 3 {
		t.Fatalf("TotalFiles=%d want 3", snap.TotalFiles)
	}
	if snap.ByTopLevel[paths.ProjectDataDir] == nil || snap.ByTopLevel[paths.ProjectDataDir].Files != 1 {
		t.Fatalf(".zqk bucket: %+v", snap.ByTopLevel[paths.ProjectDataDir])
	}
	if snap.ZqkByChild["logs"] == nil || snap.ZqkByChild["logs"].Files != 1 {
		t.Fatalf("zqk logs: %+v", snap.ZqkByChild["logs"])
	}
	if snap.ByTopLevel["docs"].Files != 1 || snap.ByTopLevel["pkg"].Files != 1 {
		t.Fatalf("docs/pkg: %+v %+v", snap.ByTopLevel["docs"], snap.ByTopLevel["pkg"])
	}
	if runtime.GOOS != "windows" {
		if snap.Volume == nil || snap.Volume.Source != "statfs" || snap.Volume.TotalBytes == 0 {
			t.Fatalf("expected statfs volume stats on unix, got %+v", snap.Volume)
		}
	}
}

func TestRunFilesystemProjectSnapshot_SkipsVendor(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	_ = fileutil.WriteFile(filepath.Join(root, "ok.txt"), []byte("x"), paths.FilePerm600)
	_ = fileutil.MkdirAll(filepath.Join(root, "vendor", "v"), paths.DirPerm750)
	_ = fileutil.WriteFile(filepath.Join(root, "vendor", "v", "skip.go"), []byte("x"), paths.FilePerm600)

	snap, err := RunFilesystemProjectSnapshot(context.Background(), root, FSSnapshotScopeFull)
	if err != nil {
		t.Fatal(err)
	}
	if snap.TotalFiles != 1 {
		t.Fatalf("TotalFiles=%d want 1 (vendor skipped)", snap.TotalFiles)
	}
}
