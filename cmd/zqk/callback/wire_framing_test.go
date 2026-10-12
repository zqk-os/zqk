package callback

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	testWorkerLabelFormat = `wire_worker_%d`
	testWorkerTaskPurpose = `concurrent wire framing worker`
	testAlphaPayload      = `event_alpha_payload_content`
	testBetaPayload       = `event_beta_payload_content`
	testGammaPayload      = `event_gamma_payload_content`
	testDeltaPayload      = `event_delta_payload_content`
	testWorkerPayloadFmt  = `worker-%d-cycle-%d`
	testZeroAllocMsg      = `pooled path must have zero heap allocations`
)

type wireHarness struct {
	t *testing.T
}

func newWireHarness(t *testing.T) *wireHarness {
	t.Helper()
	return &wireHarness{t: t}
}

func (h *wireHarness) encodeValid(header FrameHeader, payload []byte, dst []byte) int {
	h.t.Helper()
	written, err := EncodeFrame(header, payload, dst)
	require.NoError(h.t, err)
	require.Positive(h.t, written)
	return written
}

func (h *wireHarness) decodeValid(src []byte) (Frame, int) {
	h.t.Helper()
	frame, consumed, err := DecodeFrame(src)
	require.NoError(h.t, err)
	return frame, consumed
}

func (h *wireHarness) verifyEquality(expectedHeader FrameHeader, expectedPayload []byte, actual Frame) {
	h.t.Helper()
	assert.Equal(h.t, expectedHeader.Magic, actual.Header.Magic)
	assert.Equal(h.t, expectedHeader.Flags, actual.Header.Flags)
	assert.Equal(h.t, expectedHeader.SeqID, actual.Header.SeqID)
	assert.Equal(h.t, uint32(len(expectedPayload)), actual.Header.Length)
	assert.True(h.t, bytes.Equal(expectedPayload, actual.Payload))
}

func TestWire_SingleFrameRoundTrip(t *testing.T) {
	h := newWireHarness(t)
	cases := []struct {
		name    string
		flags   uint16
		seqID   uint64
		payload []byte
	}{
		{name: "empty_payload", flags: FlagNone, seqID: 100, payload: nil},
		{name: "short_payload", flags: FlagHeartbeat, seqID: 101, payload: []byte("ping")},
		{name: `medium_payload`, flags: FlagCompressed, seqID: 102, payload: []byte(testAlphaPayload)},
		{name: "large_payload", flags: FlagNone, seqID: 103, payload: bytes.Repeat([]byte{0x42}, 4096)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hdr := NewFrameHeader(tc.seqID, tc.flags, uint32(len(tc.payload)))
			buf := make([]byte, HeaderSize+len(tc.payload)+16)
			written := h.encodeValid(hdr, tc.payload, buf)

			frame, consumed := h.decodeValid(buf[:written])
			assert.Equal(t, written, consumed)
			assert.Equal(t, hdr.TotalSize(tc.payload), frame.TotalSize())
			h.verifyEquality(hdr, tc.payload, frame)

			if len(tc.payload) > 0 {
				assert.Equal(t, &buf[HeaderSize], &frame.Payload[0])
			}
		})
	}
}

func (h FrameHeader) TotalSize(payload []byte) int {
	return HeaderSize + len(payload)
}

func TestWire_VectorizedBatchRoundTrip(t *testing.T) {
	payloads := [][]byte{
		[]byte(testAlphaPayload),
		[]byte(testBetaPayload),
		[]byte(""),
		[]byte(testGammaPayload),
		[]byte(testDeltaPayload),
	}

	baseSeq := uint64(500)
	encoded, err := PackBatchPayloads(baseSeq, FlagNone, payloads, nil)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	frames, err := UnpackBatch(encoded)
	require.NoError(t, err)
	require.Len(t, frames, len(payloads))

	for idx, frame := range frames {
		expectedSeq := baseSeq + uint64(idx)
		assert.Equal(t, expectedSeq, frame.Header.SeqID)
		assert.Equal(t, FlagBatch, frame.Header.Flags&FlagBatch)
		assert.True(t, bytes.Equal(payloads[idx], frame.Payload))
	}

	reuseSlice := make([]Frame, 0, len(payloads))
	reused, err := UnpackBatchInto(encoded, reuseSlice)
	require.NoError(t, err)
	assert.Len(t, reused, len(payloads))
}

func TestWire_CorruptedHeaderAndBoundaryErrors(t *testing.T) {
	h := newWireHarness(t)
	payload := []byte(testAlphaPayload)
	hdr := NewFrameHeader(2001, FlagNone, uint32(len(payload)))
	validBuf := make([]byte, HeaderSize+len(payload))
	written := h.encodeValid(hdr, payload, validBuf)
	require.Equal(t, len(validBuf), written)

	t.Run(`header_truncated`, func(t *testing.T) {
		_, _, err := DecodeFrame(validBuf[:HeaderSize-1])
		assert.ErrorIs(t, err, ErrHeaderTruncated)
	})

	t.Run("invalid_magic", func(t *testing.T) {
		badMagic := make([]byte, len(validBuf))
		copy(badMagic, validBuf)
		badMagic[0] = 'X'
		_, _, err := DecodeFrame(badMagic)
		assert.ErrorIs(t, err, ErrInvalidMagic)
	})

	t.Run(`checksum_mismatch`, func(t *testing.T) {
		corrupted := make([]byte, len(validBuf))
		copy(corrupted, validBuf)
		corrupted[4] ^= 0xFF
		_, _, err := DecodeFrame(corrupted)
		assert.ErrorIs(t, err, ErrChecksumMismatch)
	})

	t.Run(`payload_truncated`, func(t *testing.T) {
		_, _, err := DecodeFrame(validBuf[:len(validBuf)-2])
		assert.ErrorIs(t, err, ErrPayloadTruncated)
	})

	t.Run(`buffer_too_small_encode`, func(t *testing.T) {
		smallDst := make([]byte, HeaderSize-1)
		_, err := EncodeFrame(hdr, payload, smallDst)
		assert.ErrorIs(t, err, ErrBufferTooSmall)
	})

	t.Run(`oversized_payload`, func(t *testing.T) {
		oversized := make([]byte, HeaderSize)
		copy(oversized, validBuf[:HeaderSize])
		binary.BigEndian.PutUint32(oversized[14:18], MaxFramePayloadSize+1)
		newCS := CalculateHeaderChecksum(MagicWireProtocol, hdr.Flags, hdr.SeqID, MaxFramePayloadSize+1)
		binary.BigEndian.PutUint32(oversized[HeaderChecksumFieldOffset:HeaderSize], newCS)
		_, _, err := DecodeFrame(oversized)
		assert.ErrorIs(t, err, ErrFrameTooLarge)
	})

	t.Run(`empty_batch_rejection`, func(t *testing.T) {
		_, err := PackBatch(nil, nil)
		assert.ErrorIs(t, err, ErrEmptyBatch)

		_, err = UnpackBatch(nil)
		assert.ErrorIs(t, err, ErrEmptyBatch)
	})
}

func TestWire_ConcurrentEncodingDecoding(t *testing.T) {
	const workerCount = 8
	const iterations = 50

	var wg sync.WaitGroup
	ctx := context.Background()

	for workerID := 0; workerID < workerCount; workerID++ {
		wg.Add(1)
		currentID := workerID
		workerName := fmt.Sprintf(testWorkerLabelFormat, currentID)

		goroutinelabels.NewGoroutine(workerName, testWorkerTaskPurpose).WithLifecycleContext(ctx).StartSimple(func() {
			defer wg.Done()
			pool := NewWireBufferPool(1024)

			for iter := 0; iter < iterations; iter++ {
				pb := pool.Acquire()
				seq := uint64(currentID*1000 + iter)
				payloadStr := fmt.Sprintf(testWorkerPayloadFmt, currentID, iter)
				payload := []byte(payloadStr)

				h := NewFrameHeader(seq, FlagHeartbeat, uint32(len(payload)))
				bytesWritten, encErr := EncodeFrame(h, payload, pb.Buf)
				assert.NoError(t, encErr)

				frame, consumed, decErr := DecodeFrame(pb.Buf[:bytesWritten])
				assert.NoError(t, decErr)
				assert.Equal(t, bytesWritten, consumed)
				assert.Equal(t, seq, frame.Header.SeqID)
				assert.Equal(t, payload, frame.Payload)

				pool.Release(pb)
			}
		})
	}

	wg.Wait()
}

func TestWire_ZeroAllocations(t *testing.T) {
	pool := NewWireBufferPool(2048)
	payload := []byte(testBetaPayload)
	header := NewFrameHeader(42, FlagNone, uint32(len(payload)))

	// Warm pool once
	warmBuf := pool.Acquire()
	pool.Release(warmBuf)

	allocations := testing.AllocsPerRun(100, func() {
		pb := pool.Acquire()
		written, encErr := EncodeFrame(header, payload, pb.Buf)
		if encErr != nil {
			t.Fatalf("encode: %v", encErr)
		}
		frame, consumed, decErr := DecodeFrame(pb.Buf[:written])
		if decErr != nil || consumed != written || len(frame.Payload) != len(payload) {
			t.Fatalf("decode: %v", decErr)
		}
		pool.Release(pb)
	})

	assert.Equal(t, float64(0), allocations, testZeroAllocMsg)
}

func TestWire_EncodeFramePooled(t *testing.T) {
	payload := []byte(testGammaPayload)
	hdr := NewFrameHeader(8888, FlagCompressed, uint32(len(payload)))

	encoded, release, err := EncodeFramePooled(hdr, payload)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)
	defer release()

	frame, consumed, err := DecodeFrame(encoded)
	require.NoError(t, err)
	assert.Equal(t, len(encoded), consumed)
	assert.Equal(t, hdr.SeqID, frame.Header.SeqID)
	assert.Equal(t, payload, frame.Payload)
}

func BenchmarkWire_FrameRoundTrip(b *testing.B) {
	pool := NewWireBufferPool(1024)
	payload := []byte(testDeltaPayload)
	hdr := NewFrameHeader(1, FlagNone, uint32(len(payload)))

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pb := pool.Acquire()
		n, err := EncodeFrame(hdr, payload, pb.Buf)
		if err != nil {
			b.Fatal(err)
		}
		_, _, err = DecodeFrame(pb.Buf[:n])
		if err != nil {
			b.Fatal(err)
		}
		pool.Release(pb)
	}
}
