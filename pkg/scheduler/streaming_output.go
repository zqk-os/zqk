// Package scheduler: streaming_output provides a writer that streams to a file
// and keeps a bounded in-memory ring buffer for previews. Used by run_wrapper to
// avoid holding full command stdout/stderr in memory (which caused high RSS for
// long-running or verbose test jobs).

package scheduler

import (
	"io"
	"os"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
)

// DefaultPreviewBytes is the max bytes kept in memory for stdout/stderr preview (logging, notifications).
const DefaultPreviewBytes = 128 * 1024 // 128KB

// streamingOutputWriter writes to an optional file and a fixed-size ring buffer.
// Callers get preview via Preview(); full output is in the file only.
// Safe for concurrent Write from one goroutine and Preview/Len from others.
type streamingOutputWriter struct {
	file       io.Writer // nil if not streaming to file
	mu         sync.Mutex
	ring       []byte // ring buffer, len(ring) <= cap(ring)
	n          int    // number of bytes in ring (0 <= n <= len(ring))
	targetSize int    // maximum size of ring buffer
}

// newStreamingOutputWriter creates a writer that streams to file (if non-nil) and
// keeps the last previewSize bytes in memory. If file is nil, only the ring buffer is used.
func newStreamingOutputWriter(file io.Writer, previewSize int) *streamingOutputWriter {
	if previewSize <= 0 {
		previewSize = DefaultPreviewBytes
	}
	return &streamingOutputWriter{
		file:       file,
		ring:       make([]byte, 0, previewSize),
		targetSize: previewSize,
	}
}

// Write implements io.Writer. Writes to file first (if set), then appends to ring buffer (keeping last cap bytes).
// File I/O is done outside the lock; only the ring buffer is updated under RunInLock (LOCK_ORDERING, POLICY-ARCH-004).
func (w *streamingOutputWriter) Write(p []byte) (int, error) {
	if w == nil {
		return 0, nil
	}
	n := len(p)
	if n == 0 {
		return 0, nil
	}
	if w.file != nil {
		if _, err := w.file.Write(p); err != nil {
			// Best-effort: continue to ring buffer even if file write fails
		}
	}
	err := concurrency.RunInLock(&w.mu, func() error {
		if w.targetSize == 0 {
			return nil
		}
		w.ring = append(w.ring, p...)
		if len(w.ring) > w.targetSize {
			// Keep only the last targetSize bytes
			start := len(w.ring) - w.targetSize
			// Overwrite the beginning of the slice with the end to avoid reallocation
			copy(w.ring[0:w.targetSize], w.ring[start:])
			w.ring = w.ring[:w.targetSize]
		}
		w.n = len(w.ring)
		return nil
	})
	if err != nil {
		return n, err
	}
	return n, nil
}

// Preview returns a copy of the ring buffer contents (last N bytes written). Safe to call concurrently.
func (w *streamingOutputWriter) Preview() string {
	if w == nil {
		return ""
	}
	var s string
	_ = concurrency.RunInLock(&w.mu, func() error {
		if w.n == 0 {
			return nil
		}
		s = string(w.ring[:w.n])
		return nil
	})
	return s
}

// Len returns the number of bytes currently in the ring buffer (for progress logging). Safe to call concurrently.
func (w *streamingOutputWriter) Len() int {
	if w == nil {
		return 0
	}
	var n int
	_ = concurrency.RunInLock(&w.mu, func() error {
		n = w.n
		return nil
	})
	return n
}

// maxBytesToReadForTestParse is the limit when reading job stdout/stderr from disk for test output parsing.
// Keeps memory bounded while usually including the final test summary and failure lines.
const maxBytesToReadForTestParse = 2 * 1024 * 1024 // 2MB total (1MB per stream when combined)

// readLastBytesFromFile reads up to maxBytes from the end of the file (for test parsing without loading full output).
// Returns nil if the file does not exist or cannot be read.
func readLastBytesFromFile(path string, maxBytes int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	if size == 0 {
		return nil, nil
	}
	if int(size) <= maxBytes {
		maxBytes = int(size)
	}
	_, err = f.Seek(-int64(maxBytes), io.SeekEnd)
	if err != nil {
		return nil, err
	}
	buf := make([]byte, maxBytes)
	n, err := io.ReadFull(f, buf)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return nil, err
	}
	return buf[:n], nil
}
