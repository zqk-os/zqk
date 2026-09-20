package mcp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// RollingTraceWriter implements a rolling file appender for trace logs
// It automatically rotates logs when they reach a maximum size and retains a configurable number of old logs
type RollingTraceWriter struct {
	basePath      string         // Base path for trace file (e.g., "mcp-trace.log")
	maxSize       int64          // Maximum file size before rotation (bytes)
	maxFiles      int            // Maximum number of rotated files to retain
	currentFile   *fileutil.File // Current file being written to
	currentSize   int64          // Current size of the file
	mu            sync.Mutex     // Protects concurrent writes and rotations
	rotationCount int            // Number of rotations performed
}

// RollingTraceConfig configures rolling trace writer behavior
type RollingTraceConfig struct {
	// MaxSize is the maximum file size before rotation (e.g., 10MB = 10 * 1024 * 1024)
	// Default: 10MB
	MaxSize int64 `yaml:"max_size"`

	// MaxFiles is the maximum number of rotated files to retain
	// Oldest files are deleted when this limit is exceeded
	// Default: 5
	MaxFiles int `yaml:"max_files"`
}

// DefaultRollingTraceConfig returns default rolling trace configuration
func DefaultRollingTraceConfig() RollingTraceConfig {
	return RollingTraceConfig{
		MaxSize:  10 * 1024 * 1024, // 10MB
		MaxFiles: 5,                // Keep 5 rotated files
	}
}

// NewRollingTraceWriter creates a new rolling trace writer
func NewRollingTraceWriter(basePath string, config RollingTraceConfig) (*RollingTraceWriter, error) {
	if config.MaxSize <= 0 {
		config.MaxSize = DefaultRollingTraceConfig().MaxSize
	}
	if config.MaxFiles <= 0 {
		config.MaxFiles = DefaultRollingTraceConfig().MaxFiles
	}

	// Create directory if needed
	if err := fileutil.EnsureDir(filepath.Dir(basePath)); err != nil {
		return nil, errfmt.Newf("failed to create trace directory").Wrap(err)
	}

	// Open or create the initial file
	f, err := fileutil.OpenFile(basePath, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, 0o600)
	if err != nil {
		return nil, errfmt.Newf("failed to open trace file").Wrap(err)
	}

	// Get current file size
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, errfmt.Newf("failed to stat trace file").Wrap(err)
	}

	rtw := &RollingTraceWriter{
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
// Automatically rotates the log file when it exceeds maxSize
func (rtw *RollingTraceWriter) Write(p []byte) (n int, err error) {
	var writeN int
	var writeErr error
	_ = concurrency.RunInLockWithLogger(
		&rtw.mu, LockNameRollingTraceWriterWrite, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Check if we need to rotate before writing
			if rtw.currentSize+int64(len(p)) > rtw.maxSize {
				if err := rtw.rotateLocked(); err != nil {
					writeErr = errfmt.Newf("failed to rotate trace file").Wrap(err)
					return nil
				}
			}

			// Write to current file
			writeN, writeErr = rtw.currentFile.Write(p)
			if writeErr != nil {
				return nil
			}

			// Update size
			rtw.currentSize += int64(writeN)

			// Check if we need to rotate after writing (in case write pushed us over)
			if rtw.currentSize > rtw.maxSize {
				if err := rtw.rotateLocked(); err != nil {
					writeErr = errfmt.Newf("failed to rotate trace file after write").Wrap(err)
					return nil
				}
			}

			return nil
		},
	)
	return writeN, writeErr
}

// rotateLocked rotates the log file (must be called with mu held)
func (rtw *RollingTraceWriter) rotateLocked() error {
	// Close current file
	if rtw.currentFile != nil {
		if err := rtw.currentFile.Close(); err != nil {
			return errfmt.Newf("failed to close current trace file").Wrap(err)
		}
	}

	// Generate rotated filename with timestamp
	rtw.rotationCount++
	rotatedPath := rtw.getRotatedPath(rtw.rotationCount)

	// Rename current file to rotated name
	if err := fileutil.Rename(rtw.basePath, rotatedPath); err != nil {
		// If rename fails, try to continue with new file (old file may be gone)
		// This prevents blocking if there's a filesystem issue
	}

	// Clean up old rotated files
	if err := rtw.cleanupOldFilesLocked(); err != nil {
		// Log error but don't fail rotation
		// Errors are non-critical - we can continue with new file
	}

	// Open new file
	f, err := fileutil.OpenFile(rtw.basePath, fileutil.O_CREATE|fileutil.O_WRONLY|fileutil.O_APPEND, 0o600)
	if err != nil {
		return errfmt.Newf("failed to open new trace file after rotation").Wrap(err)
	}

	rtw.currentFile = f
	rtw.currentSize = 0

	return nil
}

// getRotatedPath generates a path for a rotated file
func (rtw *RollingTraceWriter) getRotatedPath(rotationNum int) string {
	dir := filepath.Dir(rtw.basePath)
	baseName := filepath.Base(rtw.basePath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)

	// Format: <name>-<timestamp>-<rotation>.log
	// Example: mcp-trace-20240101-120000-1.log
	timestamp := time.Now().Format("20060102-150405")
	return filepath.Join(dir, fmt.Sprintf("%s-%s-%d%s", nameWithoutExt, timestamp, rotationNum, ext))
}

// cleanupOldFilesLocked removes old rotated files beyond maxFiles limit (must be called with mu held)
func (rtw *RollingTraceWriter) cleanupOldFilesLocked() error {
	dir := filepath.Dir(rtw.basePath)
	baseName := filepath.Base(rtw.basePath)
	ext := filepath.Ext(baseName)
	nameWithoutExt := strings.TrimSuffix(baseName, ext)

	// Pattern: <name>-<timestamp>-<rotation>.log
	pattern := nameWithoutExt + "-*-*" + ext

	// Find all rotated files matching the pattern
	matches, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		return errfmt.Newf("failed to glob rotated files").Wrap(err)
	}

	// Sort by modification time (oldest first)
	sort.Slice(matches, func(i, j int) bool {
		statI, errI := fileutil.Stat(matches[i])
		statJ, errJ := fileutil.Stat(matches[j])
		if errI != nil || errJ != nil {
			return false
		}
		return statI.ModTime().Before(statJ.ModTime())
	})

	// Delete oldest files if we exceed maxFiles
	if len(matches) > rtw.maxFiles {
		for i := 0; i < len(matches)-rtw.maxFiles; i++ {
			if err := fileutil.Remove(matches[i]); err != nil {
				// Log error but continue - don't fail cleanup
				continue
			}
		}
	}

	return nil
}

// Close implements io.Closer interface
func (rtw *RollingTraceWriter) Close() error {
	var closeErr error
	_ = concurrency.RunInLockWithLogger(
		&rtw.mu, LockNameRollingTraceWriterClose, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if rtw.currentFile != nil {
				closeErr = rtw.currentFile.Close()
				rtw.currentFile = nil
			}
			return nil
		},
	)
	return closeErr
}

// Sync flushes the current file to disk
func (rtw *RollingTraceWriter) Sync() error {
	var syncErr error
	_ = concurrency.RunInLockWithLogger(
		&rtw.mu, LockNameRollingTraceWriterSync, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			if rtw.currentFile != nil {
				syncErr = rtw.currentFile.Sync()
			}
			return nil
		},
	)
	return syncErr
}
