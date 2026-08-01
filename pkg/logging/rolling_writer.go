package logging

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// RollingPolicy defines how log files should be rotated
type RollingPolicy struct {
	// Strategy determines the rotation strategy
	// Options: "size", "time", "size_and_time", "none"
	// Default: "size"
	Strategy string `yaml:"strategy"`

	// Size-based rotation (used by "size" and "size_and_time" strategies)
	MaxSize  int64 `yaml:"max_size"`  // Maximum file size in bytes before rotation (default: 10MB)
	MaxFiles int   `yaml:"max_files"` // Maximum number of rotated files to retain (default: 5)

	// Time-based rotation (used by "time" and "size_and_time" strategies)
	MaxAge      string `yaml:"max_age"`      // Maximum age of log files (e.g., "24h", "7d", "30d")
	RotateEvery string `yaml:"rotate_every"` // Rotate every N time units (e.g., "1d", "1h")
}

// DefaultRollingPolicy returns a default rolling policy
func DefaultRollingPolicy() RollingPolicy {
	return RollingPolicy{
		Strategy: "size",
		MaxSize:  10 * 1024 * 1024, // 10MB
		MaxFiles: 5,
	}
}

// RollingWriter is an interface for rolling log writers
type RollingWriter interface {
	io.Writer
	io.Closer
	Sync() error
}

// RollingWriterFactory creates rolling writers based on policy
type RollingWriterFactory struct{}

// NewRollingWriterFactory creates a new factory
func NewRollingWriterFactory() *RollingWriterFactory {
	return &RollingWriterFactory{}
}

// CreateWriter creates a rolling writer based on the policy
func (f *RollingWriterFactory) CreateWriter(filePath string, policy RollingPolicy) (RollingWriter, error) {
	// Normalize strategy
	strategy := strings.ToLower(policy.Strategy)
	if strategy == emptyValue {
		strategy = "size"
	}

	switch strategy {
	case "none", "disabled":
		// No rolling - use simple file writer
		return f.createSimpleWriter(filePath)
	case "size":
		return f.createSizeBasedWriter(filePath, policy)
	case "time":
		return f.createTimeBasedWriter(filePath, policy)
	case "size_and_time":
		return f.createSizeAndTimeWriter(filePath, policy)
	default:
		// Default to size-based
		return f.createSizeBasedWriter(filePath, policy)
	}
}

// createSimpleWriter creates a simple file writer without rolling
func (f *RollingWriterFactory) createSimpleWriter(filePath string) (RollingWriter, error) {
	if err := fileutil.EnsureDir(filepath.Dir(filePath)); err != nil {
		return nil, errfmt.Newf("failed to create log directory").Wrap(err)
	}

	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, errfmt.Newf("failed to open log file").Wrap(err)
	}

	return &simpleFileWriter{file: file}, nil
}

// createSizeBasedWriter creates a size-based rolling writer
func (f *RollingWriterFactory) createSizeBasedWriter(filePath string, policy RollingPolicy) (RollingWriter, error) {
	config := SizeBasedRollingConfig{
		MaxSize:  policy.MaxSize,
		MaxFiles: policy.MaxFiles,
	}

	// Apply defaults
	if config.MaxSize <= 0 {
		config.MaxSize = DefaultRollingPolicy().MaxSize
	}
	if config.MaxFiles <= 0 {
		config.MaxFiles = DefaultRollingPolicy().MaxFiles
	}

	return NewSizeBasedRollingWriter(filePath, config)
}

// createTimeBasedWriter creates a time-based rolling writer
func (f *RollingWriterFactory) createTimeBasedWriter(filePath string, policy RollingPolicy) (RollingWriter, error) {
	// Time-based rolling not yet implemented - fall back to size-based
	// TODO: Implement time-based rotation
	return f.createSizeBasedWriter(filePath, policy)
}

// createSizeAndTimeWriter creates a combined size and time-based rolling writer
func (f *RollingWriterFactory) createSizeAndTimeWriter(filePath string, policy RollingPolicy) (RollingWriter, error) {
	// Combined strategy not yet implemented - fall back to size-based
	// TODO: Implement combined size and time-based rotation
	return f.createSizeBasedWriter(filePath, policy)
}

// simpleFileWriter is a simple file writer without rolling
type simpleFileWriter struct {
	file *os.File
	mu   sync.Mutex
}

func (w *simpleFileWriter) Write(p []byte) (n int, err error) {
	var writeN int
	var writeErr error
	err = concurrency.WithLockCtx(
		&w.mu,
		pkgctx.NewSystemContext(),
		"simple_file_writer_write",
		func() error {
			writeN, writeErr = w.file.Write(p)
			return writeErr
		},
	)
	return writeN, err
}

func (w *simpleFileWriter) Close() error {
	var file *os.File
	_ = concurrency.WithLockCtx(
		&w.mu,
		pkgctx.NewSystemContext(),
		"simple_file_writer_close",
		func() error {
			file = w.file
			w.file = nil
			return nil
		},
	)
	if file != nil {
		return file.Close()
	}
	return nil
}

func (w *simpleFileWriter) Sync() error {
	var file *os.File
	_ = concurrency.WithLockCtx(
		&w.mu,
		pkgctx.NewSystemContext(),
		"simple_file_writer_sync",
		func() error {
			file = w.file
			return nil
		},
	)
	if file != nil {
		return file.Sync()
	}
	return nil
}

// SizeBasedRollingConfig configures size-based rolling behavior
type SizeBasedRollingConfig struct {
	MaxSize  int64 // Maximum file size before rotation (bytes)
	MaxFiles int   // Maximum number of rotated files to retain
}

// SizeBasedRollingWriter implements size-based rolling
type SizeBasedRollingWriter struct {
	basePath      string
	maxSize       int64
	maxFiles      int
	currentFile   *os.File
	currentSize   int64
	mu            sync.Mutex
	rotationCount int
}

// NewSizeBasedRollingWriter creates a new size-based rolling writer
func NewSizeBasedRollingWriter(basePath string, config SizeBasedRollingConfig) (*SizeBasedRollingWriter, error) {
	if config.MaxSize <= 0 {
		config.MaxSize = DefaultRollingPolicy().MaxSize
	}
	if config.MaxFiles <= 0 {
		config.MaxFiles = DefaultRollingPolicy().MaxFiles
	}

	// Create directory if needed
	if err := fileutil.EnsureDir(filepath.Dir(basePath)); err != nil {
		return nil, errfmt.Newf("failed to create log directory").Wrap(err)
	}

	// Open or create the initial file
	f, err := os.OpenFile(basePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, errfmt.Newf("failed to open log file").Wrap(err)
	}

	// Get current file size
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, errfmt.Newf("failed to stat log file").Wrap(err)
	}

	rtw := &SizeBasedRollingWriter{
		basePath:      basePath,
		maxSize:       config.MaxSize,
		maxFiles:      config.MaxFiles,
		currentFile:   f,
		currentSize:   stat.Size(),
		rotationCount: 0,
	}

	return rtw, nil
}

// Write implements io.Writer interface
func (rtw *SizeBasedRollingWriter) Write(p []byte) (n int, err error) {
	var writeN int
	var writeErr error
	err = concurrency.WithLockCtx(
		&rtw.mu,
		pkgctx.NewSystemContext(),
		"rolling_writer_write",
		func() error {
			// Check if we need to rotate before writing
			if rtw.currentSize+int64(len(p)) > rtw.maxSize {
				if rotateErr := rtw.rotateLocked(); rotateErr != nil {
					return errfmt.Newf("failed to rotate log file").Wrap(rotateErr)
				}
			}

			// Write to current file
			writeN, writeErr = rtw.currentFile.Write(p)
			if writeErr != nil {
				return writeErr
			}

			// Update size
			rtw.currentSize += int64(writeN)

			// Check if we need to rotate after writing
			if rtw.currentSize > rtw.maxSize {
				if rotateErr := rtw.rotateLocked(); rotateErr != nil {
					return errfmt.Newf("failed to rotate log file after write").Wrap(rotateErr)
				}
			}

			return nil
		},
	)
	return writeN, err
}

// rotateLocked rotates the log file (must be called with mu held)
func (rtw *SizeBasedRollingWriter) rotateLocked() error {
	// Close current file
	if rtw.currentFile != nil {
		if err := rtw.currentFile.Close(); err != nil {
			return errfmt.Newf("failed to close current log file").Wrap(err)
		}
	}

	// Generate rotated filename with timestamp
	rtw.rotationCount++
	rotatedPath := rtw.getRotatedPath(rtw.rotationCount)

	// Rename current file to rotated name
	if err := os.Rename(rtw.basePath, rotatedPath); err != nil {
		// If rename fails, try to continue with new file
	}

	// Clean up old rotated files
	if err := rtw.cleanupOldFilesLocked(); err != nil {
		// Log error but don't fail rotation
	}

	// Open new file
	f, err := os.OpenFile(rtw.basePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return errfmt.Newf("failed to open new log file after rotation").Wrap(err)
	}

	rtw.currentFile = f
	rtw.currentSize = 0

	return nil
}

// getRotatedPath generates a path for a rotated file
func (rtw *SizeBasedRollingWriter) getRotatedPath(rotationNum int) string {
	dir := filepath.Dir(rtw.basePath)
	baseName := filepath.Base(rtw.basePath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)

	// Format: <name>-<timestamp>-<rotation>.ext
	timestamp := zqktime.NowLayoutUTC(zqktime.LayoutLogRotateStamp)
	return filepath.Join(dir, fmt.Sprintf("%s-%s-%d%s", nameWithoutExt, timestamp, rotationNum, ext))
}

// cleanupOldFilesLocked removes old rotated files beyond maxFiles limit (must be called with mu held)
func (rtw *SizeBasedRollingWriter) cleanupOldFilesLocked() error {
	dir := filepath.Dir(rtw.basePath)
	baseName := filepath.Base(rtw.basePath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)

	// Pattern: <name>-<timestamp>-<rotation>.ext
	pattern := nameWithoutExt + "-*-*" + ext

	// Find all rotated files matching the pattern
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return errfmt.Newf("failed to glob rotated files").Wrap(err)
	}

	// Sort by modification time (oldest first)
	sort.Slice(matches, func(i, j int) bool {
		statI, errI := os.Stat(matches[i])
		statJ, errJ := os.Stat(matches[j])
		if errI != nil || errJ != nil {
			return false
		}
		return statI.ModTime().Before(statJ.ModTime())
	})

	// Delete oldest files if we exceed maxFiles
	if len(matches) > rtw.maxFiles {
		for i := 0; i < len(matches)-rtw.maxFiles; i++ {
			if err := os.Remove(matches[i]); err != nil {
				continue
			}
		}
	}

	return nil
}

// Close implements io.Closer interface
func (rtw *SizeBasedRollingWriter) Close() error {
	var file *os.File
	_ = concurrency.WithLockCtx(
		&rtw.mu,
		pkgctx.NewSystemContext(),
		"rolling_writer_close",
		func() error {
			file = rtw.currentFile
			rtw.currentFile = nil
			return nil
		},
	)

	if file != nil {
		return file.Close()
	}

	return nil
}

// Sync flushes the current file to disk
func (rtw *SizeBasedRollingWriter) Sync() error {
	var file *os.File
	_ = concurrency.WithLockCtx(
		&rtw.mu,
		pkgctx.NewSystemContext(),
		"rolling_writer_sync",
		func() error {
			file = rtw.currentFile
			return nil
		},
	)

	if file != nil {
		return file.Sync()
	}

	return nil
}
