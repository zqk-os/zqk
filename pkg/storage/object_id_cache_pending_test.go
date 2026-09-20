package storage

import (
	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TRACK: BLI-1785895580100186000-c5539372
func TestObjectIDCachePending_NoteClearIs(t *testing.T) {
	root := t.TempDir()
	ResetObjectIDCachePendingForTest()
	t.Cleanup(ResetObjectIDCachePendingForTest)

	if IsObjectIDCachePending(root, "GOAL-1") {
		t.Fatal("expected no pending")
	}
	NoteObjectIDCachePending(root, string(ObjectIDCachePendingOpUpdate), "GOAL-1", "goal", filepath.Join(root, paths.ProcessDir, "goals/abc.yaml"), "test")
	if !IsObjectIDCachePending(root, "GOAL-1") {
		t.Fatal("expected pending after Note")
	}
	if ObjectIDCachePendingGeneration(root) == 0 {
		t.Fatal("expected generation > 0")
	}
	list := ListObjectIDCachePending(root)
	if len(list) != 1 || list[0].ID != "GOAL-1" {
		t.Fatalf("list=%v", list)
	}
	// Durable file exists
	if _, err := fileutil.Stat(objectIDCachePendingPath(root)); err != nil {
		t.Fatalf("pending file: %v", err)
	}
	ClearObjectIDCachePending(root, "GOAL-1")
	if IsObjectIDCachePending(root, "GOAL-1") {
		t.Fatal("expected cleared")
	}
}

func TestObjectIDCachePending_RefusesDraftPlanePath(t *testing.T) {
	root := t.TempDir()
	ResetObjectIDCachePendingForTest()
	t.Cleanup(ResetObjectIDCachePendingForTest)

	draft := ObjectDraftPlanePath(root, "goal", "GOAL-draft-1")
	if err := fileutil.MkdirAll(filepath.Dir(draft), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(draft, []byte("id: GOAL-draft-1\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	NoteObjectIDCachePending(root, string(ObjectIDCachePendingOpUpdate), "GOAL-draft-1", "goal", draft, "draft_write")
	if IsObjectIDCachePending(root, "GOAL-draft-1") {
		t.Fatal("draft-plane path must not enter pending journal")
	}
}

func TestCASPostSync_NilHandlerFailsClosed(t *testing.T) {
	tmpDir := t.TempDir()
	ResetObjectIDCachePendingForTest()
	t.Cleanup(ResetObjectIDCachePendingForTest)

	processDir := datacell.ProcessPrimaryDir(tmpDir)
	if err := fileutil.MkdirAll(filepath.Join(processDir, "_internal", "object_specs"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.MkdirAll(filepath.Join(processDir, "goals"), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}

	store, err := NewFileObjectStorageForTest(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = store.Shutdown(context.Background()) }()

	prevHandler := GetCacheOperationHandler()
	t.Cleanup(func() { SetCacheOperationHandler(prevHandler) })
	SetCacheOperationHandler(nil)

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := context.Background()
	obj := map[string]any{
		objects.FieldKeyID:            "GOAL-pending-nil",
		objects.FieldKeyKind:          "goal",
		objects.FieldKeyTitle:         "Pending Nil",
		objects.FieldKeyStatus:        objects.ObjectStatusActive,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := store.Create(ctx, secCtx, obj); err == nil {
		t.Fatal("Create must fail closed when object-id-cache handler is nil")
	}
}

func TestGetFilePathForID_DropsGhostMapping(t *testing.T) {
	tmpDir := t.TempDir()
	kindDir := filepath.Join(datacell.ProcessPrimaryDir(tmpDir), "goals")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	cas := filecas.NewContentAddressableStorage(kindDir, "goal")
	if cas == nil || cas.GetIndex() == nil {
		t.Fatal("cas nil")
	}
	ghostHash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	cas.SetIndexMappingInMemory("GOAL-ghost", ghostHash, "")
	_, err := cas.GetFilePathForID("GOAL-ghost")
	if err == nil {
		t.Fatal("expected miss for ghost")
	}
	if _, gerr := cas.GetIndex().GetHash("GOAL-ghost"); gerr == nil {
		t.Fatal("ghost mapping should be removed from index")
	}
}

// TRACK: BLI-CEF-R16-OIDCACHE-SWALLOW-001 / CRIT-CEF-R2-REL-OIDCACHE-SWALLOW-A / REQ-CEF-R2-REL-OIDCACHE-SWALLOW
func TestObjectIDCachePending_LoadErrorDoesNotClobberCorruptJournal(t *testing.T) {
	root := t.TempDir()
	ResetObjectIDCachePendingForTest()
	t.Cleanup(ResetObjectIDCachePendingForTest)

	path := objectIDCachePendingPath(root)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("{not json")
	if err := fileutil.WriteFile(path, corrupt, paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	NoteObjectIDCachePending(root, string(ObjectIDCachePendingOpUpdate), "GOAL-NEW", "goal", filepath.Join(root, paths.ProcessDir, "goals/abc.yaml"), "test")
	if ObjectIDCachePendingLastIOError(root) == nil {
		t.Fatal("expected load I/O error to be retained")
	}
	got, err := fileutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(corrupt) {
		t.Fatalf("corrupt journal must not be overwritten, got %q", got)
	}
}

// TRACK: BLI-CEF-R2-REL-OIDCACHE-SWALLOW
func TestObjectIDCachePending_PersistErrorIsRetained(t *testing.T) {
	root := t.TempDir()
	ResetObjectIDCachePendingForTest()
	t.Cleanup(ResetObjectIDCachePendingForTest)

	zqkDir := filepath.Join(root, ".zqk")
	if err := fileutil.MkdirAll(zqkDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	// Make .zqk/cache a file so EnsureDir(cache) fails closed.
	if err := fileutil.WriteFile(filepath.Join(zqkDir, "cache"), []byte("not-a-dir"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	NoteObjectIDCachePending(root, string(ObjectIDCachePendingOpUpdate), "GOAL-1", "goal", filepath.Join(root, paths.ProcessDir, "goals/abc.yaml"), "test")
	if ObjectIDCachePendingLastIOError(root) == nil {
		t.Fatal("expected persist I/O error")
	}
}

// TestObjectIDCache_BLI_CEF_R16_OIDCACHE_SWALLOW_001 verifies that disk read/write failures
// in the ObjectIDCache pending journal fail closed and are surfaced rather than silently swallowed.
func TestObjectIDCache_BLI_CEF_R16_OIDCACHE_SWALLOW_001(t *testing.T) {
	root := t.TempDir()
	ResetObjectIDCachePendingForTest()
	t.Cleanup(ResetObjectIDCachePendingForTest)

	zqkDir := filepath.Join(root, ".zqk")
	if err := fileutil.MkdirAll(zqkDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(zqkDir, "cache"), []byte("not-a-dir"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	NoteObjectIDCachePending(root, string(ObjectIDCachePendingOpUpdate), "GOAL-1", "goal", filepath.Join(root, paths.ProcessDir, "goals/abc.yaml"), "test")
	if err := ObjectIDCachePendingLastIOError(root); err == nil {
		t.Fatal("expected non-nil disk I/O error when cache dir is uncreatable")
	}
}
