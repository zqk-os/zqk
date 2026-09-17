package cas_test

import (
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/storage"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestEnsureCASIndexMatchesContentHash_RepairsDrift proves the Tier-1 post-write
// invariant: when the index points at the wrong hash, ensure repairs via SetMapping.
// TRACK: [REDACTED-ID]
func TestEnsureCASIndexMatchesContentHash_RepairsDrift(t *testing.T) {
	tmp := t.TempDir()
	kindDir := filepath.Join(tmp, "backlog")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cas := filecas.NewContentAddressableStorage(kindDir, objects.KindBacklogItem)

	objectID := "BLI-postwrite-001"
	data := []byte("id: BLI-postwrite-001\nkind: backlog_item\nschema_version: \"2.0.0\"\n")
	want := storage.CalculateSHA256Hash(data)
	hashFile := filepath.Join(kindDir, want+".yaml")
	if err := fileutil.WriteFile(hashFile, data, paths.FilePerm644); err != nil {
		t.Fatalf("write hash file: %v", err)
	}

	wrong := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if err := cas.GetIndex().SetMapping(objectID, wrong); err != nil {
		t.Fatalf("seed wrong mapping: %v", err)
	}
	got, _ := cas.GetIndex().GetHash(objectID)
	if got != wrong {
		t.Fatalf("precondition: got %s want wrong %s", got, wrong)
	}

	if err := cas.EnsureCASIndexMatchesContentHash(objectID, data, ""); err != nil {
		t.Fatalf("ensure: %v", err)
	}
	got, err := cas.GetIndex().GetHash(objectID)
	if err != nil {
		t.Fatalf("GetHash after ensure: %v", err)
	}
	if got != want {
		t.Fatalf("after ensure GetHash=%s want %s", got, want)
	}
}

func TestEnsureCASIndexMatchesContentHash_NoopWhenAligned(t *testing.T) {
	tmp := t.TempDir()
	kindDir := filepath.Join(tmp, "backlog")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cas := filecas.NewContentAddressableStorage(kindDir, objects.KindBacklogItem)
	objectID := "BLI-postwrite-002"
	data := []byte("id: BLI-postwrite-002\nkind: backlog_item\n")
	want := storage.CalculateSHA256Hash(data)
	if err := cas.GetIndex().SetMapping(objectID, want); err != nil {
		t.Fatalf("SetMapping: %v", err)
	}
	if err := cas.EnsureCASIndexMatchesContentHash(objectID, data, ""); err != nil {
		t.Fatalf("ensure: %v", err)
	}
}
