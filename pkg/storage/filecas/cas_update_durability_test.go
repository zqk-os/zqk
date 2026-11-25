package filecas

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestEnsureDurableHashFileOnDisk_ReMaterializesMissingBlob(t *testing.T) {
	dir := t.TempDir()
	cas := &ContentAddressableStorage{
		kind:    "test_case",
		kindDir: dir,
	}

	data := []byte("id: TST-DURABLE-001\nkind: test_case\nstatus: originated\n")
	hash := CalculateSHA256Hash(data)
	newHashFile := filepath.Join(dir, hash+".yaml")

	// File does not exist yet: ensureDurableHashFileOnDisk must re-materialize it.
	if err := cas.ensureDurableHashFileOnDisk(newHashFile, data); err != nil {
		t.Fatalf("ensureDurableHashFileOnDisk failed: %v", err)
	}
	if _, err := fileutil.Stat(newHashFile); err != nil {
		t.Fatalf("expected re-materialized hash file %s to exist on disk: %v", newHashFile, err)
	}
}
