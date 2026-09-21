package paths

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFirstExistingFromCwd(t *testing.T) {
	root := t.TempDir()
	rel := filepath.Join(ConfigDir, ZqkConfigFileName)
	want := filepath.Join(root, rel)
	if err := fileutil.EnsureDir(filepath.Dir(want)); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(want, []byte("validation: {}\n")); err != nil {
		t.Fatal(err)
	}
	old, err := fileutil.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = fileutil.Chdir(old)
		ResetCwdDiscovery()
	})
	if err := fileutil.Chdir(root); err != nil {
		t.Fatal(err)
	}
	ResetCwdDiscovery()
	got := FirstExistingFromCwd(rel)
	wantResolved, err := filepath.EvalSymlinks(want)
	if err != nil {
		wantResolved = want
	}
	gotResolved, err := filepath.EvalSymlinks(got)
	if err != nil {
		gotResolved = got
	}
	if gotResolved != wantResolved {
		t.Fatalf("got %q want %q", got, want)
	}
}
