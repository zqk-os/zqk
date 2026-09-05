package telemetry

import (
	"path/filepath"
	"strings"
	"time"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// CompactOldSegments removes stream and metric segment files older than maxAge.
// It scans the provided roots (e.g., .zqk/streams, .zqk/metrics) and their subdirectories.
func CompactOldSegments(roots []string, maxAge time.Duration) (int, error) {
	cutoff := time.Now().Add(-maxAge)
	count := 0

	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info fileutil.FileInfo, err error) error {
			if err != nil {
				return nil // Skip on error, keep walking
			}
			if info.IsDir() {
				return nil
			}

			// Check file extensions typically used for streams and metrics
			extOk := strings.HasSuffix(info.Name(), ".jsonl") ||
				strings.HasSuffix(info.Name(), ".json") ||
				strings.HasSuffix(info.Name(), ".chunk")

			if !extOk {
				return nil
			}

			if info.ModTime().Before(cutoff) {
				if removeErr := fileutil.Remove(path); removeErr == nil {
					count++
				}
			}
			return nil
		})
		if err != nil {
			return count, err
		}
	}

	return count, nil
}
