package objectidcache

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestRelSlashInsideDir(t *testing.T) {
	t.Parallel()
	base := filepath.Join(string(filepath.Separator), paths.ProcessDir, "backlog")
	inside := filepath.Join(base, "12c26f8e.yaml")
	nested := filepath.Join(base, "2026-08", "item.yaml")
	outside := filepath.Join(filepath.Dir(base), "goals", "g.yaml")

	rel, ok := relSlashInsideDir(base, inside)
	if !ok || rel != "12c26f8e.yaml" {
		t.Fatalf("inside: got (%q, %v) want (12c26f8e.yaml, true)", rel, ok)
	}
	rel, ok = relSlashInsideDir(base, nested)
	if !ok || rel != "2026-08/item.yaml" {
		t.Fatalf("nested: got (%q, %v) want (2026-08/item.yaml, true)", rel, ok)
	}
	if _, ok = relSlashInsideDir(base, outside); ok {
		t.Fatal("sibling kind dir must not count as inside")
	}
	if _, ok = relSlashInsideDir(emptyValue, inside); ok {
		t.Fatal("empty base is not inside")
	}
	if _, ok = relSlashInsideDir(base, emptyValue); ok {
		t.Fatal("empty path is not inside")
	}
}

func TestRelWalksOutOfDir(t *testing.T) {
	t.Parallel()
	if !relWalksOutOfDir("..") || !relWalksOutOfDir("../goals/x.yaml") {
		t.Fatal("parent Rel results must walk out")
	}
	if relWalksOutOfDir(".") || relWalksOutOfDir("hash.yaml") || relWalksOutOfDir("2026-08/hash.yaml") {
		t.Fatal("in-dir Rel results must not walk out")
	}
}

func TestKindBucketPathToStore_RelativeWhenInsideKindDir(t *testing.T) {
	t.Parallel()
	kindDirName := objects.GetDirectoryFromKind("backlog_item")
	if kindDirName == emptyValue {
		t.Skip("kind mapper has no directory for backlog_item")
	}
	processDir := t.TempDir()
	kindDir := filepath.Join(processDir, kindDirName)
	filePath := filepath.Join(kindDir, "12c26f8efced54f6d819903d522913a4b08ecb3606edf4512621217d5f070d55.yaml")
	got := kindBucketPathToStore(processDir, "backlog_item", filePath)
	want := "12c26f8efced54f6d819903d522913a4b08ecb3606edf4512621217d5f070d55.yaml"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestKindBucketPathToStore_DraftPlaneStaysAbsolute(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	processDir := filepath.Join(projectRoot, paths.ProcessDir)
	draftPath := storage.ObjectDraftPlanePath(projectRoot, "goal", "GOAL-rel-path-1")
	got := kindBucketPathToStore(processDir, "goal", draftPath)
	if got != draftPath {
		t.Fatalf("got %q want absolute draft path %q", got, draftPath)
	}
}

func TestProjectRootFromProcessDir(t *testing.T) {
	t.Parallel()
	projectRoot := "/Users/test/workspace/repo"

	// Canonical .zqk/process path
	zqkProcess := filepath.Join(projectRoot, paths.ProjectDataDir, "process")
	if got := projectRootFromProcessDir(zqkProcess); got != projectRoot {
		t.Fatalf("zqkProcess: got %q, want %q", got, projectRoot)
	}

	// Legacy docs/process path
	docsProcess := filepath.Join(projectRoot, "docs", "process")
	if got := projectRootFromProcessDir(docsProcess); got != projectRoot {
		t.Fatalf("docsProcess: got %q, want %q", got, projectRoot)
	}

	// Empty and invalid paths
	if got := projectRootFromProcessDir(""); got != "" {
		t.Fatalf("empty: got %q, want empty", got)
	}
	if got := projectRootFromProcessDir("/tmp/other/dir"); got != "" {
		t.Fatalf("other dir: got %q, want empty", got)
	}
	if got := projectRootFromProcessDir("/tmp/other/process"); got != "" {
		t.Fatalf("other/process: got %q, want empty", got)
	}
}

func TestValidateAndCleanStale_DraftPlaneNotHealedToCAS(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	processDir := filepath.Join(projectRoot, paths.ProcessDir)

	cache := NewObjectIDCache()
	cache.processDir = processDir

	// Add a draft-plane entry for a deleted/non-existent draft file
	draftPath := storage.ObjectDraftPlanePath(projectRoot, "goal", "GOAL-draft-missing")
	cache.Set("GOAL-draft-missing", &ObjectIDCacheEntry{
		ID:       "GOAL-draft-missing",
		Kind:     "goal",
		FilePath: draftPath,
		MTime:    time.Now(),
		Exists:   true,
	})

	// ValidateAndCleanStale should remove the draft entry as stale, without attempting to heal it into CAS
	staleCount := cache.ValidateAndCleanStale()
	if staleCount != 1 {
		t.Fatalf("expected 1 stale entry, got %d", staleCount)
	}

	if _, ok := cache.Get("GOAL-draft-missing"); ok {
		t.Fatal("expected GOAL-draft-missing to be removed from cache")
	}
}
