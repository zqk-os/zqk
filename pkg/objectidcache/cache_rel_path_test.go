package objectidcache

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

func TestRelSlashInsideDir(t *testing.T) {
	t.Parallel()
	base := filepath.Join(string(filepath.Separator), "docs", "process", "backlog")
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
	processDir := filepath.Join(projectRoot, "docs", "process")
	draftPath := storage.ObjectDraftPlanePath(projectRoot, "goal", "GOAL-rel-path-1")
	got := kindBucketPathToStore(processDir, "goal", draftPath)
	if got != draftPath {
		t.Fatalf("got %q want absolute draft path %q", got, draftPath)
	}
}
