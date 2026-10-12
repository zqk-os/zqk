package callback

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

type wireCodecHarness struct {
	codec  *WireProtocolCodec
	framer *WireBatchFramer
}

func newWireCodecHarness(t *testing.T) *wireCodecHarness {
	t.Helper()
	return &wireCodecHarness{
		codec:  NewWireProtocolCodec(),
		framer: NewWireBatchFramer(),
	}
}

func (h *wireCodecHarness) sampleEntry(seq int64, jobID string) *CallbackEntry {
	return &CallbackEntry{
		Payload: map[string]any{
			"status": "completed",
			"code":   float64(0),
			"worker": "test-node",
		},
		Timestamp: time.Unix(1700000000+seq, 0).UTC(),
		JobID:     jobID,
		Priority:  10,
		Seq:       seq,
	}
}

func (h *wireCodecHarness) assertEntryEqual(t *testing.T, expected, actual *CallbackEntry) {
	t.Helper()
	assert.Equal(t, expected.Seq, actual.Seq)
	assert.Equal(t, expected.JobID, actual.JobID)
	assert.Equal(t, expected.Priority, actual.Priority)
	assert.Equal(t, expected.Timestamp.Unix(), actual.Timestamp.Unix())
	assert.Equal(t, expected.Payload["status"], actual.Payload["status"])
	assert.Equal(t, expected.Payload["code"], actual.Payload["code"])
}

func (h *wireCodecHarness) mustEncodeEntry(t *testing.T, entry *CallbackEntry, bufSize int) []byte {
	t.Helper()
	buf := make([]byte, bufSize)
	encoded, err := h.codec.EncodeEntry(entry, buf[:0])
	require.NoError(t, err)
	return encoded
}

func TestWireProtocol_EncodeDecode_Single(t *testing.T) {
	h := newWireCodecHarness(t)
	entry := h.sampleEntry(42, "job-42")

	encoded := h.mustEncodeEntry(t, entry, 1024)
	assert.Greater(t, len(encoded), WireHeaderSize)

	decoded, consumed, err := h.codec.DecodeEntry(encoded)
	require.NoError(t, err)
	assert.Equal(t, len(encoded), consumed)
	h.assertEntryEqual(t, entry, decoded)
}

func TestWireProtocol_BatchFraming_Fidelity(t *testing.T) {
	h := newWireCodecHarness(t)
	entries := []*CallbackEntry{
		h.sampleEntry(1, "job-1"),
		h.sampleEntry(2, "job-2"),
		h.sampleEntry(3, "job-3"),
	}

	batchBuf := make([]byte, 4096)
	encodedBatch, err := h.framer.EncodeBatch(entries, batchBuf[:0])
	require.NoError(t, err)
	assert.NotEmpty(t, encodedBatch)

	decodedEntries, err := h.framer.DecodeBatch(encodedBatch)
	require.NoError(t, err)
	require.Len(t, decodedEntries, len(entries))

	for i := range entries {
		h.assertEntryEqual(t, entries[i], decodedEntries[i])
	}
}

func TestWireProtocol_ZeroAllocations_EncodePath(t *testing.T) {
	h := newWireCodecHarness(t)
	entry := h.sampleEntry(100, "job-zero-alloc")

	buf := make([]byte, 2048)
	encodedInitial, err := h.codec.EncodeEntry(entry, buf[:0])
	require.NoError(t, err)
	require.NotEmpty(t, encodedInitial)

	allocs := testing.AllocsPerRun(1000, func() {
		b, err := h.codec.EncodeEntry(entry, buf[:0])
		if err != nil {
			panic(err)
		}
		if len(b) == 0 {
			panic("empty encoded buffer")
		}
	})

	assert.LessOrEqual(t, allocs, float64(15), "encode path must minimize allocations with pre-sized buffer")
}

func TestWireProtocol_DeterministicChecksum_Validation(t *testing.T) {
	h := newWireCodecHarness(t)
	entry := h.sampleEntry(77, "job-crc-check")

	encoded := h.mustEncodeEntry(t, entry, 1024)

	// Verify tampering triggers checksum failure
	tampered := bytes.Clone(encoded)
	tampered[len(tampered)-1] ^= 0xFF

	decoded, consumed, err := h.codec.DecodeEntry(tampered)
	assert.Error(t, err)
	assert.Nil(t, decoded)
	assert.Equal(t, 0, consumed)
	assert.Contains(t, err.Error(), "crc32 checksum mismatch")
}

func TestWireProtocol_HeaderValidation_Errors(t *testing.T) {
	h := newWireCodecHarness(t)

	// Insufficient buffer size
	shortData := make([]byte, 10)
	shortDec, shortConsumed, shortErr := h.codec.DecodeEntry(shortData)
	assert.Error(t, shortErr)
	assert.Nil(t, shortDec)
	assert.Equal(t, 0, shortConsumed)
	assert.Contains(t, shortErr.Error(), "insufficient buffer for header")

	// Invalid magic header
	invalidMagic := make([]byte, WireHeaderSize)
	binary.BigEndian.PutUint32(invalidMagic[0:4], 0xDEADBEEF)
	invalidMagic[4] = WireProtocolVersion
	magicDec, magicConsumed, magicErr := h.codec.DecodeEntry(invalidMagic)
	assert.Error(t, magicErr)
	assert.Nil(t, magicDec)
	assert.Equal(t, 0, magicConsumed)
	assert.Contains(t, magicErr.Error(), "invalid wire magic header")

	// Unsupported protocol version
	invalidVer := make([]byte, WireHeaderSize)
	binary.BigEndian.PutUint32(invalidVer[0:4], WireMagicHeader)
	invalidVer[4] = 99
	verDec, verConsumed, verErr := h.codec.DecodeEntry(invalidVer)
	assert.Error(t, verErr)
	assert.Nil(t, verDec)
	assert.Equal(t, 0, verConsumed)
	assert.Contains(t, verErr.Error(), "unsupported wire protocol version")
}

func TestWireProtocol_ConcurrentEncodeDecode_Race(t *testing.T) {
	h := newWireCodecHarness(t)
	const workerCount = 10
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(workerCount)

	for i := 0; i < workerCount; i++ {
		workerID := i
		goroutinelabels.NewGoroutine(fmt.Sprintf("wire_worker_%d", workerID), "concurrent encode decode worker").StartSimple(func() {
			defer wg.Done()
			buf := make([]byte, 1024)
			for j := 0; j < iterations; j++ {
				seq := int64(workerID*1000 + j)
				entry := h.sampleEntry(seq, fmt.Sprintf("job-worker-%d-%d", workerID, j))

				encoded, encErr := h.codec.EncodeEntry(entry, buf[:0])
				if encErr != nil {
					t.Errorf("worker %d encode failed: %v", workerID, encErr)
					return
				}

				decoded, consumed, decErr := h.codec.DecodeEntry(encoded)
				if decErr != nil {
					t.Errorf("worker %d decode failed: %v", workerID, decErr)
					return
				}

				if consumed != len(encoded) || decoded.Seq != seq {
					t.Errorf("worker %d data mismatch: got seq %d, want %d", workerID, decoded.Seq, seq)
					return
				}
			}
		})
	}

	wg.Wait()
}

func TestWireProtocol_PayloadSizeLimit_Validation(t *testing.T) {
	h := newWireCodecHarness(t)

	// 1. Verify decode rejects payloadLen exceeding WireMaxPayloadSize
	hdr := make([]byte, WireHeaderSize)
	binary.BigEndian.PutUint32(hdr[0:4], WireMagicHeader)
	hdr[4] = WireProtocolVersion
	binary.BigEndian.PutUint32(hdr[24:28], uint32(WireMaxPayloadSize+1))

	dec, consumed, err := h.codec.DecodeEntry(hdr)
	assert.Error(t, err)
	assert.Nil(t, dec)
	assert.Equal(t, 0, consumed)
	assert.Contains(t, err.Error(), ErrMsgPayloadLengthTooLarge)

	// 2. Verify decode rejects maximum uint32 payloadLen that would overflow arithmetic
	binary.BigEndian.PutUint32(hdr[24:28], 0xFFFFFFFF)
	dec, consumed, err = h.codec.DecodeEntry(hdr)
	assert.Error(t, err)
	assert.Nil(t, dec)
	assert.Equal(t, 0, consumed)
	assert.Contains(t, err.Error(), ErrMsgPayloadLengthTooLarge)

	// 3. Verify encode rejects payload exceeding WireMaxPayloadSize
	largePayload := make([]byte, WireMaxPayloadSize+1)
	largeEntry := &CallbackEntry{
		Payload: map[string]any{
			"data": string(largePayload),
		},
		JobID: "job-oversize",
	}

	encoded, encErr := h.codec.EncodeEntry(largeEntry, nil)
	assert.Error(t, encErr)
	assert.Nil(t, encoded)
	assert.Contains(t, encErr.Error(), ErrMsgPayloadTooLarge)
}
