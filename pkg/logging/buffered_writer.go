package logging

import (
	"bufio"
	"io"
	"sync"
	"time"
)

// BufferedWriterConfig configures buffered writer behavior
type BufferedWriterConfig struct {
	// BufferSize is the size of the buffer in bytes
	// Default: 64KB
	BufferSize int

	// FlushInterval is how often to flush the buffer automatically
	// Set to 0 to disable automatic flushing
	// Default: 1 second
	FlushInterval time.Duration

	// FlushOnError causes immediate flush on error-level logs
	// Default: true
	FlushOnError bool

	// FlushOnFatal causes immediate flush on fatal-level logs
	// Default: true
	FlushOnFatal bool
}

// DefaultBufferedWriterConfig returns default buffered writer configuration
func DefaultBufferedWriterConfig() BufferedWriterConfig {
	return BufferedWriterConfig{
		BufferSize:    64 * 1024, // 64KB
		FlushInterval: 1 * time.Second,
		FlushOnError:  true,
		FlushOnFatal:  true,
	}
}

// BufferedWriter wraps an io.Writer with buffering to minimize I/O context switches.
// Auto-flush is process-wide (one ticker), not one goroutine per writer.
type BufferedWriter struct {
	writer    io.Writer
	buf       *bufio.Writer
	config    BufferedWriterConfig
	mu        sync.Mutex
	autoFlush bool
}

// NewBufferedWriter creates a new buffered writer
func NewBufferedWriter(writer io.Writer, config BufferedWriterConfig) *BufferedWriter {
	if config.BufferSize <= 0 {
		config.BufferSize = DefaultBufferedWriterConfig().BufferSize
	}
	if config.FlushInterval <= 0 {
		config.FlushInterval = DefaultBufferedWriterConfig().FlushInterval
	}

	bw := &BufferedWriter{
		writer:    writer,
		buf:       bufio.NewWriterSize(writer, config.BufferSize),
		config:    config,
		autoFlush: config.FlushInterval > 0,
	}
	if bw.autoFlush {
		processBufferedFlusher.register(bw)
	}
	return bw
}

// Write writes data to the buffer.
func (bw *BufferedWriter) Write(p []byte) (int, error) {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.writeLocked(p, false)
}

// WriteWithLevel writes data and flushes in the same critical section for
// error/fatal (config flags are immutable after NewBufferedWriter).
func (bw *BufferedWriter) WriteWithLevel(p []byte, level LogLevel) (int, error) {
	forceFlush := (level == ErrorLevel && bw.config.FlushOnError) ||
		(level == FatalLevel && bw.config.FlushOnFatal)
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.writeLocked(p, forceFlush)
}

func (bw *BufferedWriter) writeLocked(p []byte, forceFlush bool) (int, error) {
	n, err := bw.buf.Write(p)
	if err != nil {
		return n, err
	}
	if forceFlush || bw.buf.Available() == 0 {
		if err := bw.buf.Flush(); err != nil {
			return n, err
		}
	}
	return n, nil
}

// Flush flushes the buffer to the underlying writer.
// Plain mutex: timeout-context wrappers belong on contended locks, not the 1s log flusher
// (REQ-CEF-MUTEX-DISCIPLINE). bufio.Writer is not concurrent-safe, so
// Buffered/Available also take this lock rather than racing b.n.
func (bw *BufferedWriter) Flush() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.buf.Flush()
}

// Sync flushes the buffer and syncs the underlying writer if it supports it
func (bw *BufferedWriter) Sync() error {
	if err := bw.Flush(); err != nil {
		return err
	}

	if syncer, ok := bw.writer.(interface{ Sync() error }); ok {
		return syncer.Sync()
	}

	return nil
}

// Close closes the buffered writer, flushing any remaining data
func (bw *BufferedWriter) Close() error {
	if bw.autoFlush {
		processBufferedFlusher.unregister(bw)
	}

	bw.mu.Lock()
	err := bw.buf.Flush()
	bw.mu.Unlock()
	if err != nil {
		return err
	}

	if closer, ok := bw.writer.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}

// Buffered returns the number of bytes currently buffered
func (bw *BufferedWriter) Buffered() int {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.buf.Buffered()
}

// Available returns the number of bytes available in the buffer
func (bw *BufferedWriter) Available() int {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	return bw.buf.Available()
}
