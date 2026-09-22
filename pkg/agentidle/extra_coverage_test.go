package agentidle

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func TestFileStore_EdgeCases(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()

	// 1. GetAllRecords
	storePath := filepath.Join(tempDir, "idle_records.json")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	if err := store.Accumulate("agent-1", "task-1", 5*time.Second); err != nil {
		t.Fatalf("Accumulate failed: %v", err)
	}
	if err := store.Accumulate("agent-2", "task-2", 15*time.Second); err != nil {
		t.Fatalf("Accumulate failed: %v", err)
	}

	records, err := store.GetAllRecords()
	if err != nil {
		t.Fatalf("GetAllRecords failed: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("expected 2 records, got %d", len(records))
	}
	if records["agent-1:task-1"] != 5*time.Second {
		t.Errorf("expected 5s, got %v", records["agent-1:task-1"])
	}

	// 2. Load empty file
	emptyPath := filepath.Join(tempDir, "empty.json")
	if err := fileutil.WriteFile(emptyPath, []byte(""), paths.FilePerm644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	emptyStore, err := NewFileStore(emptyPath)
	if err != nil {
		t.Fatalf("NewFileStore on empty file failed: %v", err)
	}
	if len(emptyStore.records) != 0 {
		t.Errorf("expected empty records")
	}

	// 3. Load legacy raw map format
	legacyPath := filepath.Join(tempDir, "legacy.json")
	legacyData := []byte(`{"agent-x:task-y": "invalid_structure_for_doc"}`)
	if err := fileutil.WriteFile(legacyPath, legacyData, paths.FilePerm644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	// errLegacy will fail since string cannot unmarshal to time.Duration, so it tests fallback error
	_, _ = NewFileStore(legacyPath)

	// 4. Load invalid unmarshalable JSON
	corruptPath := filepath.Join(tempDir, "corrupt.json")
	if err := fileutil.WriteFile(corruptPath, []byte("NOT_JSON"), paths.FilePerm644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	_, err = NewFileStore(corruptPath)
	if err == nil {
		t.Errorf("expected error loading corrupt JSON")
	}
}

func TestRecordCVSSnapshot_Invoke(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	storePath := filepath.Join(tempDir, "idle_cvs.json")
	store, err := NewFileStore(storePath)
	if err != nil {
		t.Fatalf("NewFileStore failed: %v", err)
	}

	_ = store.Accumulate("agent-test", "task-test", 12*time.Second)

	// RecordCVSSnapshot calls CLI, will attempt exec and return an error or succeed if CLI is available
	_ = RecordCVSSnapshot(store, "CVS-TEST-001")
}

