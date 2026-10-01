package asynccheck

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Limit concurrent per-file content reads during discovery fallback.
var extractObjectIDReadSem = make(chan struct{}, 32)

// Track how often we skip reads due to contention.
var extractObjectIDSkippedReads uint64

// ScannedFile represents an object file identified during system check discovery.
type ScannedFile struct {
	ObjectID string `json:"object_id"`
	Kind     string `json:"kind"`
	Path     string `json:"path"`
}

// CalculateMaxConcurrentWorkers calculates the worker pool ceiling for discovery.
func CalculateMaxConcurrentWorkers(numKinds int) int {
	maxConcurrent := runtime.NumCPU() * 2
	if maxConcurrent > numKinds {
		maxConcurrent = numKinds
	}
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	return maxConcurrent
}

// IsHexString reports whether s consists entirely of hexadecimal characters.
func IsHexString(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

// ExtractObjectIDFromFile extracts object ID from file path or content (legacy system context).
func ExtractObjectIDFromFile(filePath, kind string) string {
	ctx := pkgctx.NewSystemContext()
	return ExtractObjectIDFromFileWithContext(ctx, filePath, kind, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)))
}

// ExtractObjectIDFromFileWithContext extracts object ID from file path or content with timeout.
func ExtractObjectIDFromFileWithContext(ctx context.Context, filePath, kind string, logger logging.Logger) string {
	base := filepath.Base(filePath)
	isCASFile := len(base) == 69 && strings.HasSuffix(base, ".yaml") && IsHexString(base[:64])
	if !isCASFile {
		if ext := filepath.Ext(base); ext != "" {
			id := base[:len(base)-len(ext)]
			if id != "" && len(id) < 64 {
				return id
			}
		}
	}
	return extractIDFromContentWithSem(ctx, filePath, kind, logger)
}

func extractIDFromContentWithSem(ctx context.Context, filePath, kind string, logger logging.Logger) string {
	select {
	case extractObjectIDReadSem <- struct{}{}:
	case <-ctx.Done():
		return ""
	case <-time.After(50 * time.Millisecond):
		skipped := atomic.AddUint64(&extractObjectIDSkippedReads, 1)
		if skipped == 1 || skipped%1000 == 0 {
			logging.Fluent(logger).Warn("Skipping object ID extraction reads due to contention (fallback path)").
				String("skipped_reads", fmt.Sprintf("%d", skipped)).
				Kind(kind).
				Log()
		}
		return ""
	}

	resultChan := make(chan []byte, 1)
	builder := goroutinelabels.NewGoroutine("asynccheck_extract_read", "reading file content for ID extraction").
		WithContext(ctx)
	if bud := goroutinelabels.DefaultBudget(); bud != nil {
		builder = builder.WithBudget(bud)
	}
	builder.StartSimple(func() {
		defer func() { <-extractObjectIDReadSem }()
		data, err := fileutil.ReadFile(filePath)
		if err == nil {
			resultChan <- data
		} else {
			resultChan <- nil
		}
	})

	select {
	case data := <-resultChan:
		if len(data) == 0 {
			return ""
		}
		return parseIDFromYAMLBytes(data)
	case <-time.After(2 * time.Second):
		logging.Fluent(logger).Debug("Timeout reading file to extract ID").
			File(filePath).
			Kind(kind).
			Log()
		return ""
	case <-ctx.Done():
		return ""
	}
}

func parseIDFromYAMLBytes(data []byte) string {
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "id:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 {
				id := strings.TrimSpace(parts[1])
				if id != "" {
					return id
				}
			}
		}
	}
	return ""
}

// FilterFilesByIDs returns only scanned files matching targetIDs (or all files if targetIDs is empty).
func FilterFilesByIDs(files []ScannedFile, targetIDs []string) []ScannedFile {
	if len(targetIDs) == 0 {
		return files
	}
	targetSet := make(map[string]bool, len(targetIDs))
	for _, id := range targetIDs {
		targetSet[id] = true
	}
	filtered := make([]ScannedFile, 0, len(files))
	for _, f := range files {
		if targetSet[f.ObjectID] {
			filtered = append(filtered, f)
		}
	}
	return filtered
}

// DeduplicateFilesByObjectID removes duplicate entries holding the same ObjectID.
func DeduplicateFilesByObjectID(files []ScannedFile, logger logging.Logger) []ScannedFile {
	seen := make(map[string]bool, len(files))
	deduped := make([]ScannedFile, 0, len(files))
	for _, f := range files {
		if f.ObjectID == "" {
			deduped = append(deduped, f)
			continue
		}
		if seen[f.ObjectID] {
			logging.Fluent(logger).Debug("Skipping duplicate object ID during discovery").
				String("object_id", f.ObjectID).
				Kind(f.Kind).
				Log()
			continue
		}
		seen[f.ObjectID] = true
		deduped = append(deduped, f)
	}
	return deduped
}

// CollectDiscoveryResults drains filesChan and aggregates all batches into a single slice.
func CollectDiscoveryResults(filesChan <-chan []ScannedFile) []ScannedFile {
	var allFiles []ScannedFile
	for batch := range filesChan {
		allFiles = append(allFiles, batch...)
	}
	return allFiles
}
