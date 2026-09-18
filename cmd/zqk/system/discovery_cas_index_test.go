package system

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

func TestScanObjectFilesWithContext_UsesCASIndexFastPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	kind := "backlog_item"
	kindDir := datacell.CellCASPrimaryDir(tmp, "backlog_item")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir kind dir: %v", err)
	}

	// Write a minimal CAS index file: .<kind>.index
	objectID := "BLI-001"
	hash := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	indexPath := filepath.Join(kindDir, "."+kind+".index")
	indexPayload := map[string]any{
		objects.FieldKeyVersion: "1.0",
		objects.FieldKeyKind:    kind,
		"mappings":              map[string]string{objectID: hash},
	}
	data, err := json.Marshal(indexPayload)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}
	if err := fileutil.WriteFile(indexPath, data, paths.FilePerm644); err != nil { //nolint:gosec // test file perms OK
		t.Fatalf("write index: %v", err)
	}

	// Create the corresponding hash file so the fast path can stat it.
	hashFile := filepath.Join(kindDir, hash+".yaml")
	if err := fileutil.WriteFile(hashFile, []byte("id: "+objectID+"\nkind: "+kind+"\n"), paths.FilePerm644); err != nil { //nolint:gosec // test file perms OK
		t.Fatalf("write hash file: %v", err)
	}

	files, err := scanObjectFilesWithContext(context.Background(), kindDir, kind, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), nil)
	if err != nil {
		t.Fatalf("scanObjectFilesWithContext error: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	if files[0].ObjectID != objectID {
		t.Fatalf("expected objectID %q, got %q", objectID, files[0].ObjectID)
	}
	if files[0].Path != hashFile {
		t.Fatalf("expected path %q, got %q", hashFile, files[0].Path)
	}
}

func TestProcessKindForDiscovery_ExcludesDraftPlane(t *testing.T) {
	tmpDir, fileStorage, _ := storage.SetupTestingFactoryCompleteTestEnvironmentForTest(t)
	secCtx := pkgctx.NewSystemSecurityContext()
	ctx := storage.WithCLIOperation(pkgctx.NewSystemContext())

	draftID := "DOC-draft-disc-001"
	draftDoc := map[string]any{
		objects.FieldKeyID:            draftID,
		objects.FieldKeyKind:          objects.KindDocEntry,
		objects.FieldKeyTitle:         "Draft Doc",
		objects.FieldKeyStatus:        objects.ObjectStatusDraft,
		objects.FieldKeySchemaVersion: objects.DefaultSchemaVersion,
	}
	if err := fileStorage.Create(ctx, secCtx, draftDoc); err != nil {
		t.Fatalf("Create draft doc: %v", err)
	}

	filesChan := make(chan []scannedFile, 1)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	err := processKindForDiscovery(tmpDir, objects.KindDocEntry, []string{draftID}, logger, filesChan, nil, ctx, fileStorage, string(pkgctx.ProfileSystem))
	if err != nil {
		t.Fatalf("processKindForDiscovery failed: %v", err)
	}

	select {
	case files := <-filesChan:
		for _, f := range files {
			if f.ObjectID == draftID {
				t.Fatalf("processKindForDiscovery should NOT return draft-plane object %s", draftID)
			}
		}
	default:
		// Channel empty is expected because the only targetID was draft-plane and got filtered out!
	}
}
