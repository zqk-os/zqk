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

func (h *wireBoundaryHarness) assertDecodeError(data []byte, expectedSubstr string) {
	h.t.Helper()
	decoded, bytesConsumed, err := h.codec.DecodeEntry(data)
	assert.Error(h.t, err)
	assert.Nil(h.t, decoded)
	assert.Equal(h.t, 0, bytesConsumed)
	assert.Contains(h.t, err.Error(), expectedSubstr)
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
	h.assertDecodeError(corrupted, "crc32 checksum mismatch")
}

func TestWireBoundary_TruncatedPayloadLength(t *testing.T) {
	h := newWireBoundaryHarness(t)
	truncated := h.encodeSample(99)[:WireHeaderSize+2]
	h.assertDecodeError(truncated, "incomplete frame data")
}
