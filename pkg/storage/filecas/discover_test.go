package filecas

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func writeCASYAML(t *testing.T, dir, id string) string {
	t.Helper()
	body := []byte("id: " + id + "\nkind: goal\n")
	path := filepath.Join(dir, CalculateSHA256Hash(body)+".yaml")
	if err := fileutil.WriteSecureFile(path, body); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDiscoverCASFilePathByScanning(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	id := "GOAL-DISCOVER-1"
	wantPath := writeCASYAML(t, kindDir, id)

	gotPath, hash, err := DiscoverCASFilePathByScanning(id, kindDir)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != wantPath {
		t.Fatalf("path=%q want %q", gotPath, wantPath)
	}
	if !CasHashFilenameRe.MatchString(hash + ".yaml") {
		t.Fatalf("hash=%q is not a CAS filename stem", hash)
	}

	_, _, err = DiscoverCASFilePathByScanning("GOAL-MISSING", kindDir)
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !strings.Contains(err.Error(), "GOAL-MISSING") {
		t.Fatalf("error=%v", err)
	}
}

func TestDiscoverCASFilePathsByScanning_bucketAndEmpty(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	bucket := filepath.Join(kindDir, "ab")
	if err := fileutil.Mkdir(bucket, 0o755); err != nil {
		t.Fatal(err)
	}
	id := "GOAL-BUCKET-1"
	wantPath := writeCASYAML(t, bucket, id)

	found, err := DiscoverCASFilePathsByScanning([]string{id, ""}, kindDir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := found[id]
	if !ok {
		t.Fatalf("missing %s in %+v", id, found)
	}
	if got.path != wantPath {
		t.Fatalf("bucket path=%q want %q", got.path, wantPath)
	}

	empty, err := DiscoverCASFilePathsByScanning(nil, kindDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty want=0 got=%d", len(empty))
	}
}
