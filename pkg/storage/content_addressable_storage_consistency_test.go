package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestContentAddressableStorage_Create_ReadImmediate(t *testing.T) {
	kindDir := t.TempDir()
	kind := "backlog_item"
	casQueue := NewListingIndexWriteQueueForTest()
	defer casQueue.Shutdown()
	cas := NewContentAddressableStorage(kindDir, kind, casQueue)

	objectID := "ITEM-0001"
	content := []byte("id: " + objectID + "\nkind: " + kind + "\nschema_version: \"" + objects.DefaultSchemaVersion + "\"\n")
	expectedHash := CalculateSHA256Hash(content)

	if err := cas.Create(objectID, content); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Must be readable immediately in the same process (index is updated in-memory).
	if _, err := cas.Read(objectID); err != nil {
		t.Fatalf("Read failed: %v", err)
	}

	// Ensure the content-addressed file exists.
	if _, err := os.Stat(filepath.Join(kindDir, expectedHash+".yaml")); err != nil {
		t.Fatalf("expected hash file to exist: %v", err)
	}

	// Flush background index updates to avoid async writes racing test cleanup.
	_ = GetGlobalListingIndexWriteQueue().FlushKind(kind, 2*time.Second)
}
