// BLI-STARTER-COMMUNITY-039 / PRI-STARTER-COMMUNITY-039 coverage elevation
package localci

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func fakeGit(t *testing.T) GitRun {
	t.Helper()
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		switch {
		case strings.Contains(joined, "rev-parse --verify"):
			return []byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n"), nil
		case strings.Contains(joined, "rev-parse --short"):
			return []byte("aaaaaaa\n"), nil
		case strings.Contains(joined, "status --porcelain"):
			return []byte(""), nil
		case len(args) > 0 && args[0] == "archive":
			for i, a := range args {
				if a == "-o" && i+1 < len(args) {
					_ = fileutil.WriteFile(args[i+1], []byte("tar"), paths.FilePerm644)
				}
			}
			return nil, nil
		default:
			return nil, nil
		}
	}
}

func TestExtraLocalCIOptionsAndCheckout(t *testing.T) {
	ctx := context.Background()
	if _, err := Checkout(ctx, Options{}); err == nil {
		t.Fatal("expected missing root")
	}
	if err := Demote(ctx, Options{}); err == nil {
		t.Fatal("expected demote missing root")
	}

	root := t.TempDir()
	base := t.TempDir()
	if BaseDir(root, base) != base {
		t.Fatal("override")
	}
	t.Setenv(zqkenv.LocalCIDir().Name(), "")
	if !strings.Contains(BaseDir(root, ""), paths.ProjectDataDir) {
		t.Fatal("default base")
	}
	t.Setenv(zqkenv.LocalCIDir().Name(), "/tmp/local-ci-env")
	if BaseDir(root, "") != "/tmp/local-ci-env" {
		t.Fatal("env base")
	}

	opt := Options{ArchiveKeep: 3}
	if opt.keep() != 3 {
		t.Fatal("keep flag")
	}
	opt.ArchiveKeep = 0
	t.Setenv(zqkenv.LocalCIArchiveKeep().Name(), "9")
	if opt.keep() != 9 {
		t.Fatal("keep env")
	}
	t.Setenv(zqkenv.LocalCIArchiveKeep().Name(), "0")
	if opt.keep() != defaultArchiveKeep {
		t.Fatal("keep default")
	}
	if opt.git() == nil {
		t.Fatal("default git")
	}

	dirty := Options{RepoRoot: root, Git: func(context.Context, string, ...string) ([]byte, error) {
		return []byte(" M dirty.go\n"), nil
	}}
	if _, err := Checkout(ctx, dirty); err == nil {
		t.Fatal("expected dirty error")
	}
	if _, err := Checkout(ctx, Options{RepoRoot: root, AllowDirty: true, Git: fakeGit(t), BaseDir: base, Archive: true}); err != nil {
		t.Fatal(err)
	}
	if err := Demote(ctx, Options{RepoRoot: root, BaseDir: base, Git: fakeGit(t)}); err != nil {
		t.Fatal(err)
	}

	legacy := t.TempDir()
	wd := workdirPath(legacy)
	if err := fileutil.MkdirAll(wd, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := swingWorkdir(legacy, wd, "short", fakeGit(t), ctx, root); err != nil {
		t.Fatal(err)
	}

	if firstLine([]byte("one\ntwo\n")) != "one" {
		t.Fatal("firstLine")
	}
	if firstLine([]byte("only")) != "only" {
		t.Fatal("firstLine single")
	}
	_ = pruneOldTars(t.TempDir(), 0)
	_ = dirExists(filepath.Join(t.TempDir(), "missing"))

	pinned := Options{RepoRoot: root, SHAPinned: true, Git: func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "rev-parse --verify") {
			return []byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\n"), nil
		}
		if strings.Contains(joined, "rev-parse --short") {
			return []byte("bbbbbbb\n"), nil
		}
		return []byte(" M dirty.go\n"), nil
	}, BaseDir: t.TempDir()}
	if _, err := Checkout(ctx, pinned); err != nil {
		t.Fatal(err)
	}
	dropDir := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(dropDir, "drop-aaaa.txt"), []byte("x"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if man, err := listDropManifest(dropDir); err != nil || !strings.Contains(man, "drop-aaaa.txt") {
		t.Fatalf("manifest = %q %v", man, err)
	}
	clobber := t.TempDir()
	wdFile := workdirPath(clobber)
	if err := fileutil.WriteFile(wdFile, []byte("not-a-dir"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := swingWorkdir(clobber, wdFile, "x", fakeGit(t), ctx, root); err == nil {
		t.Fatal("expected refuse clobber")
	}
	trees := treesDir(base)
	_ = fileutil.MkdirAll(filepath.Join(trees, "oldtree"), paths.DirPerm755)
	if err := pruneOtherTrees(ctx, fakeGit(t), root, trees, "aaaaaaa"); err != nil {
		t.Fatal(err)
	}
}
