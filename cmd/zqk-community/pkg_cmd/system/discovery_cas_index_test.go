package system

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/datacell"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
)

func TestScanObjectFilesWithContext_UsesCASIndexFastPath(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()

	kind := "backlog_item"
	kindDir := datacell.CellCASPrimaryDir(tmp, "backlog_item")
	if err := os.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir kind dir: %v", err)
	}

	// Write a minimal CAS index file: .<kind>.index
	objectID := "ITEM-001"
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
	if err := os.WriteFile(indexPath, data, paths.FilePerm644); err != nil { //nolint:gosec // test file perms OK
		t.Fatalf("write index: %v", err)
	}

	// Create the corresponding hash file so the fast path can stat it.
	hashFile := filepath.Join(kindDir, hash+".yaml")
	if err := os.WriteFile(hashFile, []byte("id: "+objectID+"\nkind: "+kind+"\n"), paths.FilePerm644); err != nil { //nolint:gosec // test file perms OK
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
