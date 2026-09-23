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
	writer        io.Writer
	buf           *bufio.Writer
	config        BufferedWriterConfig
	mu            sync.Mutex
	autoFlush     bool
	lastFlushTime time.Time
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
		writer:        writer,
		buf:           bufio.NewWriterSize(writer, config.BufferSize),
		config:        config,
		autoFlush:     config.FlushInterval > 0,
		lastFlushTime: time.Now(),
	}
	if bw.autoFlush {
		processBufferedFlusher.register(bw)
	}
	return bw
}

// Write writes data to the buffer
func (bw *BufferedWriter) Write(p []byte) (n int, err error) {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	n, err = bw.buf.Write(p)
	if err != nil {
		return n, err
	}
	if bw.buf.Available() == 0 {
		if err := bw.buf.Flush(); err != nil {
			return n, err
		}
		bw.lastFlushTime = time.Now()
	}
	return n, nil
}

// WriteWithLevel writes data with a log level, flushing immediately for error/fatal if configured
func (bw *BufferedWriter) WriteWithLevel(p []byte, level LogLevel) (n int, err error) {
	n, err = bw.Write(p)
	if err != nil {
		return n, err
	}

	bw.mu.Lock()
	shouldFlush := (level == ErrorLevel && bw.config.FlushOnError) ||
		(level == FatalLevel && bw.config.FlushOnFatal)
	bw.mu.Unlock()

	if shouldFlush {
		if err := bw.Flush(); err != nil {
			return n, err
		}
	}

	return n, nil
}

// Flush flushes the buffer to the underlying writer.
// Plain mutex: timeout-context wrappers belong on contended locks, not the 1s log flusher
// (REQ-1790151409621692000-007e7255).
func (bw *BufferedWriter) Flush() error {
	bw.mu.Lock()
	defer bw.mu.Unlock()
	if err := bw.buf.Flush(); err != nil {
		return err
	}
	bw.lastFlushTime = time.Now()
	return nil
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
