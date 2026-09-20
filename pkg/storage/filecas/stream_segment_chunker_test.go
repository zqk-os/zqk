package filecas

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestIdenticalStreamSegmentChunksShareOneCASBlob verifies CRIT-1789333114473875000-490ba391:
// Identical stream-segment chunks share one CAS blob; get rehydrates original bytes.
// Covering BLI-1789333140423696000-7ac580bb and BLI-1789333142411909000-c315d401.
func TestIdenticalStreamSegmentChunksShareOneCASBlob(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "audit_event")

	payload := []byte(strings.Repeat("event_type: audit_log\nuser: agent-peer-42\naction: claim_bli\n", 50))
	expectedHash := CalculateSHA256Hash(payload)

	// Step 1: Put first stream segment payload
	hash1, err := cas.PutStreamSegmentChunk("SEG-CHUNK-001", payload)
	if err != nil {
		t.Fatalf("first PutStreamSegmentChunk failed: %v", err)
	}
	if hash1 != expectedHash {
		t.Fatalf("expected hash %s, got %s", expectedHash, hash1)
	}

	// Step 2: Put identical stream segment payload under different ID
	hash2, err := cas.PutStreamSegmentChunk("SEG-CHUNK-002", payload)
	if err != nil {
		t.Fatalf("second PutStreamSegmentChunk failed: %v", err)
	}
	if hash2 != expectedHash {
		t.Fatalf("expected hash %s, got %s", expectedHash, hash2)
	}

	// Step 3: Verify content-addressed deduplication — exactly ONE blob file exists on disk
	membrane := cas.StreamSegmentMembrane()
	blobCount, err := membrane.CountLiveChunkBlobs()
	if err != nil {
		t.Fatalf("CountLiveChunkBlobs failed: %v", err)
	}
	if blobCount != 1 {
		t.Fatalf("expected exactly 1 live CAS chunk blob for identical payloads, found %d", blobCount)
	}

	// Step 4: Verify compression — on-disk blob should be compressed and smaller than raw text
	blobPath := filepath.Join(kindDir, StreamSegmentChunkDir, expectedHash+StreamSegmentChunkExtension)
	info, err := fileutil.Stat(blobPath)
	if err != nil {
		t.Fatalf("failed to stat chunk blob at %s: %v", blobPath, err)
	}
	if info.Size() >= int64(len(payload)) {
		t.Errorf("expected compressed blob size (%d) to be smaller than original (%d)", info.Size(), len(payload))
	}

	// Step 5: Get path rehydrates original bytes for both IDs
	rehydrated1, err := cas.GetStreamSegmentChunk("SEG-CHUNK-001")
	if err != nil {
		t.Fatalf("GetStreamSegmentChunk(SEG-CHUNK-001) failed: %v", err)
	}
	if !bytes.Equal(rehydrated1, payload) {
		t.Fatalf("SEG-CHUNK-001 rehydrated bytes mismatch with original payload")
	}

	rehydrated2, err := cas.GetStreamSegmentChunk("SEG-CHUNK-002")
	if err != nil {
		t.Fatalf("GetStreamSegmentChunk(SEG-CHUNK-002) failed: %v", err)
	}
	if !bytes.Equal(rehydrated2, payload) {
		t.Fatalf("SEG-CHUNK-002 rehydrated bytes mismatch with original payload")
	}

	// Step 6: Direct hash rehydration
	directRehydrated, err := cas.RehydrateChunk(expectedHash)
	if err != nil {
		t.Fatalf("RehydrateChunk(%s) failed: %v", expectedHash, err)
	}
	if !bytes.Equal(directRehydrated, payload) {
		t.Fatalf("direct RehydrateChunk mismatch with original payload")
	}
}

// TestStreamSegmentChunker_MultiChunkDeduplication verifies that multi-chunk segments
// deduplicate shared chunks and correctly rehydrate complete original streams.
func TestStreamSegmentChunker_MultiChunkDeduplication(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	membrane := NewStreamSegmentCASMembrane(kindDir)
	// Small 1KB chunk size for testing multi-chunk partitioning
	membrane.chunker = NewStreamSegmentChunker(1024)

	chunk1Data := bytes.Repeat([]byte("1234567890abcdef"), 64)             // exactly 1024 bytes
	chunk2DataA := bytes.Repeat([]byte("stream A unique body line\n"), 10) // 270 bytes
	chunk2DataB := bytes.Repeat([]byte("stream B unique body line\n"), 10) // 270 bytes

	streamA := append(chunk1Data, chunk2DataA...)
	streamB := append(chunk1Data, chunk2DataB...)

	hashesA, err := membrane.PutStreamSegment("STREAM-SEG-A", streamA)
	if err != nil {
		t.Fatalf("PutStreamSegment(STREAM-SEG-A) failed: %v", err)
	}
	if len(hashesA) < 2 {
		t.Fatalf("expected at least 2 chunks for stream A, got %d", len(hashesA))
	}

	hashesB, err := membrane.PutStreamSegment("STREAM-SEG-B", streamB)
	if err != nil {
		t.Fatalf("PutStreamSegment(STREAM-SEG-B) failed: %v", err)
	}
	if len(hashesB) < 2 {
		t.Fatalf("expected at least 2 chunks for stream B, got %d", len(hashesB))
	}

	// First chunk is identical in both streams
	if hashesA[0] != hashesB[0] {
		t.Fatalf("expected shared first chunk hash %s == %s", hashesA[0], hashesB[0])
	}
	// Second chunk differs
	if hashesA[1] == hashesB[1] {
		t.Fatalf("expected distinct second chunk hashes")
	}

	// Total unique blobs on disk should be exactly 3 (shared chunk 1 + chunk 2A + chunk 2B)
	blobCount, err := membrane.CountLiveChunkBlobs()
	if err != nil {
		t.Fatalf("CountLiveChunkBlobs failed: %v", err)
	}
	if blobCount != 3 {
		t.Fatalf("expected exactly 3 unique chunk blobs across both streams, got %d", blobCount)
	}

	// Rehydrate both streams and verify byte-for-byte fidelity
	rehydratedA, err := membrane.GetStreamSegment("STREAM-SEG-A")
	if err != nil {
		t.Fatalf("GetStreamSegment(STREAM-SEG-A) failed: %v", err)
	}
	if !bytes.Equal(rehydratedA, streamA) {
		t.Fatalf("rehydrated STREAM-SEG-A does not match original bytes")
	}

	rehydratedB, err := membrane.GetStreamSegment("STREAM-SEG-B")
	if err != nil {
		t.Fatalf("GetStreamSegment(STREAM-SEG-B) failed: %v", err)
	}
	if !bytes.Equal(rehydratedB, streamB) {
		t.Fatalf("rehydrated STREAM-SEG-B does not match original bytes")
	}
}

// TestStreamSegmentGet_FailClosedOnMiss verifies that an unmapped or missing segment ID
// fails closed with ErrObjectNotFound and does not perform a silent kind-root scan success.
func TestStreamSegmentGet_FailClosedOnMiss(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "audit_event")

	_, err := cas.GetStreamSegmentChunk("NON-EXISTENT-SEGMENT-ID")
	if err == nil {
		t.Fatal("expected error on missing stream-segment ID, got nil")
	}
	if !errors.Is(err, ErrObjectNotFound) {
		t.Fatalf("expected ErrObjectNotFound, got: %v", err)
	}
}

// TestStreamSegmentGet_FailClosedOnHashMismatch verifies that if a chunk blob on disk is corrupted
// or tampered with, rehydration fails closed with an integrity failure.
func TestStreamSegmentGet_FailClosedOnHashMismatch(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "audit_event")

	payload := []byte("critical tamper-evident stream-segment payload\n")
	hash, err := cas.PutStreamSegmentChunk("TAMPER-TEST-SEG", payload)
	if err != nil {
		t.Fatalf("PutStreamSegmentChunk failed: %v", err)
	}

	// Corrupt the blob file on disk with invalid/tampered compressed bytes
	blobPath := filepath.Join(kindDir, StreamSegmentChunkDir, hash+StreamSegmentChunkExtension)
	tamperedBytes, err := compressPayload([]byte("malicious altered payload\n"))
	if err != nil {
		t.Fatalf("failed to compress tampered payload: %v", err)
	}
	if err := fileutil.WriteSecureFile(blobPath, tamperedBytes); err != nil {
		t.Fatalf("failed to overwrite chunk blob: %v", err)
	}

	// Rehydrate must fail closed on hash mismatch
	_, err = cas.GetStreamSegmentChunk("TAMPER-TEST-SEG")
	if err == nil {
		t.Fatal("expected error on tampered chunk blob, got nil")
	}
	if !strings.Contains(err.Error(), "integrity failure") {
		t.Fatalf("expected integrity failure error, got: %v", err)
	}
}

// TestStreamSegmentPut_EmptyContentRejection verifies that empty content is rejected.
func TestStreamSegmentPut_EmptyContentRejection(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	cas := NewContentAddressableStorage(kindDir, "audit_event")

	_, err := cas.PutStreamSegmentChunk("EMPTY-SEG", nil)
	if err == nil {
		t.Fatal("expected error on empty content, got nil")
	}

	_, err = cas.PutStreamSegmentChunk("", []byte("some data"))
	if err == nil {
		t.Fatal("expected error on empty objectID, got nil")
	}
}

// TestStreamSegmentPut_FailClosedOnMappingFailure verifies fail-closed put when mapping cannot be recorded.
func TestStreamSegmentPut_FailClosedOnMappingFailure(t *testing.T) {
	t.Parallel()
	kindDir := t.TempDir()
	membrane := NewStreamSegmentCASMembrane(kindDir)

	// Create mappingPath as a directory so writing the mapping file fails
	if err := fileutil.MkdirAll(membrane.mappingPath, paths.DirPerm750); err != nil {
		t.Fatal(err)
	}

	_, err := membrane.PutStreamSegmentChunk("FAIL-MAPPING-ID", []byte("some segment content\n"))
	if err == nil {
		t.Fatal("expected error when mapping file cannot be written, got nil")
	}
	if !strings.Contains(err.Error(), "fail-closed") {
		t.Fatalf("expected fail-closed error message, got: %v", err)
	}
}
