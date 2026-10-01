package asynccheck

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/appledouble"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/when"
)

const (
	logMsgSkippedContentionReads = "Skipping object ID extraction reads due to contention (fallback path)"
	labelExtractRead             = "asynccheck_extract_read"
	descExtractRead              = "reading file content for ID extraction"
	logMsgTimeoutRead            = "Timeout reading file to extract ID"
	logMsgSkipDupObjectID        = "Skipping duplicate object ID during discovery"
	labelScanReadDir             = "scan_read_dir"
	descScanReadDirFmt           = "reading directory %s"
	labelScanCheckIndex          = "scan_check_index"
	descScanCheckIndexFmt        = "checking for index file %s"
	labelScanObjectFilesWalk     = "scan_object_files_walk"
	descScanObjectFilesWalkFmt   = "walking directory %s for kind %s"
	labelScanObjectFilesColl     = "scan_object_files_collect"
	descScanObjectFilesCollFmt   = "collecting files from walk for %s"
	logWarnScanDirTimeout        = "Directory scan timed out or was cancelled"
	yamlExtShort                 = ".yaml"
	ymlExtShort                  = ".yml"
)

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}(-\d{2})?$`)

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
			logging.Fluent(logger).Warn(logMsgSkippedContentionReads).
				String("skipped_reads", fmt.Sprintf("%d", skipped)).
				Kind(kind).
				Log()
		}
		return ""
	}

	resultChan := make(chan []byte, 1)
	builder := goroutinelabels.NewGoroutine(labelExtractRead, descExtractRead).
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
		logging.Fluent(logger).Debug(logMsgTimeoutRead).
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
			logging.Fluent(logger).Debug(logMsgSkipDupObjectID).
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

// ScanObjectFiles scans a directory for object files (legacy wrapper).
func ScanObjectFiles(dir, kind string) ([]ScannedFile, error) {
	ctx := pkgctx.NewSystemContext()
	return ScanObjectFilesWithContext(ctx, dir, kind, logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)), nil)
}

// ScanObjectFilesWithContext scans a directory for object files with context cancellation and timeout.
// When storageProvider is non-nil and unwraps to *storage.FileObjectStorage, uses cached CAS to avoid loading the full index.
func ScanObjectFilesWithContext(ctx context.Context, dir, kind string, logger logging.Logger, storageProvider storage.ObjectStorageProvider) ([]ScannedFile, error) {
	files, found, err := scanFromCASIndex(ctx, dir, kind, storageProvider)
	if err != nil {
		return nil, err
	}
	if found {
		return files, nil
	}
	return scanFromFilesystemWalk(ctx, dir, kind, logger)
}

func scanFromCASIndex(ctx context.Context, dir, kind string, storageProvider storage.ObjectStorageProvider) ([]ScannedFile, bool, error) {
	indexPath := filepath.Join(dir, fmt.Sprintf(".%s.index", kind))
	indexExists := false
	statDone := make(chan bool, 1)
	indexBud := goroutinelabels.DefaultBudget()
	indexBuilder := goroutinelabels.NewGoroutine(labelScanCheckIndex, fmt.Sprintf(descScanCheckIndexFmt, indexPath)).
		WithContext(ctx)
	if indexBud != nil {
		indexBuilder = indexBuilder.WithBudget(indexBud)
	}
	indexBuilder.StartSimple(func() {
		when.When(func() bool {
			_, statErr := fileutil.Stat(indexPath)
			return statErr == nil
		}).Then(func() {
			statDone <- true
		}).OrElse(func() {
			statDone <- false
		}).Run()
	})

	select {
	case indexExists = <-statDone:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-time.After(1 * time.Second):
		indexExists = false
	}

	if !indexExists {
		return nil, false, nil
	}

	var cas *storage.ContentAddressableStorage
	if fileStorage := storage.UnwrapToFileObjectStorage(storageProvider); fileStorage != nil {
		if c, err := fileStorage.GetContentAddressableStorage(kind); err == nil && c != nil {
			cas = c
		}
	}
	if cas == nil {
		cas = storage.OpenContentAddressableStorage(dir, kind)
	}

	mappings, err := cas.GetAllMappings()
	if err != nil || len(mappings) == 0 {
		return nil, false, err
	}

	readDirDone := make(chan []fileutil.DirEntry, 1)
	readDirBud := goroutinelabels.DefaultBudget()
	readDirBuilder := goroutinelabels.NewGoroutine(labelScanReadDir, fmt.Sprintf(descScanReadDirFmt, dir)).
		WithContext(ctx)
	if readDirBud != nil {
		readDirBuilder = readDirBuilder.WithBudget(readDirBud)
	}
	readDirBuilder.StartSimple(func() {
		entries, readErr := fileutil.ReadDir(dir)
		if readErr != nil {
			readDirDone <- nil
			return
		}
		readDirDone <- entries
	})

	var entries []fileutil.DirEntry
	select {
	case entries = <-readDirDone:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	case <-time.After(2 * time.Second):
		entries = []fileutil.DirEntry{}
	}

	bucketDirs := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() && datePattern.MatchString(e.Name()) {
			bucketDirs = append(bucketDirs, e.Name())
		}
	}

	files := make([]ScannedFile, 0, len(mappings))
	for objectID, hash := range mappings {
		hashFile := filepath.Join(dir, hash+yamlExtShort)
		if len(bucketDirs) > 0 {
			mostRecentBucket := bucketDirs[len(bucketDirs)-1]
			hashFile = filepath.Join(dir, mostRecentBucket, hash+yamlExtShort)
		}
		files = append(files, ScannedFile{
			ObjectID: objectID,
			Kind:     kind,
			Path:     hashFile,
		})
	}
	return files, true, nil
}

func scanFromFilesystemWalk(ctx context.Context, dir, kind string, logger logging.Logger) ([]ScannedFile, error) {
	walkCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	filesChan := make(chan ScannedFile, 100)
	walkErrChan := make(chan error, 1)

	walkBud := goroutinelabels.DefaultBudget()
	walkBuilder := goroutinelabels.NewGoroutine(labelScanObjectFilesWalk, fmt.Sprintf(descScanObjectFilesWalkFmt, dir, kind)).
		WithContext(walkCtx)
	if walkBud != nil {
		walkBuilder = walkBuilder.WithBudget(walkBud)
	}
	walkBuilder.StartSimple(func() {
		defer close(filesChan)
		walkErr := filepath.Walk(dir, func(path string, info fileutil.FileInfo, err error) error {
			select {
			case <-walkCtx.Done():
				return walkCtx.Err()
			default:
			}

			if err != nil || info.IsDir() || appledouble.SkipPathInTreeWalk(path) {
				return nil
			}

			if !strings.HasSuffix(path, yamlExtShort) && !strings.HasSuffix(path, ymlExtShort) {
				return nil
			}

			objectID := ExtractObjectIDFromFileWithContext(walkCtx, path, kind, logger)
			if objectID == "" {
				return nil
			}

			select {
			case filesChan <- ScannedFile{
				ObjectID: objectID,
				Kind:     kind,
				Path:     path,
			}:
			case <-walkCtx.Done():
				return walkCtx.Err()
			}

			return nil
		})
		walkErrChan <- walkErr
	})

	var files []ScannedFile
	collectDone := make(chan struct{})
	collectWalkBud := goroutinelabels.DefaultBudget()
	collectWalkBuilder := goroutinelabels.NewGoroutine(labelScanObjectFilesColl, fmt.Sprintf(descScanObjectFilesCollFmt, kind)).
		WithContext(walkCtx)
	if collectWalkBud != nil {
		collectWalkBuilder = collectWalkBuilder.WithBudget(collectWalkBud)
	}
	collectWalkBuilder.StartSimple(func() {
		for file := range filesChan {
			files = append(files, file)
		}
		close(collectDone)
	})

	select {
	case err := <-walkErrChan:
		<-collectDone
		return files, err
	case <-walkCtx.Done():
		logging.Fluent(logger).Warn(logWarnScanDirTimeout).
			String("dir", dir).
			Kind(kind).
			WithError(walkCtx.Err()).
			Log()
		select {
		case <-collectDone:
		case <-time.After(1 * time.Second):
		}
		return files, walkCtx.Err()
	}
}
