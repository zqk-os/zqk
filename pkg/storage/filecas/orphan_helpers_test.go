package filecas

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestRemoveRetiredCASHashFileOnIDChange(t *testing.T) {
	t.Parallel()
	if err := RemoveRetiredCASHashFileOnIDChange(""); err != nil {
		t.Fatalf("empty path: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, CalculateSHA256Hash([]byte("retire"))+".yaml")
	if err := fileutil.WriteSecureFile(path, []byte("id: GOAL-RETIRE\n")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveRetiredCASHashFileOnIDChange(path); err != nil {
		t.Fatal(err)
	}
	if _, err := fileutil.Stat(path); !fileutil.IsNotExist(err) {
		t.Fatalf("retired file still present: %v", err)
	}
}

func TestRemoveOrphanCASHashFileSync(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, CalculateSHA256Hash([]byte("orphan"))+".yaml")
	if err := fileutil.WriteSecureFile(path, []byte("id: GOAL-ORPHAN\n")); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOrphanCASHashFileSync(path); err != nil {
		t.Fatal(err)
	}
	if _, err := fileutil.Stat(path); !fileutil.IsNotExist(err) {
		t.Fatalf("orphan still present: %v", err)
	}
	if _, err := fileutil.Stat(path + ".tmp"); !fileutil.IsNotExist(err) {
		t.Fatalf("tmp leftover: %v", err)
	}
	if err := RemoveOrphanCASHashFileSync(path); err != nil {
		t.Fatalf("missing file should be ok: %v", err)
	}
}
