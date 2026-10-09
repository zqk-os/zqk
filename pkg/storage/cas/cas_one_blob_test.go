package cas_test

import (
	"github.com/zqk-os/zqk/pkg/storage"
	caspkg "github.com/zqk-os/zqk/pkg/storage/cas"

	"context"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRemoveOrphanCASHashFileSync_DeletesSoleLookingPredecessorWhenKeeperKnown(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.yaml")
	if err := fileutil.WriteStandardFile(oldPath, []byte("id: CRIT-KEEPER\nkind: criteria\n")); err != nil {
		t.Fatal(err)
	}
	keeper := "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := caspkg.RemoveOrphanCASHashFileSync(oldPath, keeper); err != nil {
		t.Fatal(err)
	}
	if fileutil.Exists(oldPath) {
		t.Fatal("predecessor must be deleted when Update names the keeper hash")
	}
}

func TestFileObjectStorage_UpdateLeavesExactlyOneCASBlob(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	storage.MustEnsureProcessSpecsLayoutForTest(t, root)
	kindDir := filepath.Join(datacell.ProcessPrimaryDir(root), objects.GetDirectoryFromKind(objects.KindBacklogItem))
	if err := paths.EnsureDir(kindDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	fos, err := storage.NewFileObjectStorageForTest(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		opts := storage.TempProjectTeardown(root, fos)
		if err := storage.RunProjectTestTeardown(opts); err != nil {
			t.Logf("teardown: %v", err)
		}
	})

	secCtx := pkgctx.NewSecurityContext("ACC-TEST", []string{"admin"}, []string{"read:*", "write:*"})
	ctx := pkgctx.WithPromoteOnCreate(context.Background())
	id := "BLI-CAS-ONE-BLOB-001"
	obj := map[string]any{
		objects.FieldKeyID:            id,
		objects.FieldKeyKind:          objects.KindBacklogItem,
		objects.FieldKeyTitle:         "first",
		objects.FieldKeyDescription:   "Valid substantive description for one blob test",
		objects.FieldKeyStatus:        objects.ObjectStatusValidated,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := fos.Create(ctx, secCtx, obj); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := fos.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyTitle: "second"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := fos.Update(ctx, secCtx, id, map[string]any{objects.FieldKeyTitle: "third"}); err != nil {
		t.Fatalf("second update: %v", err)
	}

	// Do not wait for the orphan worker — Update must already have one blob.
	matches, err := filepath.Glob(filepath.Join(kindDir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var hashes []string
	for _, p := range matches {
		if filecas.CasHashFilenameRe.MatchString(filepath.Base(p)) && filecas.CasHashFilePeekObjectID(p) == id {
			hashes = append(hashes, filepath.Base(p))
		}
	}
	if len(hashes) != 1 {
		t.Fatalf("want exactly 1 live CAS blob after FileObjectStorage.Update (no queue wait), got %d: %v", len(hashes), hashes)
	}
}

func TestCAS_UpdateSweepsPlantedPredecessorWithoutQueueWait(t *testing.T) {
	t.Parallel()
	testRoot := t.TempDir()
	kindDir := filepath.Join(datacell.ProcessPrimaryDir(testRoot), objects.GetDirectoryFromKind(objects.KindBacklogItem))
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatal(err)
	}
	casQueue := caspkg.NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := storage.NewContentAddressableStorage(kindDir, objects.KindBacklogItem, casQueue)

	objectID := "BLI-planted-leak-001"
	data1 := []byte("id: " + objectID + "\nkind: " + objects.KindBacklogItem + "\ntitle: First\nstatus: exploring\nschema_version: \"" + objects.DefaultSchemaVersion + "\"\n")
	if err := cas.Create(objectID, data1); err != nil {
		t.Fatal(err)
	}
	planted := filepath.Join(kindDir, "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee.yaml")
	if err := fileutil.WriteStandardFile(planted, []byte("id: "+objectID+"\nkind: "+objects.KindBacklogItem+"\ntitle: leaked\n")); err != nil {
		t.Fatal(err)
	}
	data2 := []byte("id: " + objectID + "\nkind: " + objects.KindBacklogItem + "\ntitle: Second\nstatus: exploring\nschema_version: \"" + objects.DefaultSchemaVersion + "\"\n")
	if err := cas.Update(objectID, data2); err != nil {
		t.Fatal(err)
	}
	if fileutil.Exists(planted) {
		t.Fatal("planted predecessor must be gone when Update returns")
	}
	matches, err := filepath.Glob(filepath.Join(kindDir, "*.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, p := range matches {
		if filecas.CasHashFilenameRe.MatchString(filepath.Base(p)) && filecas.CasHashFilePeekObjectID(p) == objectID {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("want 1 blob, got %d", n)
	}
}
