// Package storage: stream (append-only segment) storage for high-volume kinds.
// Avoids CAS overhead by writing multiple records per segment file; uses change-journal pattern
// (delta-only fields) where applicable. See BYPASS_KIND_STORAGE.md, AUDIT_STREAM_FORMAT.md,
// INTERNAL_OBJECTS_AS_DEDICATED_WALS.md.

package storage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const (
	streamPathOffsetSep    = "::"           // segmentPath::offset for cache/registry
	legacyStreamFileSuffix = ".jsonl"       // Legacy suffix for existing stream segments
	streamFileSuffix       = "_stream.json" // New convention: <yyyy-mm-dd>_stream.json
)

// Stream storage uses one mutex per kind to allow concurrent appends to different kinds.
// sync.Map avoids a global mutex for the kind→mutex table (LoadOrStore is concurrency-safe).
var streamKindLocks sync.Map // kind string -> *sync.Mutex

func streamMutexForKind(kind string) *sync.Mutex {
	v, isLoaded := streamKindLocks.LoadOrStore(kind, &sync.Mutex{})
	_ = isLoaded
	return v.(*sync.Mutex)
}

// streamTimestampFields are field names stored as Unix seconds in segment JSONL to save bytes (DATA_STORAGE_PRODUCTION_ROADMAP §2).
// When read, they are decoded back to RFC3339 strings so instance validation (datetime pattern) passes.
var streamTimestampFields = []string{
	"created_at", "updated_at",
	"window_start", "window_end", "first_seen", "last_seen",
	"aggregation_window_start", "aggregation_window_end",
}

// encodeStreamRecordTimestamps converts timestamp fields in record from string/time to Unix seconds (int64) for compact storage.
func encodeStreamRecordTimestamps(record map[string]any) {
	for _, key := range streamTimestampFields {
		v, ok := record[key]
		if !ok {
			continue
		}
		var sec int64
		switch t := v.(type) {
		case string:
			parsed, err := time.Parse(time.RFC3339, t)
			if err != nil {
				continue
			}
			sec = parsed.Unix()
		case time.Time:
			sec = t.Unix()
		default:
			continue
		}
		record[key] = sec
	}
}

// decodeStreamRecordTimestamps converts timestamp fields that are numeric (Unix seconds) back to RFC3339 strings for downstream.
func decodeStreamRecordTimestamps(record map[string]any) {
	for _, key := range streamTimestampFields {
		v, ok := record[key]
		if !ok {
			continue
		}
		var sec int64
		switch t := v.(type) {
		case float64:
			sec = int64(t)
		case int64:
			sec = t
		case int:
			sec = int64(t)
		default:
			continue
		}
		record[key] = time.Unix(sec, 0).UTC().Format(time.RFC3339)
	}
}

// recordForStream reduces obj to the fields we persist for this kind (delta-only for change_journal, minimal for metrics).
// Field list comes from stream_delta_fields config when present, else built-in defaults. See stream_delta_config.go.
func recordForStream(projectRoot, kind string, obj map[string]any) map[string]any {
	fields := getStreamDeltaFieldsForKind(projectRoot, kind)
	if len(fields) == 0 {
		return obj
	}
	out := make(map[string]any)
	for _, key := range fields {
		if v, ok := obj[key]; ok {
			out[key] = v
		}
	}
	return out
}

// getSegmentPath returns the segment file path for the given kind and creation time (daily window).
// Dir comes from stream path resolver (cache built at pre-warm); filename follows the convention:
//
//	<yyyy-mm-dd>_stream.json
//
// The kind is expressed by the directory (.zqk/streams/<kind>/), so filenames stay short.
func getSegmentPath(projectRoot, kind string, createdAt time.Time) (string, error) {
	dir, err := GetStreamSegmentDir(projectRoot, kind)
	if err != nil {
		return "", err
	}
	date := zqktime.FormatLayoutUTC(createdAt, zqktime.LayoutDate)
	filename := fmt.Sprintf("%s_pid%d%s", date, os.Getpid(), streamFileSuffix)
	return filepath.Join(dir, filename), nil
}

// AppendToStream appends one record to the kind's segment file (daily window).
// Returns segment path and byte offset of the start of the written line (for later read by offset).
// Safe to call from multiple goroutines (per-kind mutex). Creates directory and file if needed.
// Returns ErrPathAliasNotInCache if path cache is not built—run path-cache check or pre-warm first.
func AppendToStream(projectRoot, kind, id string, obj map[string]any, createdAt time.Time) (segmentPath string, offset int64, err error) {
	if projectRoot == emptyValue || kind == emptyValue || id == emptyValue {
		return "", 0, errfmt.Errorf(ConstStreamStreamProjectrootKindAndIdRequired)
	}
	segmentPath, err = getSegmentPath(projectRoot, kind, createdAt)

	if err != nil {
		return "", 0, err
	}
	record := recordForStream(projectRoot, kind, obj)
	record[objects.FieldKeyID] = id
	record[objects.FieldKeyKind] = kind
	encodeStreamRecordTimestamps(record)

	// Apply semantic compression based on policy
	policy := GetCompressionPolicy(projectRoot, kind)
	if policy != nil && policy.CompressFieldKeys {
		registry := GetFieldRegistry(projectRoot)
		record = registry.TranslateFieldKeys(record)
	}

	line, err := json.Marshal(record)
	if err != nil {
		return "", 0, errfmt.Newf(ConstStreamStreamMarshal).Wrap(err)
	}
	line = append(line, '\n')

	mu := streamMutexForKind(kind)
	mu.Lock()
	defer mu.Unlock()

	dir := filepath.Dir(segmentPath)
	if err := fileutil.EnsureDir(dir); err != nil {
		return "", 0, errfmt.Newf("stream: mkdir").Wrap(err)
	}
	f, err := fileutil.OpenFile(segmentPath, fileutil.O_CREATE|fileutil.O_APPEND|fileutil.O_WRONLY, paths.FilePerm600)
	if err != nil {
		return "", 0, errfmt.Newf("stream: open").Wrap(err)
	}
	offset, err = f.Seek(0, io.SeekEnd)
	if err != nil {
		var _err_84019037 = f.Close()
		if _err_84019037 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84019037).Log()
		}
		return "", 0, errfmt.Newf("stream: seek").Wrap(err)
	}
	_, err = f.Write(line)
	if err != nil {
		var _err_84019151 = f.Close()
		if _err_84019151 != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84019151).Log()
		}
		return "", 0, errfmt.Newf("stream: write").Wrap(err)
	}
	if err := f.Close(); err != nil {
		return "", 0, errfmt.Newf("stream: close").Wrap(err)
	}
	return segmentPath, offset, nil
}

// ReadRecordAt reads a single JSONL record from segmentPath at the given byte offset.
// Used by Get/List when resolving stream-backed objects (segmentPath::offset).
func ReadRecordAt(segmentPath string, offset int64) (map[string]any, error) {
	f, err := fileutil.Open(segmentPath)
	if err != nil {
		return nil, errfmt.Newf(ConstStreamStreamReadOpen).Wrap(err)
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, errfmt.Newf(ConstStreamStreamReadSeek).Wrap(err)
	}
	dec := json.NewDecoder(f)
	var obj map[string]any
	if err := dec.Decode(&obj); err != nil {
		return nil, errfmt.Newf(ConstStreamStreamReadDecode).Wrap(err)
	}
	decodeStreamRecordTimestamps(obj)
	return obj, nil
}

// StreamPathAndOffset parses "segmentPath::offset" into path and offset. Returns ok false if not in stream format.
func StreamPathAndOffset(pathWithOffset string) (segmentPath string, offset int64, ok bool) {
	i := strings.LastIndex(pathWithOffset, streamPathOffsetSep)
	if i < 0 {
		return "", 0, false
	}
	segmentPath = pathWithOffset[:i]
	offsetStr := pathWithOffset[i+len(streamPathOffsetSep):]
	offset, err := strconv.ParseInt(offsetStr, 10, 64)
	if err != nil {
		return "", 0, false
	}
	return segmentPath, offset, true
}

// FormatStreamLocation returns "segmentPath::offset" for cache and registry.
func FormatStreamLocation(segmentPath string, offset int64) string {
	return segmentPath + streamPathOffsetSep + strconv.FormatInt(offset, 10)
}

// getStreamSegmentDir returns the directory containing segment files for the kind.
// Resolves from path alias cache (built at pre-warm). Used by CountStreamSegmentLines.
func getStreamSegmentDir(projectRoot, kind string) (string, error) {
	return GetStreamSegmentDir(projectRoot, kind)
}

// CountStreamSegmentLines returns the total number of JSONL lines (records) in all segment files for the kind.
// Does not parse JSON; counts newlines only. Used when high-volume cache is empty so Count() does not do unbounded full scans.
// Returns 0, nil if the segment dir does not exist or has no .jsonl files. Returns error if path cache is not built.
// CountStreamSegmentLines counts the total number of lines across all segment files for a kind.
// Does not parse JSON; counts newlines only. Used when high-volume cache is empty so Count() does not do unbounded full scans.
// Returns 0, nil if the segment dir does not exist or has no .jsonl files. Returns error if path cache is not built.
func CountStreamSegmentLines(ctx context.Context, projectRoot, kind string) (int, error) {
	if projectRoot == emptyValue || kind == emptyValue {
		return 0, nil
	}
	dir, err := getStreamSegmentDir(projectRoot, kind)
	if err != nil {
		return 0, err
	}
	entries, err := fileutil.ReadDir(dir)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return 0, nil
		}
		return 0, errfmt.Newf(ConstStreamStreamCountReadDir).Wrap(err)
	}

	var segments []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, streamFileSuffix) && !strings.HasSuffix(name, legacyStreamFileSuffix) {
			continue
		}
		segments = append(segments, filepath.Join(dir, name))
	}

	if len(segments) == 0 {
		return 0, nil
	}

	// Use parallel scan for millions of events (RECOMP-STREAM-001)
	maxWorkers := 16 // Sufficient parallelism for counting
	if len(segments) < maxWorkers {
		maxWorkers = len(segments)
	}

	workCh := make(chan string, len(segments))
	for _, s := range segments {
		workCh <- s
	}
	close(workCh)

	results := make(chan int, maxWorkers)
	var wg sync.WaitGroup

	bud := goroutinelabels.DefaultBudget()

	for i := 0; i < maxWorkers; i++ {
		builder := goroutinelabels.NewGoroutine(ConstStreamStreamCountSegment, ConstStreamCountingLinesInSegment).
			WithWaitGroup(&wg)
		if bud != nil {
			builder = builder.WithBudget(bud)
		}
		builder.StartSimple(func() {
			workerTotal := 0
			interrupt := concurrency.NewInterruptChecker(concurrency.DefaultInterruptCheckFrequency)
			for path := range workCh {
				if ctx.Err() != nil {
					break
				}
				f, err := fileutil.Open(path)
				if err != nil {
					continue
				}
				scanner := bufio.NewScanner(f)
				// Use a larger buffer for potentially large JSON records (up to 1MB)
				buf := make([]byte, 0, 1024*1024)
				scanner.Buffer(buf, 1024*1024)
				for scanner.Scan() {
					workerTotal++
					if interrupt.Check(ctx) != nil {
						break
					}
				}
				var _err_84023450 = f.Close()
				if _err_84023450 != nil {
					logging.Fluent(logging.GetLoggerFromProfile(

						// Deterministic wait for results (POL-CODE-004)
						string(pkgctx.ProfileSystem))).Error(ErrMsgSwallowedError, _err_84023450).Log()
				}
			}
			results <- workerTotal
		})
	}

	waitDone := make(chan struct{})
	goroutinelabels.NewGoroutine(ConstStreamStreamCountWaiter, ConstStreamWaitingForStreamCountSegments).
		StartSimple(func() {
			wg.Wait()
			close(results)
			close(waitDone)
		})

	total := 0
	select {
	case <-waitDone:
		for n := range results {
			total += n
		}
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	return total, nil
}
