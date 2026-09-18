package logging

import (
	"bufio"
	"io"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
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
	err = concurrency.WithLockCtx(
		&bw.mu,
		pkgctx.NewSystemContext(),
		"buffered_writer_write",
		func() error {
			var writeErr error
			n, writeErr = bw.buf.Write(p)
			if writeErr != nil {
				return writeErr
			}

			// Check if buffer is full and needs flushing
			if bw.buf.Available() == 0 {
				if flushErr := bw.buf.Flush(); flushErr != nil {
					return flushErr
				}
				bw.lastFlushTime = time.Now()
			}

			return nil
		},
	)
	return n, err
}

// WriteWithLevel writes data with a log level, flushing immediately for error/fatal if configured
func (bw *BufferedWriter) WriteWithLevel(p []byte, level LogLevel) (n int, err error) {
	n, err = bw.Write(p)
	if err != nil {
		return n, err
	}

	// Flush immediately for error/fatal logs if configured
	var shouldFlush bool
	_ = concurrency.WithLock(
		&bw.mu,
		"buffered_writer_check_flush",
		func() error {
			shouldFlush = false
			if level == ErrorLevel && bw.config.FlushOnError {
				shouldFlush = true
			} else if level == FatalLevel && bw.config.FlushOnFatal {
				shouldFlush = true
			}
			return nil
		},
	)

	if shouldFlush {
		if err := bw.Flush(); err != nil {
			return n, err
		}
	}

	return n, nil
}

// Flush flushes the buffer to the underlying writer
func (bw *BufferedWriter) Flush() error {
	var flushErr error
	err := concurrency.WithLockCtx(
		&bw.mu,
		pkgctx.NewSystemContext(),
		"buffered_writer_flush",
		func() error {
			flushErr = bw.buf.Flush()
			if flushErr == nil {
				bw.lastFlushTime = time.Now()
			}
			return flushErr
		},
	)
	return err
}

// Sync flushes the buffer and syncs the underlying writer if it supports it
func (bw *BufferedWriter) Sync() error {
	if err := bw.Flush(); err != nil {
		return err
	}

	// If the underlying writer supports Sync, call it
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

	err := concurrency.WithLockCtx(
		&bw.mu,
		pkgctx.NewSystemContext(),
		"buffered_writer_close_flush",
		func() error {
			return bw.buf.Flush()
		},
	)
	if err != nil {
		return err
	}

	// Close underlying writer if it's a Closer (outside lock)
	if closer, ok := bw.writer.(io.Closer); ok {
		return closer.Close()
	}

	return nil
}

// Buffered returns the number of bytes currently buffered
func (bw *BufferedWriter) Buffered() int {
	var buffered int
	_ = concurrency.WithLockCtx(
		&bw.mu,
		pkgctx.NewSystemContext(),
		"buffered_writer_buffered",
		func() error {
			buffered = bw.buf.Buffered()
			return nil
		},
	)
	return buffered
}

// Available returns the number of bytes available in the buffer
func (bw *BufferedWriter) Available() int {
	var available int
	_ = concurrency.WithLockCtx(
		&bw.mu,
		pkgctx.NewSystemContext(),
		"buffered_writer_available",
		func() error {
			available = bw.buf.Available()
			return nil
		},
	)
	return available
}
