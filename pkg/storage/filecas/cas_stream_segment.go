package filecas

// StreamSegmentMembrane returns a StreamSegmentCASMembrane bound to this CAS kind directory.
func (cas *ContentAddressableStorage) StreamSegmentMembrane() *StreamSegmentCASMembrane {
	return NewStreamSegmentCASMembrane(cas.kindDir)
}

// PutStreamSegmentChunk chunks, compresses, and persists a stream-segment payload under content-addressed
// identity. If an identical chunk payload was already stored, the existing blob is reused (deduped).
// Records the mapping objectID -> chunkHash in the membrane. Fails closed if mapping fails.
func (cas *ContentAddressableStorage) PutStreamSegmentChunk(objectID string, data []byte) (string, error) {
	return cas.StreamSegmentMembrane().PutStreamSegmentChunk(objectID, data)
}

// PutStreamSegment partitions, compresses, and persists a multi-chunk stream segment under content-addressed
// identity. Duplicate chunks across segments share a single CAS blob.
func (cas *ContentAddressableStorage) PutStreamSegment(objectID string, data []byte) ([]string, error) {
	return cas.StreamSegmentMembrane().PutStreamSegment(objectID, data)
}

// GetStreamSegmentChunk rehydrates the original bytes for objectID from deduped/compressed CAS blobs.
// Miss or hash mismatch is fail-closed, not a silent kind-root scan success.
func (cas *ContentAddressableStorage) GetStreamSegmentChunk(objectID string) ([]byte, error) {
	return cas.StreamSegmentMembrane().GetStreamSegmentChunk(objectID)
}

// RehydrateChunk reads and decompresses the chunk blob for hash, verifying content integrity.
// Fails closed on missing blob or hash mismatch.
func (cas *ContentAddressableStorage) RehydrateChunk(hash string) ([]byte, error) {
	return cas.StreamSegmentMembrane().RehydrateChunk(hash)
}
