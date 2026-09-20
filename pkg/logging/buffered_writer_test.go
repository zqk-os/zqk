package logging

import (
	"io"
	"runtime"
	"testing"
	"time"
)

func TestBufferedWriter_SharedFlusherDoesNotSpawnPerWriter(t *testing.T) {
	before := runtime.NumGoroutine()
	writers := make([]*BufferedWriter, 0, 32)
	for i := 0; i < 32; i++ {
		writers = append(writers, NewBufferedWriter(io.Discard, DefaultBufferedWriterConfig()))
	}
	t.Cleanup(func() {
		for _, w := range writers {
			_ = w.Close()
		}
	})
	time.Sleep(30 * time.Millisecond)
	delta := runtime.NumGoroutine() - before
	// Shared flusher is one goroutine (already started after the first writer).
	// Allow a little harness noise; 32 per-writer tickers would be ~+32.
	if delta > 8 {
		t.Fatalf("NumGoroutine grew by %d after 32 BufferedWriters; shared flusher should not spawn per writer", delta)
	}
}

func TestBufferedWriter_WriteThenCloseFlushes(t *testing.T) {
	var got []byte
	w := &captureWriter{}
	bw := NewBufferedWriter(w, DefaultBufferedWriterConfig())
	if _, err := bw.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := bw.Close(); err != nil {
		t.Fatal(err)
	}
	got = w.buf
	if string(got) != "hello" {
		t.Fatalf("flushed %q, want hello", got)
	}
}

type captureWriter struct {
	buf []byte
}

func (c *captureWriter) Write(p []byte) (int, error) {
	c.buf = append(c.buf, p...)
	return len(p), nil
}
