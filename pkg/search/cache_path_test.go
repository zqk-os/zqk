package search

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestCachePath_ZeroPollutionStandaloneXDG(t *testing.T) {
	xdgRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdgRoot)

	repo := t.TempDir()
	if err := fileutil.WriteFile(filepath.Join(repo, "main.go"), []byte("package main\nfunc StandAloneEntry() {}\n"), paths.FilePerm644); err != nil {
		t.Fatalf("write sample source: %v", err)
	}

	engine := NewEngine(repo)
	cachePath := engine.CachePath()
	if !strings.HasPrefix(cachePath, xdgRoot) {
		t.Fatalf("expected standalone cache path under XDG_CACHE_HOME (%s), got %s", xdgRoot, cachePath)
	}

	if err := engine.BuildTrigramIndex(SearchOptions{UseIndex: true}); err != nil {
		t.Fatalf("standalone BuildTrigramIndex failed: %v", err)
	}
	res, err := engine.Search(context.Background(), SearchOptions{
		Query:    "StandAloneEntry",
		UseIndex: true,
	})
	if err != nil {
		t.Fatalf("standalone indexed search failed: %v", err)
	}
	if len(res.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(res.Matches))
	}

	// Zero-pollution guarantee: .zqk/ MUST NOT be created inside the uninitialized repo.
	if fileutil.Exists(filepath.Join(repo, paths.ProjectDataDir)) {
		t.Fatalf("expected zero-pollution standalone zgrep NOT to create %s inside repo", paths.ProjectDataDir)
	}
	if !fileutil.Exists(cachePath) {
		t.Fatalf("expected trigram index cache to be persisted at %s", cachePath)
	}
}

func TestCachePath_InitializedWorkspaceUsesProjectCache(t *testing.T) {
	xdgRoot := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", xdgRoot)

	repo := t.TempDir()
	if err := fileutil.MkdirAll(filepath.Join(repo, paths.ProjectDataDir), paths.DirPerm755); err != nil {
		t.Fatalf("mkdir .zqk: %v", err)
	}

	engine := NewEngine(repo)
	want := filepath.Join(repo, paths.ProjectDataDir, paths.CacheDir, "trigram.idx")
	if got := engine.CachePath(); got != want {
		t.Fatalf("expected initialized workspace cache path %s, got %s", want, got)
	}
}
