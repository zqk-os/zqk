package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestRemoveOrphanCASHashFileSync_renamesThenRemoves(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a1b2c3d4e5f6789012345678901234567890123456789012345678901234.yaml")
	if err := fileutil.WriteSecureFile(p, []byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := removeOrphanCASHashFileSync(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("expected original gone: %v", err)
	}
	tmp := p + ".tmp"
	if _, err := os.Stat(tmp); !os.IsNotExist(err) {
		t.Fatalf("expected .tmp gone: %v", err)
	}
}

func TestRemoveOrphanCASHashFileSync_idempotentWhenMissing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "0000000000000000000000000000000000000000000000000000000000000001.yaml")
	if err := removeOrphanCASHashFileSync(p); err != nil {
		t.Fatal(err)
	}
}
