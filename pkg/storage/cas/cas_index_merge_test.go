package cas_test

import (
	"github.com/zqk-os/zqk/pkg/storage/filecas"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
)

func TestMergeCASIndexMaps_StaleMemoryDoesNotClobberHealedDisk(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	oldHash := strings.Repeat("aa", 32)
	newHash := strings.Repeat("bb", 32)
	if err := fileutil.WriteFile(filepath.Join(kindDir, newHash+".yaml"), []byte("id: BLI-1\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	disk := map[string]string{"BLI-1": newHash}
	memory := map[string]string{"BLI-1": oldHash}
	got := filecas.MergeCASIndexMaps(kindDir, disk, memory)
	if got["BLI-1"] != newHash {
		t.Fatalf("want healed disk hash %s, got %s", newHash, got["BLI-1"])
	}
}

func TestMergeCASIndexMaps_MemoryWinsWhenDiskHashFileMissing(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	oldHash := strings.Repeat("cc", 32)
	newHash := strings.Repeat("dd", 32)
	if err := fileutil.WriteFile(filepath.Join(kindDir, newHash+".yaml"), []byte("id: BLI-2\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	disk := map[string]string{"BLI-2": oldHash} // points at deleted blob
	memory := map[string]string{"BLI-2": newHash}
	got := filecas.MergeCASIndexMaps(kindDir, disk, memory)
	if got["BLI-2"] != newHash {
		t.Fatalf("want memory hash when disk blob missing, got %s", got["BLI-2"])
	}
}

func TestSetMapping_DoesNotRevertForeignHeal(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	oldHash := strings.Repeat("11", 32)
	newHash := strings.Repeat("22", 32)
	otherNew := strings.Repeat("33", 32)
	if err := fileutil.WriteFile(filepath.Join(kindDir, newHash+".yaml"), []byte("id: BLI-A\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteFile(filepath.Join(kindDir, otherNew+".yaml"), []byte("id: BLI-B\n"), paths.FilePerm644); err != nil {
		t.Fatal(err)
	}

	cas := filecas.NewContentAddressableStorage(kindDir, "backlog_item")
	// Stale process view
	cas.GetIndex().Mappings["BLI-A"] = oldHash
	cas.GetIndex().Mappings["BLI-B"] = oldHash

	// Foreign heal on disk (sync-cas-index)
	healed := map[string]string{"BLI-A": newHash, "BLI-B": otherNew}
	if err := cas.GetIndex().SetMappings(healed, nil); err != nil {
		t.Fatal(err)
	}

	// Simulate stale cache still holding old in-memory for a third touch
	casStale := filecas.NewContentAddressableStorage(kindDir, "backlog_item")
	casStale.GetIndex().Mappings["BLI-A"] = oldHash
	casStale.GetIndex().Mappings["BLI-B"] = oldHash
	if err := casStale.GetIndex().SetMapping("BLI-A", newHash); err != nil {
		t.Fatal(err)
	}

	reloaded := filecas.NewContentAddressableStorage(kindDir, "backlog_item")
	if got := reloaded.GetIndex().Mappings["BLI-B"]; got != otherNew {
		t.Fatalf("SetMapping must not revert peer heal for BLI-B: got %s want %s", got, otherNew)
	}
	if got := reloaded.GetIndex().Mappings["BLI-A"]; got != newHash {
		t.Fatalf("BLI-A want %s got %s", newHash, got)
	}
}
