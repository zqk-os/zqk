package chunking

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
)

// Chunk represents a piece of data from a stream
type Chunk struct {
	SequenceID int
	IsLast     bool
	Data       []byte
	Hash       string
}

// ComputeHash computes the SHA-256 hash of the chunk data
func ComputeHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// Chunker reads from an io.Reader and emits chunks to the given channel.
func Chunker(reader io.Reader, chunkSize int, out chan<- Chunk) error {
	defer close(out)
	if chunkSize <= 0 {
		return errors.New("chunk size must be greater than 0")
	}

	seqID := 0
	buf := make([]byte, chunkSize)

	var prevChunk *Chunk

	for {
		n, err := reader.Read(buf)
		if n > 0 {
			chunkData := make([]byte, n)
			copy(chunkData, buf[:n])

			if prevChunk != nil {
				out <- *prevChunk
				seqID++
			}

			prevChunk = &Chunk{
				SequenceID: seqID,
				IsLast:     false,
				Data:       chunkData,
				Hash:       ComputeHash(chunkData),
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
	}

	if prevChunk != nil {
		prevChunk.IsLast = true
		out <- *prevChunk
	} else {
		// Empty reader
		out <- Chunk{
			SequenceID: 0,
			IsLast:     true,
			Data:       []byte{},
			Hash:       ComputeHash([]byte{}),
		}
	}

	return nil
}

// Reassembler takes chunks, verifies their hashes, and reconstructs the asset.
type Reassembler struct {
	chunks     map[int]Chunk
	writer     io.Writer
	nextSeq    int
	lastSeq    int
	isLastSeen bool
}

// NewReassembler creates a new Reassembler.
func NewReassembler(writer io.Writer) *Reassembler {
	return &Reassembler{
		chunks:  make(map[int]Chunk),
		writer:  writer,
		lastSeq: -1,
	}
}

// AddChunk adds an out-of-order chunk and writes to the underlying writer if possible.
func (r *Reassembler) AddChunk(c Chunk) error {
	// Verify hash
	if ComputeHash(c.Data) != c.Hash {
		return errors.New("hash verification failed")
	}

	if c.IsLast {
		r.isLastSeen = true
		r.lastSeq = c.SequenceID
	}

	r.chunks[c.SequenceID] = c

	// Try to write contiguous chunks
	for {
		chunk, ok := r.chunks[r.nextSeq]
		if !ok {
			break
		}

		_, err := r.writer.Write(chunk.Data)
		if err != nil {
			return err
		}

		delete(r.chunks, r.nextSeq)
		r.nextSeq++
	}

	return nil
}

// IsComplete returns true if all chunks have been received and written.
func (r *Reassembler) IsComplete() bool {
	return r.isLastSeen && r.nextSeq > r.lastSeq
}
