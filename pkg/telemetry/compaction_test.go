package telemetry

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCompactOldSegments(t *testing.T) {
	dir := t.TempDir()

	// Create an old file
	oldPath := filepath.Join(dir, "old_stream.jsonl")
	err := os.WriteFile(oldPath, []byte("data"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Force mod time to be 10 days ago
	oldTime := time.Now().Add(-240 * time.Hour)
	err = os.Chtimes(oldPath, oldTime, oldTime)
	if err != nil {
		t.Fatal(err)
	}

	// Create a new file
	newPath := filepath.Join(dir, "new_stream.json")
	err = os.WriteFile(newPath, []byte("data"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Create an old file with uninteresting extension, shouldn't be deleted
	ignoredPath := filepath.Join(dir, "old_file.txt")
	err = os.WriteFile(ignoredPath, []byte("data"), 0644)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Chtimes(ignoredPath, oldTime, oldTime)
	if err != nil {
		t.Fatal(err)
	}

	// Run compaction for files older than 5 days
	count, err := CompactOldSegments([]string{dir}, 120*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if count != 1 {
		t.Errorf("expected 1 file to be deleted, got %d", count)
	}

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("expected old file to be deleted")
	}

	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("expected new file to exist")
	}

	if _, err := os.Stat(ignoredPath); err != nil {
		t.Errorf("expected ignored old file to exist")
	}
}
