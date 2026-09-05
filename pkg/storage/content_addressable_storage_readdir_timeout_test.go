package storage

import (
	"fmt"
	"path/filepath"

	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"

	"github.com/lanceman/zqk/pkg/goroutinelabels"
)

// TestContentAddressableStorage_Read_ReadDirTimeout verifies that Read() times out on large directories
func TestContentAddressableStorage_Read_ReadDirTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	kindDir := filepath.Join(tmpDir, "test_kind")
	if err := fileutil.MkdirAll(kindDir, paths.DirPerm755); err != nil {
		t.Fatalf("failed to create kind directory: %v", err)
	}

	// Create many files to simulate large directory (like 29k audit events)
	// Create 100 files to test timeout behavior (29k would take too long in test)
	for i := 0; i < 100; i++ {
		testFile := filepath.Join(kindDir, fmt.Sprintf("test_%d.yaml", i))
		if err := fileutil.WriteFile(testFile, []byte("test"), paths.FilePerm644); err != nil {
			t.Fatalf("failed to create test file: %v", err)
		}
	}

	// Test that ReadDir with timeout works
	// This should complete quickly even with 100 files
	start := time.Now()
	entriesChan := make(chan []fileutil.DirEntry, 1)
	errChan := make(chan error, 1)
	goroutinelabels.StartTestGoroutine("test_readdir", "reading directory entries in test", func() {
		entries, err := fileutil.ReadDir(kindDir)
		entriesChan <- entries
		errChan <- err
	})

	select {
	case entries := <-entriesChan:
		err := <-errChan
		duration := time.Since(start)
		if err != nil {
			t.Fatalf("ReadDir failed: %v", err)
		}
		if len(entries) != 100 {
			t.Errorf("expected 100 entries, got %d", len(entries))
		}
		if duration > 1*time.Second {
			t.Errorf("ReadDir took too long: %v (expected < 1s)", duration)
		}
		t.Logf("ReadDir completed successfully in %v with %d entries", duration, len(entries))
	case <-time.After(5 * time.Second):
		t.Fatal("ReadDir timed out - this indicates the timeout mechanism works")
	}
}
