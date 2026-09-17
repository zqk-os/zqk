package storage_test

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/storage/filecas"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// TestFileObjectStorage_StreamSegmentIdenticalChunksShareOneBlob verifies CRIT-REDACTED:
// Two identical stream-segment payloads persist as one content-addressed blob (compression allowed)
// and object get rehydrates the original bytes through FileObjectStorage.
// Covering BLI-REDACTED and BLI-REDACTED.
func TestFileObjectStorage_StreamSegmentIdenticalChunksShareOneBlob(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	storage.MustEnsureProcessSpecsLayoutForTest(t, root)
	fos, err := storage.NewFileObjectStorageForTest(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fos.Shutdown(context.Background()) })

	payload := []byte(strings.Repeat("event_type: stream_segment_event\naction: cas_dedup_verify\nactor: agent-lead\n", 40))
	expectedHash := filecas.CalculateSHA256Hash(payload)

	// Step 1: Put first stream segment chunk
	hash1, err := fos.PutStreamSegmentChunk(objects.KindAuditEvent, "AUDIT-SEG-CHUNK-001", payload)
	if err != nil {
		t.Fatalf("first PutStreamSegmentChunk failed: %v", err)
	}
	if hash1 != expectedHash {
		t.Fatalf("expected hash %s, got %s", expectedHash, hash1)
	}

	// Step 2: Put identical stream segment chunk under a second ID
	hash2, err := fos.PutStreamSegmentChunk(objects.KindAuditEvent, "AUDIT-SEG-CHUNK-002", payload)
	if err != nil {
		t.Fatalf("second PutStreamSegmentChunk failed: %v", err)
	}
	if hash2 != expectedHash {
		t.Fatalf("expected hash %s, got %s", expectedHash, hash2)
	}

	// Step 3: Verify single chunk blob on disk (deduplication)
	cas, err := fos.GetContentAddressableStorage(objects.KindAuditEvent)
	if err != nil {
		t.Fatalf("GetContentAddressableStorage failed: %v", err)
	}
	blobCount, err := cas.StreamSegmentMembrane().CountLiveChunkBlobs()
	if err != nil {
		t.Fatalf("CountLiveChunkBlobs failed: %v", err)
	}
	if blobCount != 1 {
		t.Fatalf("expected exactly 1 live CAS chunk blob for identical payloads, found %d", blobCount)
	}

	// Step 4: Rehydrate bytes for both IDs through FileObjectStorage
	rehydrated1, err := fos.GetStreamSegmentChunk(objects.KindAuditEvent, "AUDIT-SEG-CHUNK-001")
	if err != nil {
		t.Fatalf("GetStreamSegmentChunk(001) failed: %v", err)
	}
	if !bytes.Equal(rehydrated1, payload) {
		t.Fatalf("rehydrated bytes for 001 do not match original payload")
	}

	rehydrated2, err := fos.GetStreamSegmentChunk(objects.KindAuditEvent, "AUDIT-SEG-CHUNK-002")
	if err != nil {
		t.Fatalf("GetStreamSegmentChunk(002) failed: %v", err)
	}
	if !bytes.Equal(rehydrated2, payload) {
		t.Fatalf("rehydrated bytes for 002 do not match original payload")
	}

	// Step 5: Fail-closed on missing ID
	_, err = fos.GetStreamSegmentChunk(objects.KindAuditEvent, "NON-EXISTENT-ID")
	if err == nil {
		t.Fatal("expected error on non-existent stream segment ID, got nil")
	}
	if !errors.Is(err, filecas.ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound, got: %v", err)
	}

	// Step 6: Fail-closed on corrupted hash blob
	blobPath := filepath.Join(cas.StreamSegmentMembrane().ChunksDir(), expectedHash+filecas.StreamSegmentChunkExtension)
	corruptedData := []byte("definitely not valid compressed stream segment bytes")
	if err := fileutil.WriteSecureFile(blobPath, corruptedData); err != nil {
		t.Fatalf("failed to corrupt blob file: %v", err)
	}
	_, err = fos.GetStreamSegmentChunk(objects.KindAuditEvent, "AUDIT-SEG-CHUNK-001")
	if err == nil {
		t.Fatal("expected error on corrupted chunk blob, got nil")
	}
}
