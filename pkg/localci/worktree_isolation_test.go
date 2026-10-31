package localci

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqkenv"
)

func TestCheckout_ContextCancellationAndMissingRoot(t *testing.T) {
	t.Parallel()

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := Checkout(canceledCtx, Options{RepoRoot: t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if _, err := Checkout(context.Background(), Options{RepoRoot: "   "}); err == nil || !strings.Contains(err.Error(), "project root not found") {
		t.Fatalf("expected project root not found error, got %v", err)
	}
}

func TestDemote_ContextCancellationAndMissingRoot(t *testing.T) {
	t.Parallel()

	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := Demote(canceledCtx, Options{RepoRoot: t.TempDir()}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	if err := Demote(context.Background(), Options{RepoRoot: "   "}); err == nil || !strings.Contains(err.Error(), "project root not found") {
		t.Fatalf("expected project root not found error, got %v", err)
	}
}

func TestCheckout_GitFailures(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	base := t.TempDir()

	errVerify := errors.New("rev-parse verify failed")
	gitVerifyFail := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "rev-parse --verify") {
			return nil, errVerify
		}
		return nil, nil
	}
	if _, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: gitVerifyFail}); !errors.Is(err, errVerify) {
		t.Fatalf("expected errVerify, got %v", err)
	}

	errShort := errors.New("rev-parse short failed")
	gitShortFail := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "rev-parse --verify") {
			return []byte("1111111111111111111111111111111111111111\n"), nil
		}
		if strings.Contains(joined, "rev-parse --short") {
			return nil, errShort
		}
		return nil, nil
	}
	if _, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: gitShortFail}); !errors.Is(err, errShort) {
		t.Fatalf("expected errShort, got %v", err)
	}

	errStatus := errors.New("status porcelain failed")
	gitStatusFail := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "rev-parse --verify") {
			return []byte("1111111111111111111111111111111111111111\n"), nil
		}
		if strings.Contains(joined, "rev-parse --short") {
			return []byte("1111111\n"), nil
		}
		if strings.Contains(joined, "status --porcelain") {
			return nil, errStatus
		}
		return nil, nil
	}
	if _, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: gitStatusFail}); !errors.Is(err, errStatus) {
		t.Fatalf("expected errStatus, got %v", err)
	}

	errWorktreeAdd := errors.New("worktree add failed")
	gitWorktreeAddFail := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "rev-parse --verify") {
			return []byte("1111111111111111111111111111111111111111\n"), nil
		}
		if strings.Contains(joined, "rev-parse --short") {
			return []byte("1111111\n"), nil
		}
		if strings.Contains(joined, "status --porcelain") {
			return []byte(""), nil
		}
		if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
			return nil, errWorktreeAdd
		}
		return nil, nil
	}
	if _, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: gitWorktreeAddFail}); !errors.Is(err, errWorktreeAdd) {
		t.Fatalf("expected errWorktreeAdd, got %v", err)
	}
}

func TestCheckout_SkipWhenCurrentAndDirExists(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	base := t.TempDir()
	wd := workdirPath(base)
	if err := fileutil.MkdirAll(wd, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	targetSHA := "2222222222222222222222222222222222222222"
	targetShort := "2222222"
	if err := fileutil.WriteFile(sourceSHAPath(base), []byte(targetSHA+"\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	mockGit := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "rev-parse --verify") {
			return []byte(targetSHA + "\n"), nil
		}
		if strings.Contains(joined, "rev-parse --short") {
			return []byte(targetShort + "\n"), nil
		}
		if strings.Contains(joined, "status --porcelain") {
			return []byte(""), nil
		}
		return nil, nil
	}

	res, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: mockGit})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Skipped {
		t.Fatal("expected checkout to be skipped")
	}
	if res.SHA != targetSHA || res.Short != targetShort {
		t.Fatalf("unexpected result: %+v", res)
	}
	if res.Workdir != wd {
		t.Fatalf("expected workdir %s, got %s", wd, res.Workdir)
	}
}

func TestCheckout_DirectoryAndFileCreationFailures(t *testing.T) {
	t.Parallel()
	mockGit := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "rev-parse --verify") {
			return []byte("3333333333333333333333333333333333333333\n"), nil
		}
		if strings.Contains(joined, "rev-parse --short") {
			return []byte("3333333\n"), nil
		}
		if strings.Contains(joined, "status --porcelain") {
			return []byte(""), nil
		}
		return nil, nil
	}

	// 1. MkdirAll trees failure: trees path is an existing regular file
	{
		root := t.TempDir()
		base := t.TempDir()
		if err := fileutil.WriteFile(treesDir(base), []byte("blocker"), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
		_, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: mockGit})
		if err == nil || !strings.Contains(err.Error(), "local-ci trees") {
			t.Fatalf("expected local-ci trees error, got %v", err)
		}
	}

	// 2. MkdirAll archives failure: archives path is an existing regular file
	{
		root := t.TempDir()
		base := t.TempDir()
		if err := fileutil.WriteFile(filepath.Join(base, paths.LocalCIArchivesDir), []byte("blocker"), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
		_, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: mockGit})
		if err == nil || !strings.Contains(err.Error(), "local-ci archives") {
			t.Fatalf("expected local-ci archives error, got %v", err)
		}
	}

	// 3. WriteFile SOURCE_SHA failure: target path is a directory
	{
		root := t.TempDir()
		base := t.TempDir()
		if err := fileutil.MkdirAll(sourceSHAPath(base), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		_, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: mockGit})
		if err == nil || !strings.Contains(err.Error(), "write SOURCE_SHA") {
			t.Fatalf("expected write SOURCE_SHA error, got %v", err)
		}
	}

	// 4. WriteFile PROMOTED_AT failure: target path is a directory
	{
		root := t.TempDir()
		base := t.TempDir()
		if err := fileutil.MkdirAll(filepath.Join(base, paths.LocalCIPromotedAtFile), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		_, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: mockGit})
		if err == nil || !strings.Contains(err.Error(), "write PROMOTED_AT") {
			t.Fatalf("expected write PROMOTED_AT error, got %v", err)
		}
	}

	// 5. WriteFile CURRENT.env failure: target path is a directory
	{
		root := t.TempDir()
		base := t.TempDir()
		if err := fileutil.MkdirAll(filepath.Join(base, paths.LocalCICurrentEnvFile), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		_, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: mockGit})
		if err == nil || !strings.Contains(err.Error(), "write CURRENT.env") {
			t.Fatalf("expected write CURRENT.env error, got %v", err)
		}
	}

	// 6. swingWorkdir failure during checkout: workdir is a non-clobberable regular file
	{
		root := t.TempDir()
		base := t.TempDir()
		if err := fileutil.WriteFile(workdirPath(base), []byte("regular-file"), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
		_, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: mockGit})
		if err == nil || !strings.Contains(err.Error(), "refusing to clobber unexpected workdir") {
			t.Fatalf("expected refusing to clobber error, got %v", err)
		}
	}

	// 7. pruneOtherTrees failure during checkout: trees path becomes non-directory
	{
		root := t.TempDir()
		base := t.TempDir()
		failPruneGit := func(_ context.Context, _ string, args ...string) ([]byte, error) {
			joined := strings.Join(args, " ")
			if strings.Contains(joined, "rev-parse --verify") {
				return []byte("4444444444444444444444444444444444444444\n"), nil
			}
			if strings.Contains(joined, "rev-parse --short") {
				return []byte("4444444\n"), nil
			}
			if strings.Contains(joined, "status --porcelain") {
				return []byte(""), nil
			}
			if len(args) >= 2 && args[0] == "worktree" && args[1] == "add" {
				// Destroy trees directory so pruneOtherTrees fails with ENOTDIR on ReadDir
				_ = fileutil.RemoveAll(treesDir(base))
				_ = fileutil.WriteFile(treesDir(base), []byte("not-a-dir"), paths.FilePerm644)
				return nil, nil
			}
			return nil, nil
		}
		_, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Git: failPruneGit})
		if err == nil {
			t.Fatal("expected pruneOtherTrees error during checkout")
		}
	}

	// 8. Archive failure during checkout: git archive fails
	{
		root := t.TempDir()
		base := t.TempDir()
		failArchGit := func(_ context.Context, _ string, args ...string) ([]byte, error) {
			joined := strings.Join(args, " ")
			if strings.Contains(joined, "rev-parse --verify") {
				return []byte("3333333333333333333333333333333333333333\n"), nil
			}
			if strings.Contains(joined, "rev-parse --short") {
				return []byte("3333333\n"), nil
			}
			if strings.Contains(joined, "status --porcelain") {
				return []byte(""), nil
			}
			if len(args) > 0 && args[0] == "archive" {
				return nil, errors.New("archive fail")
			}
			return nil, nil
		}
		_, err := Checkout(context.Background(), Options{RepoRoot: root, BaseDir: base, Archive: true, Git: failArchGit})
		if err == nil || !strings.Contains(err.Error(), "archive fail") {
			t.Fatalf("expected archive fail error, got %v", err)
		}
	}
}

func TestSwingWorkdir_AllTransitions(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	base := t.TempDir()
	wd := workdirPath(base)

	fakeGit := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, nil
	}

	// Transition 1: workdir does not exist -> creates symlink
	if err := swingWorkdir(base, wd, "treeA", fakeGit, ctx, root); err != nil {
		t.Fatalf("initial symlink: %v", err)
	}
	target, err := fileutil.Readlink(wd)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if want := filepath.Join(paths.LocalCITreesDir, "treeA"); target != want {
		t.Fatalf("target=%s want %s", target, want)
	}

	// Transition 2: workdir is already a symlink -> replaces symlink
	if err := swingWorkdir(base, wd, "treeB", fakeGit, ctx, root); err != nil {
		t.Fatalf("update symlink: %v", err)
	}
	target, err = fileutil.Readlink(wd)
	if err != nil {
		t.Fatalf("readlink 2: %v", err)
	}
	if want := filepath.Join(paths.LocalCITreesDir, "treeB"); target != want {
		t.Fatalf("target=%s want %s", target, want)
	}

	// Transition 3: workdir is an existing directory (legacy) -> migrates and symlinks
	if err := fileutil.Remove(wd); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(wd, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(wd, "data.txt"), []byte("legacy"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	if err := swingWorkdir(base, wd, "treeC", fakeGit, ctx, root); err != nil {
		t.Fatalf("migrate legacy dir: %v", err)
	}
	target, err = fileutil.Readlink(wd)
	if err != nil {
		t.Fatalf("readlink 3: %v", err)
	}
	if want := filepath.Join(paths.LocalCITreesDir, "treeC"); target != want {
		t.Fatalf("target=%s want %s", target, want)
	}

	// Transition 4: workdir is an unexpected regular file -> refuses to clobber
	if err := fileutil.Remove(wd); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(wd, []byte("file"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := swingWorkdir(base, wd, "treeD", fakeGit, ctx, root); err == nil || !strings.Contains(err.Error(), "refusing to clobber unexpected workdir") {
		t.Fatalf("expected refusal error, got %v", err)
	}

	// Transition 5: Lstat error other than NotExist (ENOTDIR on invalid path structure)
	invalidBaseFile := filepath.Join(base, "file-not-dir")
	if err := fileutil.WriteFile(invalidBaseFile, []byte("x"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	invalidWd := filepath.Join(invalidBaseFile, "sub", "workdir")
	if err := swingWorkdir(base, invalidWd, "treeE", fakeGit, ctx, root); err == nil {
		t.Fatal("expected error on invalid workdir path")
	}

	// Transition 6: directory rename failure when legacy dir already exists and is non-empty
	renameFailBase := t.TempDir()
	renameFailWd := workdirPath(renameFailBase)
	if err := fileutil.MkdirAll(renameFailWd, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	legacyDir := filepath.Join(renameFailBase, fmt.Sprintf("workdir.legacy.%d", os.Getpid()))
	if err := fileutil.MkdirAll(legacyDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(legacyDir, "blocker.txt"), []byte("non-empty"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := swingWorkdir(renameFailBase, renameFailWd, "treeF", fakeGit, ctx, root); err == nil || !strings.Contains(err.Error(), "migrate legacy workdir") {
		t.Fatalf("expected migrate legacy workdir error, got %v", err)
	}
}

func TestPruneOtherTrees_Boundaries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()
	base := t.TempDir()

	fakeGit := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, nil
	}

	// 1. trees directory does not exist -> returns nil
	missingTrees := filepath.Join(base, "nonexistent-trees")
	if err := pruneOtherTrees(ctx, fakeGit, root, missingTrees, "any"); err != nil {
		t.Fatalf("expected nil for missing trees dir, got %v", err)
	}

	// 2. trees is an existing regular file -> returns error
	treesFile := filepath.Join(base, "trees-file")
	if err := fileutil.WriteFile(treesFile, []byte("x"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := pruneOtherTrees(ctx, fakeGit, root, treesFile, "any"); err == nil {
		t.Fatal("expected error when trees is a regular file")
	}

	// 3. trees contains keepShort and older directories
	treesDir := filepath.Join(base, "trees-real")
	keepDir := filepath.Join(treesDir, "keep777")
	oldDir1 := filepath.Join(treesDir, "old111")
	oldDir2 := filepath.Join(treesDir, "old222")
	for _, d := range []string{keepDir, oldDir1, oldDir2} {
		if err := fileutil.MkdirAll(d, paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
	}

	if err := pruneOtherTrees(ctx, fakeGit, root, treesDir, "keep777"); err != nil {
		t.Fatalf("unexpected prune error: %v", err)
	}
	if !dirExists(keepDir) {
		t.Fatal("keepDir was pruned unexpectedly")
	}
	if dirExists(oldDir1) || dirExists(oldDir2) {
		t.Fatal("old trees were not pruned")
	}
}

func TestWriteArchive_FullLifecycleAndErrorBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// 1. Successful first run (initializes archive git) and second run (reuses existing archive git)
	{
		root := t.TempDir()
		base := t.TempDir()
		mockGit := func(_ context.Context, dir string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "archive" {
				for i, a := range args {
					if a == "-o" && i+1 < len(args) {
						_ = fileutil.MkdirAll(filepath.Dir(args[i+1]), paths.DirPerm755)
						_ = fileutil.WriteFile(args[i+1], []byte("tar-payload"), paths.FilePerm644)
					}
				}
				return nil, nil
			}
			if len(args) > 0 && args[0] == "init" {
				_ = fileutil.MkdirAll(filepath.Join(dir, ".git"), paths.DirPerm755)
				return nil, nil
			}
			return nil, nil
		}

		// First run: initializes archive git
		if err := writeArchive(ctx, mockGit, root, base, "sha111", "1111111", "2026-10-31T00:00:00Z", 5); err != nil {
			t.Fatalf("first writeArchive: %v", err)
		}
		archiveGit := filepath.Join(base, paths.LocalCIArchiveGitDir)
		manifestBytes, err := fileutil.ReadFile(filepath.Join(archiveGit, "MANIFEST"))
		if err != nil {
			t.Fatalf("reading MANIFEST: %v", err)
		}
		if !strings.Contains(string(manifestBytes), "drop-1111111.txt") {
			t.Fatalf("MANIFEST missing drop-1111111.txt: %s", manifestBytes)
		}

		// Second run: archiveGit/.git already exists
		if err := writeArchive(ctx, mockGit, root, base, "sha222", "2222222", "2026-10-31T01:00:00Z", 5); err != nil {
			t.Fatalf("second writeArchive: %v", err)
		}
		manifestBytes, err = fileutil.ReadFile(filepath.Join(archiveGit, "MANIFEST"))
		if err != nil {
			t.Fatalf("reading updated MANIFEST: %v", err)
		}
		if !strings.Contains(string(manifestBytes), "drop-2222222.txt") {
			t.Fatalf("MANIFEST missing drop-2222222.txt: %s", manifestBytes)
		}
	}

	// 2. Git archive failure
	{
		root := t.TempDir()
		base := t.TempDir()
		errArch := errors.New("git archive fail")
		failGit := func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "archive" {
				return nil, errArch
			}
			return nil, nil
		}
		if err := writeArchive(ctx, failGit, root, base, "sha", "short", "time", 5); !errors.Is(err, errArch) {
			t.Fatalf("expected errArch, got %v", err)
		}
	}

	// 3. sha256File failure (archive file was not produced)
	{
		root := t.TempDir()
		base := t.TempDir()
		noWriteGit := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, nil
		}
		if err := writeArchive(ctx, noWriteGit, root, base, "sha", "short", "time", 5); err == nil {
			t.Fatal("expected sha256File error when tar is missing")
		}
	}

	// 4. MkdirAll archiveGit failure
	{
		root := t.TempDir()
		base := t.TempDir()
		// Make archiveGit path a regular file
		archiveGitPath := filepath.Join(base, paths.LocalCIArchiveGitDir)
		if err := fileutil.WriteFile(archiveGitPath, []byte("blocker"), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
		mockGit := func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "archive" {
				for i, a := range args {
					if a == "-o" && i+1 < len(args) {
						_ = fileutil.MkdirAll(filepath.Dir(args[i+1]), paths.DirPerm755)
						_ = fileutil.WriteFile(args[i+1], []byte("tar-payload"), paths.FilePerm644)
					}
				}
			}
			return nil, nil
		}
		if err := writeArchive(ctx, mockGit, root, base, "sha", "short", "time", 5); err == nil || !strings.Contains(err.Error(), "archive-git dir") {
			t.Fatalf("expected archive-git dir error, got %v", err)
		}
	}

	// 5. git init failure
	{
		root := t.TempDir()
		base := t.TempDir()
		errInit := errors.New("git init fail")
		mockGit := func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "archive" {
				for i, a := range args {
					if a == "-o" && i+1 < len(args) {
						_ = fileutil.MkdirAll(filepath.Dir(args[i+1]), paths.DirPerm755)
						_ = fileutil.WriteFile(args[i+1], []byte("tar-payload"), paths.FilePerm644)
					}
				}
				return nil, nil
			}
			if len(args) > 0 && args[0] == "init" {
				return nil, errInit
			}
			return nil, nil
		}
		if err := writeArchive(ctx, mockGit, root, base, "sha", "short", "time", 5); !errors.Is(err, errInit) {
			t.Fatalf("expected errInit, got %v", err)
		}
	}

	// 6. WriteFile drop-short.txt failure: target path is a directory
	{
		root := t.TempDir()
		base := t.TempDir()
		archiveGit := filepath.Join(base, paths.LocalCIArchiveGitDir)
		if err := fileutil.MkdirAll(filepath.Join(archiveGit, "drop-short.txt"), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		if err := fileutil.MkdirAll(filepath.Join(archiveGit, ".git"), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		mockGit := func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "archive" {
				for i, a := range args {
					if a == "-o" && i+1 < len(args) {
						_ = fileutil.MkdirAll(filepath.Dir(args[i+1]), paths.DirPerm755)
						_ = fileutil.WriteFile(args[i+1], []byte("tar-payload"), paths.FilePerm644)
					}
				}
			}
			return nil, nil
		}
		if err := writeArchive(ctx, mockGit, root, base, "sha", "short", "time", 5); err == nil {
			t.Fatal("expected write drop error")
		}
	}

	// 7. WriteFile MANIFEST failure: target path is a directory
	{
		root := t.TempDir()
		base := t.TempDir()
		archiveGit := filepath.Join(base, paths.LocalCIArchiveGitDir)
		if err := fileutil.MkdirAll(filepath.Join(archiveGit, "MANIFEST"), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		if err := fileutil.MkdirAll(filepath.Join(archiveGit, ".git"), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		mockGit := func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "archive" {
				for i, a := range args {
					if a == "-o" && i+1 < len(args) {
						_ = fileutil.MkdirAll(filepath.Dir(args[i+1]), paths.DirPerm755)
						_ = fileutil.WriteFile(args[i+1], []byte("tar-payload"), paths.FilePerm644)
					}
				}
			}
			return nil, nil
		}
		if err := writeArchive(ctx, mockGit, root, base, "sha", "short", "time", 5); err == nil {
			t.Fatal("expected write MANIFEST error")
		}
	}

	// 8. git add failure
	{
		root := t.TempDir()
		base := t.TempDir()
		archiveGit := filepath.Join(base, paths.LocalCIArchiveGitDir)
		if err := fileutil.MkdirAll(filepath.Join(archiveGit, ".git"), paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		errAdd := errors.New("git add fail")
		mockGit := func(_ context.Context, _ string, args ...string) ([]byte, error) {
			if len(args) > 0 && args[0] == "archive" {
				for i, a := range args {
					if a == "-o" && i+1 < len(args) {
						_ = fileutil.MkdirAll(filepath.Dir(args[i+1]), paths.DirPerm755)
						_ = fileutil.WriteFile(args[i+1], []byte("tar-payload"), paths.FilePerm644)
					}
				}
				return nil, nil
			}
			if len(args) > 0 && args[0] == "add" {
				return nil, errAdd
			}
			return nil, nil
		}
		if err := writeArchive(ctx, mockGit, root, base, "sha", "short", "time", 5); !errors.Is(err, errAdd) {
			t.Fatalf("expected errAdd, got %v", err)
		}
	}
}

func TestPruneOldTars_Comprehensive(t *testing.T) {
	t.Parallel()
	archives := t.TempDir()

	// Write 6 tar files with small pauses to ensure modtime ordering
	for i := 1; i <= 6; i++ {
		name := fmt.Sprintf("drop-%02d.tar", i)
		if err := fileutil.WriteFile(filepath.Join(archives, name), []byte(name), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Also write non-drop and non-tar files that should never be pruned
	nonMatching := []string{"other.txt", "drop-invalid.zip", "drop-.tar.bak"}
	for _, n := range nonMatching {
		if err := fileutil.WriteFile(filepath.Join(archives, n), []byte("keep"), paths.FilePerm644); err != nil {
			t.Fatal(err)
		}
	}

	// Prune keeping 2
	if err := pruneOldTars(archives, 2); err != nil {
		t.Fatalf("pruneOldTars: %v", err)
	}

	entries, err := fileutil.ReadDir(archives)
	if err != nil {
		t.Fatal(err)
	}
	var remainingTars []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "drop-") && strings.HasSuffix(e.Name(), ".tar") {
			remainingTars = append(remainingTars, e.Name())
		}
	}
	if len(remainingTars) != 2 {
		t.Fatalf("expected 2 remaining tars, got %v", remainingTars)
	}
	for _, n := range nonMatching {
		if _, err := fileutil.Stat(filepath.Join(archives, n)); err != nil {
			t.Fatalf("non-matching file was pruned: %s", n)
		}
	}

	// Keep <= 0 resets to defaultArchiveKeep
	if err := pruneOldTars(archives, -1); err != nil {
		t.Fatalf("pruneOldTars <= 0: %v", err)
	}

	// Non-existent directory returns nil safely
	if err := pruneOldTars("/nonexistent/directory/path", 5); err != nil {
		t.Fatalf("pruneOldTars nonexistent dir: %v", err)
	}
}

func TestListDropManifest_Comprehensive(t *testing.T) {
	t.Parallel()

	// 1. Nonexistent directory returns error
	if _, err := listDropManifest("/nonexistent/archive/dir"); err == nil {
		t.Fatal("expected error for nonexistent directory")
	}

	// 2. Directory with no matching files returns empty string
	dir := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(dir, "README.md"), []byte("hi"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if res, err := listDropManifest(dir); err != nil || res != "" {
		t.Fatalf("expected empty manifest, got %q, %v", res, err)
	}

	// 3. Directory with multiple drop text files returns sorted lines
	if err := fileutil.WriteFile(filepath.Join(dir, "drop-beta.txt"), []byte("beta"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(dir, "drop-alpha.txt"), []byte("alpha"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	manifest, err := listDropManifest(dir)
	if err != nil {
		t.Fatalf("listDropManifest: %v", err)
	}
	want := "drop-alpha.txt\ndrop-beta.txt\n"
	if manifest != want {
		t.Fatalf("manifest = %q, want %q", manifest, want)
	}
}

func TestSha256File_Comprehensive(t *testing.T) {
	t.Parallel()

	// 1. Nonexistent file returns error
	if _, err := sha256File("/nonexistent/file.tar"); err == nil {
		t.Fatal("expected error on nonexistent file")
	}

	// 2. Valid file computes correct sha256
	p := filepath.Join(t.TempDir(), "sample.txt")
	content := []byte("antigravity-localci-test-content")
	if err := fileutil.WriteFile(p, content, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	expectedHash := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(expectedHash[:])

	gotHex, err := sha256File(p)
	if err != nil {
		t.Fatalf("sha256File: %v", err)
	}
	if gotHex != expectedHex {
		t.Fatalf("got hex %s, want %s", gotHex, expectedHex)
	}
}

func TestDemote_ComprehensiveBranches(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	root := t.TempDir()

	fakeGit := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, nil
	}

	// Case 1: workdir is a symlink pointing to relative tree target that exists
	{
		base := t.TempDir()
		trees := treesDir(base)
		shortTree := filepath.Join(trees, "tree1")
		if err := fileutil.MkdirAll(shortTree, paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		wd := workdirPath(base)
		rel := filepath.Join(paths.LocalCITreesDir, "tree1")
		if err := fileutil.Symlink(rel, wd); err != nil {
			t.Fatal(err)
		}
		// Create pointer files
		for _, name := range []string{paths.LocalCISourceSHAFile, paths.LocalCIPromotedAtFile, paths.LocalCICurrentEnvFile} {
			if err := fileutil.WriteFile(filepath.Join(base, name), []byte("data"), paths.FilePerm644); err != nil {
				t.Fatal(err)
			}
		}

		if err := Demote(ctx, Options{RepoRoot: root, BaseDir: base, Git: fakeGit}); err != nil {
			t.Fatalf("Demote case 1: %v", err)
		}
		if dirExists(shortTree) {
			t.Fatal("shortTree should be removed")
		}
		if _, err := fileutil.Lstat(wd); !fileutil.IsNotExist(err) {
			t.Fatal("wd symlink should be removed")
		}
		for _, name := range []string{paths.LocalCISourceSHAFile, paths.LocalCIPromotedAtFile, paths.LocalCICurrentEnvFile} {
			if _, err := fileutil.Stat(filepath.Join(base, name)); !fileutil.IsNotExist(err) {
				t.Fatalf("%s should be removed", name)
			}
		}
	}

	// Case 2: workdir is a symlink pointing to absolute target
	{
		base := t.TempDir()
		absTree := filepath.Join(t.TempDir(), "external-tree")
		if err := fileutil.MkdirAll(absTree, paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		wd := workdirPath(base)
		if err := fileutil.Symlink(absTree, wd); err != nil {
			t.Fatal(err)
		}

		if err := Demote(ctx, Options{RepoRoot: root, BaseDir: base, Git: fakeGit}); err != nil {
			t.Fatalf("Demote case 2: %v", err)
		}
		if dirExists(absTree) {
			t.Fatal("absTree should be removed")
		}
	}

	// Case 3: workdir is a symlink pointing to a nonexistent target
	{
		base := t.TempDir()
		wd := workdirPath(base)
		if err := fileutil.Symlink(filepath.Join(base, "nonexistent-target"), wd); err != nil {
			t.Fatal(err)
		}
		if err := Demote(ctx, Options{RepoRoot: root, BaseDir: base, Git: fakeGit}); err != nil {
			t.Fatalf("Demote case 3: %v", err)
		}
		if _, err := fileutil.Lstat(wd); !fileutil.IsNotExist(err) {
			t.Fatal("wd symlink should be removed")
		}
	}

	// Case 4: workdir is a directory (legacy)
	{
		base := t.TempDir()
		wd := workdirPath(base)
		if err := fileutil.MkdirAll(wd, paths.DirPerm755); err != nil {
			t.Fatal(err)
		}
		if err := Demote(ctx, Options{RepoRoot: root, BaseDir: base, Git: fakeGit}); err != nil {
			t.Fatalf("Demote case 4: %v", err)
		}
		if dirExists(wd) {
			t.Fatal("wd directory should be removed")
		}
	}

	// Case 5: treesDir contains multiple remaining worktrees
	{
		base := t.TempDir()
		trees := treesDir(base)
		for _, name := range []string{"treeA", "treeB", "treeC"} {
			if err := fileutil.MkdirAll(filepath.Join(trees, name), paths.DirPerm755); err != nil {
				t.Fatal(err)
			}
		}
		if err := Demote(ctx, Options{RepoRoot: root, BaseDir: base, Git: fakeGit}); err != nil {
			t.Fatalf("Demote case 5: %v", err)
		}
		entries, _ := fileutil.ReadDir(trees)
		if len(entries) != 0 {
			t.Fatalf("expected 0 entries in treesDir, found %d", len(entries))
		}
	}
}

func TestRunGit_And_GitTrim_Failures(t *testing.T) {
	t.Parallel()

	// 1. runGit handles nil context and empty dir
	out, err := runGit(nil, "", "version")
	if err != nil {
		t.Fatalf("runGit nil ctx: %v", err)
	}
	if !strings.Contains(string(out), "git version") {
		t.Fatalf("unexpected git version output: %s", out)
	}

	// 2. runGit with explicit dir
	tempDir := t.TempDir()
	out, err = runGit(context.Background(), tempDir, "version")
	if err != nil {
		t.Fatalf("runGit with dir: %v", err)
	}
	if !strings.Contains(string(out), "git version") {
		t.Fatalf("unexpected git version output: %s", out)
	}

	// 3. runGit failure with empty output falling back to err.Error()
	// Using a command inside an initialized repo that exits with non-zero code and produces no output
	gitRepoDir := t.TempDir()
	if _, err := runGit(context.Background(), gitRepoDir, "init", "-q"); err != nil {
		t.Fatalf("git init: %v", err)
	}
	_, err = runGit(context.Background(), gitRepoDir, "rev-parse", "-q", "--verify", "0000000000000000000000000000000000000000^{commit}")
	if err == nil {
		t.Fatal("expected error on nonexistent commit")
	}
	if !strings.Contains(err.Error(), "exit status 1") {
		t.Fatalf("expected error message to contain exit status 1, got %v", err)
	}

	// 4. gitTrim error propagation
	errTest := errors.New("git run failed")
	mockFail := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, errTest
	}
	if _, err := gitTrim(context.Background(), mockFail, tempDir, "status"); !errors.Is(err, errTest) {
		t.Fatalf("expected errTest, got %v", err)
	}

	// 5. gitTrim whitespace trimming
	mockTrim := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("  \n  output-value \t \n"), nil
	}
	val, err := gitTrim(context.Background(), mockTrim, tempDir, "status")
	if err != nil {
		t.Fatalf("gitTrim error: %v", err)
	}
	if val != "output-value" {
		t.Fatalf("expected trimmed 'output-value', got %q", val)
	}
}

func TestLocalCIOptions_And_Helpers(t *testing.T) {
	// Not parallel because of t.Setenv
	root := t.TempDir()
	base := t.TempDir()

	// BaseDir precedence
	if got := BaseDir(root, base); got != base {
		t.Fatalf("expected override %s, got %s", base, got)
	}
	t.Setenv(zqkenv.LocalCIDir().Name(), "/custom/env/base")
	if got := BaseDir(root, ""); got != "/custom/env/base" {
		t.Fatalf("expected env %s, got %s", "/custom/env/base", got)
	}
	t.Setenv(zqkenv.LocalCIDir().Name(), "")
	expectedDefault := filepath.Join(root, paths.ProjectDataDir, paths.LocalCIDir)
	if got := BaseDir(root, ""); got != expectedDefault {
		t.Fatalf("expected default %s, got %s", expectedDefault, got)
	}

	// Options.git()
	customGit := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return nil, nil
	}
	optCustom := Options{Git: customGit}
	if optCustom.git() == nil {
		t.Fatal("optCustom.git() returned nil")
	}
	optDefault := Options{}
	if optDefault.git() == nil {
		t.Fatal("optDefault.git() returned nil")
	}

	// Options.keep()
	optKeep := Options{ArchiveKeep: 8}
	if optKeep.keep() != 8 {
		t.Fatalf("expected 8, got %d", optKeep.keep())
	}
	optEnv := Options{}
	t.Setenv(zqkenv.LocalCIArchiveKeep().Name(), "12")
	if optEnv.keep() != 12 {
		t.Fatalf("expected 12 from env, got %d", optEnv.keep())
	}
	t.Setenv(zqkenv.LocalCIArchiveKeep().Name(), "")
	if optEnv.keep() != defaultArchiveKeep {
		t.Fatalf("expected default %d, got %d", defaultArchiveKeep, optEnv.keep())
	}

	// Helper paths
	if got := workdirPath("/test/base"); got != "/test/base/workdir" {
		t.Fatalf("workdirPath = %s", got)
	}
	if got := treesDir("/test/base"); got != "/test/base/trees" {
		t.Fatalf("treesDir = %s", got)
	}
	if got := sourceSHAPath("/test/base"); got != "/test/base/SOURCE_SHA" {
		t.Fatalf("sourceSHAPath = %s", got)
	}
}

func TestHasTrackedDirty_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		porcelain string
		want      bool
	}{
		{"empty string", "", false},
		{"single newline", "\n", false},
		{"single char", "M", false},
		{"untracked only", "?? file.go\n", false},
		{"ignored only", "!! ignored.so\n", false},
		{"multiple untracked and ignored", "?? foo.txt\r\n!! bar.txt\r\n", false},
		{"modified in worktree", " M file.go\n", true},
		{"staged modification", "M  file.go\n", true},
		{"staged and unstaged", "MM file.go\n", true},
		{"added to index", "A  file.go\n", true},
		{"deleted from index", "D  file.go\n", true},
		{"renamed", "R  file.go\n", true},
		{"copied", "C  file.go\n", true},
		{"untracked followed by dirty", "?? new.go\n M dirty.go\n", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasTrackedDirty(tt.porcelain); got != tt.want {
				t.Fatalf("hasTrackedDirty(%q) = %v, want %v", tt.porcelain, got, tt.want)
			}
		})
	}
}

func TestFirstLine_Table(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []byte
		want string
	}{
		{"nil slice", nil, ""},
		{"empty slice", []byte(""), ""},
		{"only whitespace", []byte("   \n\t  "), ""},
		{"single line without newline", []byte("hello"), "hello"},
		{"single line with newline", []byte("hello\n"), "hello"},
		{"multiple lines", []byte("first\nsecond\nthird"), "first"},
		{"leading whitespace on first line", []byte("  spaced first  \nsecond"), "spaced first"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstLine(tt.in); got != tt.want {
				t.Fatalf("firstLine(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestWorktreeListContains_Table(t *testing.T) {
	t.Parallel()

	porcelain := "worktree /path/to/alpha\nHEAD 12345\n\nworktree /path/to/beta\nHEAD 67890\n"

	tests := []struct {
		name string
		path string
		want bool
	}{
		{"first worktree", "/path/to/alpha", true},
		{"second worktree", "/path/to/beta", true},
		{"non-existent worktree", "/path/to/gamma", false},
		{"partial path match", "/path/to", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := worktreeListContains(porcelain, tt.path); got != tt.want {
				t.Fatalf("worktreeListContains(%s) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}
