package system

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage"
)

// TRACK: BLI-1785895580100186000-c5539372
func TestValidateReferenceWithCache_PendingSoftExclude(t *testing.T) {
	root := t.TempDir()
	storage.ResetObjectIDCachePendingForTest()
	t.Cleanup(storage.ResetObjectIDCachePendingForTest)

	storage.NoteObjectIDCachePending(root, string(storage.ObjectIDCachePendingOpUpdate), "BLI-pending-1", "backlog_item",
		filepath.Join(root, paths.ProcessDir, "backlog_items", "deadbeef.yaml"), "test")

	cache := NewObjectIDCache()
	refCtx := &ReferenceCheckContext{
		Ctx: cli.ContextForProjectRoot(root),
	}
	issues := validateReferenceWithCache(refCtx, "BLI-pending-1", "related_object_refs", cache)
	if len(issues) != 1 {
		t.Fatalf("issues=%v", issues)
	}
	if issues[0].Tier != 3 || issues[0].Category != "cache_coherence" {
		t.Fatalf("want Tier-3 cache_coherence, got %+v", issues[0])
	}
}

func TestValidateReferenceWithCache_TrueMissStillTier1(t *testing.T) {
	root := t.TempDir()
	storage.ResetObjectIDCachePendingForTest()
	t.Cleanup(storage.ResetObjectIDCachePendingForTest)

	cache := NewObjectIDCache()
	refCtx := &ReferenceCheckContext{
		Ctx: cli.ContextForProjectRoot(root),
	}
	issues := validateReferenceWithCache(refCtx, "BLI-missing-zzz", "related_object_refs", cache)
	if len(issues) != 1 || issues[0].Tier != 1 {
		t.Fatalf("want Tier-1 miss, got %+v", issues)
	}
}
