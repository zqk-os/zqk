package callback

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wireBoundaryHarness struct {
	t     *testing.T
	codec *WireProtocolCodec
}

func newWireBoundaryHarness(t *testing.T) *wireBoundaryHarness {
	t.Helper()
	return &wireBoundaryHarness{
		t:     t,
		codec: NewWireProtocolCodec(),
	}
}

func (h *wireBoundaryHarness) createSampleEntry(seq int64) *CallbackEntry {
	h.t.Helper()
	return &CallbackEntry{
		Payload: map[string]any{
			"status": "boundary_test",
			"code":   float64(seq),
		},
		Timestamp: time.Unix(1710000000+seq, 0).UTC(),
		JobID:     "job-boundary",
		Priority:  5,
		Seq:       seq,
	}
}

func (h *wireBoundaryHarness) encodeSample(seq int64) []byte {
	h.t.Helper()
	entry := h.createSampleEntry(seq)
	buf := make([]byte, 512)
	encoded, err := h.codec.EncodeEntry(entry, buf[:0])
	require.NoError(h.t, err)
	return encoded
}

func TestWireBoundary_NilEntryRejection(t *testing.T) {
	h := newWireBoundaryHarness(t)
	buf := make([]byte, 128)

	encoded, err := h.codec.EncodeEntry(nil, buf[:0])
	assert.Error(t, err)
	assert.Nil(t, encoded)
	assert.Contains(t, err.Error(), "entry cannot be nil")
}

func TestWireBoundary_CorruptedPayloadDetection(t *testing.T) {
	h := newWireBoundaryHarness(t)
	corrupted := bytes.Clone(h.encodeSample(88))
	corrupted[len(corrupted)/2] ^= 0xAA

	decoded, bytesConsumed, err := h.codec.DecodeEntry(corrupted)
	assert.Error(t, err)
	assert.Nil(t, decoded)
	assert.Equal(t, 0, bytesConsumed)
	assert.Contains(t, err.Error(), "crc32 checksum mismatch")
}

func TestWireBoundary_TruncatedPayloadLength(t *testing.T) {
	h := newWireBoundaryHarness(t)
	truncated := h.encodeSample(99)[:WireHeaderSize+2]

	decoded, bytesConsumed, err := h.codec.DecodeEntry(truncated)
	assert.Error(t, err)
	assert.Nil(t, decoded)
	assert.Equal(t, 0, bytesConsumed)
	assert.Contains(t, err.Error(), "incomplete frame data")
}
