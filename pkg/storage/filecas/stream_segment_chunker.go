package filecas

import (
	"bytes"
	"compress/zlib"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	// DefaultStreamSegmentChunkSize is the default chunk partition size (64KB).
	DefaultStreamSegmentChunkSize = 64 * 1024

	// StreamSegmentChunkDir is the subdirectory under kindDir where chunk blobs reside.
	StreamSegmentChunkDir = "chunks"

	// StreamSegmentMappingFile is the JSON index of object/segment ID to chunk hashes.
	StreamSegmentMappingFile = "stream_segment_mappings.json"

	// StreamSegmentChunkExtension is the file extension for content-addressed chunk blobs.
	StreamSegmentChunkExtension = ".blob"
)

// StreamSegmentChunk represents an individual chunk of a stream segment.
type StreamSegmentChunk struct {
	Hash           string `json:"hash"`
	OriginalSize   int    `json:"original_size"`
	CompressedSize int    `json:"compressed_size"`
	Data           []byte `json:"-"`
}

// StreamSegmentChunker partitions payloads into compressed, content-addressed chunks.
type StreamSegmentChunker struct {
	chunkSize int
}

// NewStreamSegmentChunker creates a new chunker with the specified chunk size.
func NewStreamSegmentChunker(chunkSize int) *StreamSegmentChunker {
	if chunkSize <= 0 {
		chunkSize = DefaultStreamSegmentChunkSize
	}
	return &StreamSegmentChunker{chunkSize: chunkSize}
}

// Chunk splits a stream-segment payload into compressed, content-addressed chunks.
func (c *StreamSegmentChunker) Chunk(payload []byte) ([]*StreamSegmentChunk, error) {
	if len(payload) == 0 {
		return nil, errfmt.Errorf("cannot chunk empty stream-segment payload")
	}

	var chunks []*StreamSegmentChunk
	for offset := 0; offset < len(payload); offset += c.chunkSize {
		end := offset + c.chunkSize
		if end > len(payload) {
			end = len(payload)
		}
		part := payload[offset:end]
		hash := CalculateSHA256Hash(part)

		compressed, err := compressPayload(part)
		if err != nil {
			return nil, errfmt.Newf("failed to compress stream-segment chunk %s", hash).Wrap(err)
		}

		chunks = append(chunks, &StreamSegmentChunk{
			Hash:           hash,
			OriginalSize:   len(part),
			CompressedSize: len(compressed),
			Data:           compressed,
		})
	}
	return chunks, nil
}

// StreamSegmentMapping records the association between an object/segment ID and its chunk hashes.
type StreamSegmentMapping struct {
	ObjectID          string    `json:"object_id"`
	ChunkHashes       []string  `json:"chunk_hashes"`
	TotalOriginalSize int       `json:"total_original_size"`
	CreatedAt         time.Time `json:"created_at"`
}

// StreamSegmentCASMembrane orchestrates content-addressed chunk persistence, deduplication,
// and fail-closed rehydration for stream segments.
type StreamSegmentCASMembrane struct {
	kindDir     string
	chunksDir   string
	mappingPath string
	chunker     *StreamSegmentChunker

	mu       sync.RWMutex
	mappings map[string]*StreamSegmentMapping
}

// NewStreamSegmentCASMembrane initializes a stream segment CAS membrane for a kind directory.
func NewStreamSegmentCASMembrane(kindDir string) *StreamSegmentCASMembrane {
	m := &StreamSegmentCASMembrane{
		kindDir:     kindDir,
		chunksDir:   filepath.Join(kindDir, StreamSegmentChunkDir),
		mappingPath: filepath.Join(kindDir, StreamSegmentMappingFile),
		chunker:     NewStreamSegmentChunker(DefaultStreamSegmentChunkSize),
		mappings:    make(map[string]*StreamSegmentMapping),
	}
	_ = m.loadMappings()
	return m
}

// loadMappings loads persisted mappings from disk into memory.
func (m *StreamSegmentCASMembrane) loadMappings() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := fileutil.ReadFile(m.mappingPath)
	if err != nil {
		if os.IsNotExist(err) || fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}

	var diskMap map[string]*StreamSegmentMapping
	if err := json.Unmarshal(data, &diskMap); err != nil {
		return errfmt.Newf("corrupted stream segment mappings file").Wrap(err)
	}
	for k, v := range diskMap {
		m.mappings[k] = v
	}
	return nil
}

// persistMappings writes current in-memory mappings to disk atomically with sync.
// Fail-closed: returns error if disk write fails.
func (m *StreamSegmentCASMembrane) persistMappingsLocked() error {
	if err := fileutil.MkdirAll(m.kindDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to ensure kind directory for stream membrane").Wrap(err)
	}

	data, err := json.MarshalIndent(m.mappings, "", "  ")
	if err != nil {
		return errfmt.Newf("failed to marshal stream segment mappings").Wrap(err)
	}

	return fileutil.WriteSecureFile(m.mappingPath, data)
}

// PutStreamSegment chunks, compresses, and stores a stream-segment payload under content-addressed
// chunk identities. Identical chunks share one blob on disk.
// Fails closed if the membrane cannot record the mapping.
func (m *StreamSegmentCASMembrane) PutStreamSegment(objectID string, payload []byte) ([]string, error) {
	if strings.TrimSpace(objectID) == "" {
		return nil, errfmt.Errorf("empty object ID for stream-segment put")
	}
	if len(payload) == 0 {
		return nil, errfmt.Errorf(ConstMiscCannotCreateObjectWithEmptyContent)
	}

	chunks, err := m.chunker.Chunk(payload)
	if err != nil {
		return nil, err
	}

	if err := fileutil.MkdirAll(m.chunksDir, paths.DirPerm755); err != nil {
		return nil, errfmt.Newf(ErrMsgCreateDir).Wrap(err)
	}

	var chunkHashes []string
	for _, chunk := range chunks {
		chunkHashes = append(chunkHashes, chunk.Hash)
		blobPath := filepath.Join(m.chunksDir, chunk.Hash+StreamSegmentChunkExtension)

		// Deduplication check: if blob already exists on disk, reuse it without rewriting
		if info, err := fileutil.Stat(blobPath); err == nil && info.Size() > 0 {
			continue
		}

		// Write new chunk blob atomically
		if err := fileutil.WriteSecureFile(blobPath, chunk.Data); err != nil {
			return nil, errfmt.Newf("failed to write stream-segment chunk blob %s", chunk.Hash).Wrap(err)
		}
	}

	// Record mapping under lock and persist to disk (fail-closed)
	m.mu.Lock()
	defer m.mu.Unlock()

	mapping := &StreamSegmentMapping{
		ObjectID:          objectID,
		ChunkHashes:       chunkHashes,
		TotalOriginalSize: len(payload),
		CreatedAt:         time.Now().UTC(),
	}
	m.mappings[objectID] = mapping

	if err := m.persistMappingsLocked(); err != nil {
		delete(m.mappings, objectID)
		return nil, errfmt.Newf("stream-segment membrane failed to record mapping (fail-closed)").Wrap(err)
	}

	return chunkHashes, nil
}

// PutStreamSegmentChunk is a single-chunk helper for stream-segment writes.
// Returns the content-addressed blob hash.
func (m *StreamSegmentCASMembrane) PutStreamSegmentChunk(objectID string, payload []byte) (string, error) {
	hashes, err := m.PutStreamSegment(objectID, payload)
	if err != nil {
		return "", err
	}
	if len(hashes) == 0 {
		return "", errfmt.Errorf("no chunks generated for stream segment")
	}
	return hashes[0], nil
}

// GetStreamSegment rehydrates the original stream-segment bytes for objectID from deduped,
// compressed CAS blobs.
// Miss or hash mismatch is fail-closed, never a silent kind-root scan success.
func (m *StreamSegmentCASMembrane) GetStreamSegment(objectID string) ([]byte, error) {
	if strings.TrimSpace(objectID) == "" {
		return nil, errfmt.Errorf("empty object ID for stream-segment get")
	}

	m.mu.RLock()
	mapping, ok := m.mappings[objectID]
	m.mu.RUnlock()

	if !ok || mapping == nil {
		// Try reloading mappings from disk once in case another process updated it
		if err := m.loadMappings(); err == nil {
			m.mu.RLock()
			mapping, ok = m.mappings[objectID]
			m.mu.RUnlock()
		}
	}

	if !ok || mapping == nil || len(mapping.ChunkHashes) == 0 {
		// Fail closed on miss: do NOT fall back to kind-root scan
		return nil, errfmt.Errorf("stream-segment chunk mapping for %s: %w", objectID, ErrObjectNotFound)
	}

	var rehydrated bytes.Buffer
	for _, hash := range mapping.ChunkHashes {
		chunkBytes, err := m.RehydrateChunk(hash)
		if err != nil {
			return nil, errfmt.Newf("failed to rehydrate stream-segment chunk %s for %s", hash, objectID).Wrap(err)
		}
		rehydrated.Write(chunkBytes)
	}

	return rehydrated.Bytes(), nil
}

// GetStreamSegmentChunk rehydrates a single chunk stream segment for objectID.
func (m *StreamSegmentCASMembrane) GetStreamSegmentChunk(objectID string) ([]byte, error) {
	return m.GetStreamSegment(objectID)
}

// RehydrateChunk reads and decompresses a single chunk blob by hash, validating its SHA256 integrity.
// Miss or hash mismatch is fail-closed.
func (m *StreamSegmentCASMembrane) RehydrateChunk(hash string) ([]byte, error) {
	if strings.TrimSpace(hash) == "" {
		return nil, errfmt.Errorf("empty hash for chunk rehydration")
	}

	blobPath := filepath.Join(m.chunksDir, hash+StreamSegmentChunkExtension)
	compressedData, err := fileutil.ReadFile(blobPath)
	if err != nil {
		if os.IsNotExist(err) || fileutil.IsNotExist(err) {
			return nil, errfmt.Errorf("chunk blob %s: %w", hash, ErrObjectNotFound)
		}
		return nil, errfmt.Newf("failed to read chunk blob %s", hash).Wrap(err)
	}

	decompressed, err := decompressPayload(compressedData)
	if err != nil {
		return nil, errfmt.Newf("failed to decompress chunk blob %s", hash).Wrap(err)
	}

	actualHash := CalculateSHA256Hash(decompressed)
	if actualHash != hash {
		return nil, errfmt.Errorf("stream-segment chunk integrity failure: expected %s, got %s", hash, actualHash)
	}

	return decompressed, nil
}

// HasChunk reports whether a chunk blob exists in the membrane's chunks directory.
func (m *StreamSegmentCASMembrane) HasChunk(hash string) bool {
	blobPath := filepath.Join(m.chunksDir, hash+StreamSegmentChunkExtension)
	info, err := fileutil.Stat(blobPath)
	return err == nil && info.Size() > 0
}

// ChunksDir returns the directory where chunk blobs are stored.
func (m *StreamSegmentCASMembrane) ChunksDir() string {
	return m.chunksDir
}

// CountLiveChunkBlobs returns the count of .blob files in the chunks directory.
func (m *StreamSegmentCASMembrane) CountLiveChunkBlobs() (int, error) {
	entries, err := fileutil.ReadDir(m.chunksDir)
	if err != nil {
		if os.IsNotExist(err) || fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), StreamSegmentChunkExtension) {
			count++
		}
	}
	return count, nil
}

// compressPayload compresses data using zlib.
func compressPayload(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		_ = w.Close()
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// decompressPayload decompresses zlib-compressed data.
func decompressPayload(compressed []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
