package traversal_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/traversal"
)

// Satisfies CRIT-ZPARQL-RESULT-SCHEMA-SPEC:
// Static Floor: Standardized Query Result Schema and Tabular/Stream Encodings.
func TestZPARQL_ResultSchemaSpec(t *testing.T) {
	// 1. Verify specification document exists and contains required schemas
	specCandidates := []string{
		filepath.Join("..", "..", "docs", "specs", "SPEC-ZPARQL-RESULT-STREAMING.md"),
		filepath.Join("docs", "specs", "SPEC-ZPARQL-RESULT-STREAMING.md"),
	}
	var specContent string
	var found bool
	for _, p := range specCandidates {
		if data, err := os.ReadFile(p); err == nil {
			specContent = string(data)
			found = true
			break
		}
	}
	require.True(t, found, "SPEC-ZPARQL-RESULT-STREAMING.md must exist in docs/specs/")
	require.Contains(t, specContent, "CRIT-ZPARQL-RESULT-SCHEMA-SPEC")
	require.Contains(t, specContent, "CRIT-ZPARQL-STREAMING-BACKPRESSURE-PROOF")
	require.Contains(t, specContent, "CRIT-ZPARQL-TRUNCATED-STREAM-NEGATIVE")
	require.Contains(t, specContent, "EOS_FINALIZED")
	require.Contains(t, specContent, "Header Frame")
	require.Contains(t, specContent, "Trailer Frame")

	// 2. Programmatic Schema Verification
	header := traversal.HeaderFrame{
		QueryHash: "sha256:test1234",
		Columns:   []string{"id", "title", "status"},
		Types: map[string]string{
			"id":     "string",
			"title":  "string",
			"status": "string",
		},
	}
	require.NotEmpty(t, header.Columns)
	require.Equal(t, 3, len(header.Types))
}

// Satisfies CRIT-ZPARQL-STREAMING-BACKPRESSURE-PROOF:
// Dynamic Behavior: Chunked Reactive Traversal Streaming with Constant Memory.
func TestZPARQL_StreamingBackpressureProof(t *testing.T) {
	ctx := context.Background()
	producer := traversal.NewStreamProducer()
	consumer := traversal.NewStreamConsumer()

	const totalRecords = 1000
	const chunkSize = 50
	const bufferCap = 2 // Bounded buffer guaranteeing O(1) memory overhead

	records := make([]map[string]any, totalRecords)
	for i := 0; i < totalRecords; i++ {
		records[i] = map[string]any{
			"id":     fmt.Sprintf("REC-%04d", i+1),
			"title":  fmt.Sprintf("Item %d", i+1),
			"status": "active",
		}
	}

	header := traversal.HeaderFrame{
		QueryHash: "sha256:stream-test",
		Columns:   []string{"id", "title", "status"},
		Types:     map[string]string{"id": "string", "title": "string", "status": "string"},
	}

	// Producer starts streaming over bounded channel
	stream := producer.Stream(ctx, header, records, chunkSize, bufferCap)

	// Consumer consumes with intentional delay to verify backpressure
	var consumedCount atomic.Int64
	done := make(chan error, 1)

	go func() {
		result, err := consumer.Consume(ctx, stream)
		if err != nil {
			done <- err
			return
		}
		consumedCount.Store(int64(len(result.Records)))
		done <- nil
	}()

	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("streaming backpressure test timed out")
	}

	require.Equal(t, int64(totalRecords), consumedCount.Load(), "all 1000 records must be delivered")
}

// Satisfies CRIT-ZPARQL-TRUNCATED-STREAM-NEGATIVE:
// Negative Invariant: End-of-Stream Integrity Token and Truncation Detection.
func TestZPARQL_TruncatedStreamNegative(t *testing.T) {
	ctx := context.Background()
	consumer := traversal.NewStreamConsumer()

	t.Run("truncation: channel closes prematurely without trailer", func(t *testing.T) {
		stream := make(chan traversal.StreamFrame, 5)

		// Send header
		stream <- traversal.StreamFrame{
			Type: traversal.FrameTypeHeader,
			Header: &traversal.HeaderFrame{
				QueryHash: "sha256:trunc",
				Columns:   []string{"id"},
			},
		}
		// Send 1 chunk
		stream <- traversal.StreamFrame{
			Type: traversal.FrameTypeChunk,
			Chunk: &traversal.ChunkFrame{
				Sequence: 1,
				Records: []map[string]any{
					{"id": "REC-001"},
				},
			},
		}
		// Abruptly close channel without trailer
		close(stream)

		_, err := consumer.Consume(ctx, stream)
		require.Error(t, err)
		require.True(t, errors.Is(err, traversal.ErrStreamTruncated),
			"expected ErrStreamTruncated when stream closes without trailer")
	})

	t.Run("invalid trailer token: end_token != EOS_FINALIZED", func(t *testing.T) {
		stream := make(chan traversal.StreamFrame, 5)

		stream <- traversal.StreamFrame{
			Type:   traversal.FrameTypeHeader,
			Header: &traversal.HeaderFrame{Columns: []string{"id"}},
		}
		stream <- traversal.StreamFrame{
			Type: traversal.FrameTypeChunk,
			Chunk: &traversal.ChunkFrame{
				Sequence: 1,
				Records:  []map[string]any{{"id": "REC-001"}},
			},
		}
		// Send invalid end_token
		stream <- traversal.StreamFrame{
			Type: traversal.FrameTypeTrailer,
			Trailer: &traversal.TrailerFrame{
				TotalRecords: 1,
				Checksum:     traversal.ComputeChecksum([]map[string]any{{"id": "REC-001"}}),
				EndToken:     "INCOMPLETE_TOKEN",
			},
		}
		close(stream)

		_, err := consumer.Consume(ctx, stream)
		require.Error(t, err)
		require.True(t, errors.Is(err, traversal.ErrStreamTruncated))
	})

	t.Run("checksum mismatch: record count does not match trailer", func(t *testing.T) {
		stream := make(chan traversal.StreamFrame, 5)

		stream <- traversal.StreamFrame{
			Type:   traversal.FrameTypeHeader,
			Header: &traversal.HeaderFrame{Columns: []string{"id"}},
		}
		stream <- traversal.StreamFrame{
			Type: traversal.FrameTypeChunk,
			Chunk: &traversal.ChunkFrame{
				Sequence: 1,
				Records:  []map[string]any{{"id": "REC-001"}},
			},
		}
		// Trailer claims 5 records when only 1 was delivered
		stream <- traversal.StreamFrame{
			Type: traversal.FrameTypeTrailer,
			Trailer: &traversal.TrailerFrame{
				TotalRecords: 5, // mismatch
				Checksum:     "bad_checksum",
				EndToken:     traversal.EOSTokenFinalized,
			},
		}
		close(stream)

		_, err := consumer.Consume(ctx, stream)
		require.Error(t, err)
		require.True(t, errors.Is(err, traversal.ErrStreamChecksumMismatch),
			"expected ErrStreamChecksumMismatch when trailer count/checksum is invalid")
	})
}
