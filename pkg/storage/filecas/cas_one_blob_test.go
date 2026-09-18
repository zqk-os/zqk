package filecas

import (
	"path/filepath"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestEnsureExactlyOneLiveCASBlob_RemovesExtraAndKeepsHash(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	keeper := "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
	extra := "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd"
	keeperPath := filepath.Join(dir, keeper+".yaml")
	extraPath := filepath.Join(dir, extra+".yaml")
	body := []byte("id: REQ-ONE-BLOB\nkind: requirement\n")
	if err := fileutil.WriteStandardFile(keeperPath, body); err != nil {
		t.Fatal(err)
	}
	if err := fileutil.WriteStandardFile(extraPath, body); err != nil {
		t.Fatal(err)
	}
	if err := ensureExactlyOneLiveCASBlob("REQ-ONE-BLOB", keeper, dir); err != nil {
		t.Fatal(err)
	}
	if !fileutil.Exists(keeperPath) {
		t.Fatal("keeper blob must remain")
	}
	if fileutil.Exists(extraPath) {
		t.Fatal("extra blob must be removed before Update returns")
	}
}
