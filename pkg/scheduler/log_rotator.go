package scheduler

import (
	"context"
	"path/filepath"
	"time"

	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// LogRotator cleans up logs older than retention period
type LogRotator struct {
	logDir string
	logger logging.Logger
}

// NewLogRotator creates a new log rotator
func NewLogRotator(logDir string, logger logging.Logger) LogRotatorInterface {
	return &LogRotator{logDir: logDir, logger: logger}
}

func (r *LogRotator) Rotate(ctx context.Context, retention time.Duration) error {
	cutoff := time.Now().Add(-retention)
	return filepath.Walk(r.logDir, func(path string, info fileutil.FileInfo, err error) error { //nolint:gosec
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.ModTime().Before(cutoff) {
			return fileutil.Remove(path)
		}
		return nil
	})
}
