package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"gopkg.in/yaml.v3"
)

// TestReadObjectFile_FullStreamRead verifies that readObjectFile and readObjectFileNoCache
// read the complete content of large YAML files without premature EOF or partial truncation.
func TestReadObjectFile_FullStreamRead(t *testing.T) {
	tmpDir := t.TempDir()
	largeValue := strings.Repeat("abcdef0123456789", 4096) // 64 KB of data
	testObj := map[string]any{
		"id":          "test-full-stream-obj",
		"kind":        "requirement",
		"title":       "Test Full Stream",
		"large_field": largeValue,
	}

	data, err := yaml.Marshal(testObj)
	if err != nil {
		t.Fatalf("failed to marshal YAML: %v", err)
	}

	testFilePath := filepath.Join(tmpDir, "test_large.yaml")
	if err := fileutil.WriteFile(testFilePath, data, paths.FilePerm644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	storage := &FileObjectStorage{
		projectRoot: tmpDir,
	}

	// 1. Test readObjectFileNoCache directly
	readObjNoCache, err := storage.readObjectFileNoCache(testFilePath)
	if err != nil {
		t.Fatalf("readObjectFileNoCache failed: %v", err)
	}
	if readObjNoCache["large_field"] != largeValue {
		t.Fatalf("readObjectFileNoCache returned truncated or corrupted data")
	}

	// 2. Test ReadObjectFile exported method
	readObj, err := storage.ReadObjectFile(context.Background(), testFilePath)
	if err != nil {
		t.Fatalf("ReadObjectFile failed: %v", err)
	}
	if readObj["large_field"] != largeValue {
		t.Fatalf("ReadObjectFile returned truncated or corrupted data")
	}
}
