package callback

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type wireVectorHarness struct {
	t      *testing.T
	framer *WireBatchFramer
}

func newWireVectorHarness(t *testing.T) *wireVectorHarness {
	t.Helper()
	return &wireVectorHarness{
		t:      t,
		framer: NewWireBatchFramer(),
	}
}

func (h *wireVectorHarness) makeTestBatch(count int) []*CallbackEntry {
	h.t.Helper()
	entries := make([]*CallbackEntry, count)
	now := time.Unix(1710000000, 0).UTC()
	for i := 0; i < count; i++ {
		entries[i] = &CallbackEntry{
			Payload: map[string]any{
				"idx":  float64(i),
				"node": "vector_bench",
			},
			Timestamp: now.Add(time.Duration(i) * time.Millisecond),
			JobID:     "job-vector",
			Priority:  3,
			Seq:       int64(i + 1),
		}
	}
	return entries
}

func TestWireVector_MemoryAlignmentAndZeroCopy(t *testing.T) {
	h := newWireVectorHarness(t)
	entries := h.makeTestBatch(4)
	buf := make([]byte, 4096)

	encoded, err := h.framer.EncodeBatch(entries, buf[:0])
	require.NoError(t, err)
	assert.NotEmpty(t, encoded)

	decoded, decodeErr := h.framer.DecodeBatch(encoded)
	require.NoError(t, decodeErr)
	assert.Len(t, decoded, 4)
	assert.Equal(t, int64(1), decoded[0].Seq)
	assert.Equal(t, int64(4), decoded[3].Seq)
}

func TestWireVector_BatchVectorAllocationLimits(t *testing.T) {
	h := newWireVectorHarness(t)
	entries := h.makeTestBatch(2)
	buf := make([]byte, 2048)

	allocs := testing.AllocsPerRun(10, func() {
		_, err := h.framer.EncodeBatch(entries, buf[:0])
		if err != nil {
			t.Fail()
		}
	})
	assert.LessOrEqual(t, allocs, float64(25))
}

func makeBenchBatch(count int) ([]*CallbackEntry, []byte) {
	entries := make([]*CallbackEntry, count)
	now := time.Unix(1710000000, 0).UTC()
	for i := range entries {
		entries[i] = &CallbackEntry{
			Payload: map[string]any{
				"v": float64(i),
			},
			Timestamp: now,
			JobID:     "bench-vector",
			Priority:  1,
			Seq:       int64(i + 1),
		}
	}
	return entries, make([]byte, 8192)
}

func BenchmarkWireProtocol_VectorizedBatchEncode(b *testing.B) {
	framer := NewWireBatchFramer()
	entries, buf := makeBenchBatch(16)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		batchBytes, err := framer.EncodeBatch(entries, buf[:0])
		if err != nil || len(batchBytes) == 0 {
			b.FailNow()
		}
	}
}

func BenchmarkWireProtocol_VectorizedBatchDecode(b *testing.B) {
	framer := NewWireBatchFramer()
	entries, buf := makeBenchBatch(16)
	encoded, initErr := framer.EncodeBatch(entries, buf[:0])
	if initErr != nil {
		b.FailNow()
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		decoded, err := framer.DecodeBatch(encoded)
		if err != nil || len(decoded) == 0 {
			b.FailNow()
		}
	}
}


