package storage

import (
	"fmt"
	"path/filepath"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

func TestListSegmentsParallel(t *testing.T) {
	// Setup test root
	tmpDir := t.TempDir()
	dataDir := filepath.Join(tmpDir, "data", "streams", "test_kind")
	err := fileutil.EnsureDir(dataDir)
	if err != nil {
		t.Fatal(err)
	}

	// Create dummy JSON files
	for i := 0; i < 5; i++ {
		_ = fileutil.WriteSecureFile(filepath.Join(dataDir, fmt.Sprintf("seg_00%d.json", i)), []byte("{}"))
	}
	_ = fileutil.WriteSecureFile(filepath.Join(dataDir, "other.txt"), []byte("data"))

	storage := &FileObjectStorage{projectRoot: tmpDir}

	ctx := pkgctx.NewSystemContext()
	segments, err := storage.ListSegmentsParallel(ctx, "test_kind", 2)
	if err != nil {
		t.Fatalf("ListSegmentsParallel failed: %v", err)
	}

	if len(segments) != 5 {
		t.Errorf("Expected 5 segments, got %d", len(segments))
	}
}
