package traversal

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

const (
	EOSTokenFinalized = "EOS_FINALIZED"
)

var (
	ErrStreamTruncated        = errors.New("ERR_ZPARQL_STREAM_TRUNCATED: stream closed prematurely without EOS_FINALIZED trailer")
	ErrStreamChecksumMismatch = errors.New("ERR_ZPARQL_STREAM_CHECKSUM_MISMATCH: record count or checksum mismatch in stream trailer")
	ErrStreamSequenceGap      = errors.New("ERR_ZPARQL_STREAM_SEQUENCE_GAP: gap detected in chunk sequence numbering")
)

// FrameType identifies the envelope frame type.
type FrameType string

const (
	FrameTypeHeader  FrameType = "header"
	FrameTypeChunk   FrameType = "chunk"
	FrameTypeTrailer FrameType = "trailer"
)

// HeaderFrame defines the projection metadata and column types.
type HeaderFrame struct {
	QueryHash string            `json:"query_hash"`
	Columns   []string          `json:"columns"`
	Types     map[string]string `json:"types"`
}

// ChunkFrame carries a batch of matched entities.
type ChunkFrame struct {
	Sequence int              `json:"sequence"`
	Records  []map[string]any `json:"records"`
}

// TrailerFrame carries the end-of-stream integrity token and checksum.
type TrailerFrame struct {
	TotalRecords int    `json:"total_records"`
	Checksum     string `json:"checksum"`
	EndToken     string `json:"end_token"`
}

// StreamFrame is a discriminated union of envelope frames.
type StreamFrame struct {
	Type    FrameType     `json:"type"`
	Header  *HeaderFrame  `json:"header,omitempty"`
	Chunk   *ChunkFrame   `json:"chunk,omitempty"`
	Trailer *TrailerFrame `json:"trailer,omitempty"`
}

// ComputeChecksum calculates a deterministic SHA-256 hash across record IDs.
func ComputeChecksum(records []map[string]any) string {
	h := sha256.New()
	for _, r := range records {
		if id, ok := r["id"].(string); ok {
			h.Write([]byte(id))
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// StreamProducer streams records in fixed-size chunks over a bounded channel, providing backpressure.
type StreamProducer struct{}

// NewStreamProducer creates a stream producer.
func NewStreamProducer() *StreamProducer {
	return &StreamProducer{}
}

// Stream streams records through a bounded channel buffer.
func (p *StreamProducer) Stream(
	ctx context.Context,
	header HeaderFrame,
	records []map[string]any,
	chunkSize int,
	bufferCapacity int,
) <-chan StreamFrame {
	if chunkSize <= 0 {
		chunkSize = 50
	}
	if bufferCapacity <= 0 {
		bufferCapacity = 2
	}

	out := make(chan StreamFrame, bufferCapacity)

	go func() {
		defer close(out)

		// 1. Emit Header Frame
		select {
		case <-ctx.Done():
			return
		case out <- StreamFrame{Type: FrameTypeHeader, Header: &header}:
		}

		// 2. Emit Chunk Frames in sequence
		seq := 1
		for i := 0; i < len(records); i += chunkSize {
			end := i + chunkSize
			if end > len(records) {
				end = len(records)
			}

			chunkRecords := make([]map[string]any, end-i)
			copy(chunkRecords, records[i:end])

			chunk := ChunkFrame{
				Sequence: seq,
				Records:  chunkRecords,
			}
			seq++

			select {
			case <-ctx.Done():
				return
			case out <- StreamFrame{Type: FrameTypeChunk, Chunk: &chunk}:
			}
		}

		// 3. Emit Final Trailer Frame with EOS Token and Checksum
		trailer := TrailerFrame{
			TotalRecords: len(records),
			Checksum:     ComputeChecksum(records),
			EndToken:     EOSTokenFinalized,
		}

		select {
		case <-ctx.Done():
			return
		case out <- StreamFrame{Type: FrameTypeTrailer, Trailer: &trailer}:
		}
	}()

	return out
}

// ConsumedResult represents the fully validated stream delivery.
type ConsumedResult struct {
	Header       HeaderFrame      `json:"header"`
	Records      []map[string]any `json:"records"`
	TotalRecords int              `json:"total_records"`
	Checksum     string           `json:"checksum"`
	Finalized    bool             `json:"finalized"`
}

// StreamConsumer validates chunk sequences, trailer tokens, and checksums fail-closed.
type StreamConsumer struct{}

// NewStreamConsumer creates a stream consumer.
func NewStreamConsumer() *StreamConsumer {
	return &StreamConsumer{}
}

// Consume drains a StreamFrame channel, enforcing all integrity rules.
func (c *StreamConsumer) Consume(ctx context.Context, stream <-chan StreamFrame) (*ConsumedResult, error) {
	result := &ConsumedResult{
		Records: make([]map[string]any, 0),
	}

	headerReceived := false
	trailerReceived := false
	expectedSequence := 1

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case frame, ok := <-stream:
			if !ok {
				// Channel closed: verify clean finalization
				if !trailerReceived {
					return nil, ErrStreamTruncated
				}
				return result, nil
			}

			switch frame.Type {
			case FrameTypeHeader:
				if frame.Header == nil {
					return nil, fmt.Errorf("%w: nil header frame", ErrStreamTruncated)
				}
				result.Header = *frame.Header
				headerReceived = true

			case FrameTypeChunk:
				if !headerReceived {
					return nil, fmt.Errorf("%w: chunk received before header", ErrStreamTruncated)
				}
				if frame.Chunk == nil {
					return nil, fmt.Errorf("%w: nil chunk frame", ErrStreamTruncated)
				}
				if frame.Chunk.Sequence != expectedSequence {
					return nil, fmt.Errorf("%w: expected sequence %d, got %d", ErrStreamSequenceGap, expectedSequence, frame.Chunk.Sequence)
				}
				expectedSequence++
				result.Records = append(result.Records, frame.Chunk.Records...)

			case FrameTypeTrailer:
				if frame.Trailer == nil {
					return nil, fmt.Errorf("%w: nil trailer frame", ErrStreamTruncated)
				}
				if frame.Trailer.EndToken != EOSTokenFinalized {
					return nil, fmt.Errorf("%w: invalid end_token %q", ErrStreamTruncated, frame.Trailer.EndToken)
				}

				// Checksum and Count Verification
				computedChecksum := ComputeChecksum(result.Records)
				if frame.Trailer.TotalRecords != len(result.Records) || frame.Trailer.Checksum != computedChecksum {
					return nil, fmt.Errorf("%w: expected count %d (got %d), expected checksum %s (got %s)",
						ErrStreamChecksumMismatch, frame.Trailer.TotalRecords, len(result.Records), frame.Trailer.Checksum, computedChecksum)
				}

				result.TotalRecords = frame.Trailer.TotalRecords
				result.Checksum = frame.Trailer.Checksum
				result.Finalized = true
				trailerReceived = true

			default:
				return nil, fmt.Errorf("unknown frame type: %s", frame.Type)
			}
		}
	}
}
