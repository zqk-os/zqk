package scheduler

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
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
	return filepath.Walk(r.logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() && info.ModTime().Before(cutoff) {
			return os.Remove(path)
		}
		return nil
	})
}
