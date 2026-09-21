package fileutil

import (
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/zqkenv"
)

// TRACK: BLI-PERF-FILEUTIL-METRICS-001 / /

// IOOp identifies an atomic I/O operation tracked by the metrics collector.
type IOOp string

const (
	OpRead    IOOp = "read"
	OpStat    IOOp = "stat"
	OpExists  IOOp = "exists"
	OpWrite   IOOp = "write"
	OpRename  IOOp = "rename"
	OpRemove  IOOp = "remove"
	OpMkdir   IOOp = "mkdir"
	OpReadDir IOOp = "readdir"
)

var allOps = []IOOp{
	OpRead,
	OpStat,
	OpExists,
	OpWrite,
	OpRename,
	OpRemove,
	OpMkdir,
	OpReadDir,
}

// PathClass classifies file paths coarsely without retaining full path strings.
type PathClass string

const (
	ClassProcess PathClass = "process"
	ClassState   PathClass = "state"
	ClassTmp     PathClass = "tmp"
	ClassOther   PathClass = "other"
)

var allClasses = []PathClass{
	ClassProcess,
	ClassState,
	ClassTmp,
	ClassOther,
}

// IOStats stores aggregated metrics for a specific slice of operations.
type IOStats struct {
	Count     int64 `json:"count"`
	Bytes     int64 `json:"bytes"`
	ElapsedNS int64 `json:"elapsed_ns"`
}

// IOMetricsSnapshot captures a point-in-time snapshot of fileutil I/O metrics.
type IOMetricsSnapshot struct {
	Timestamp      time.Time             `json:"timestamp"`
	Enabled        bool                  `json:"enabled"`
	TotalOps       int64                 `json:"total_ops"`
	TotalBytes     int64                 `json:"total_bytes"`
	TotalElapsedNS int64                 `json:"total_elapsed_ns"`
	ByOp           map[IOOp]IOStats      `json:"by_op"`
	ByClass        map[PathClass]IOStats `json:"by_class"`
	Breakdown      map[string]IOStats    `json:"breakdown"`
}

// IOCallback is an optional listener invoked on recorded I/O operations.
type IOCallback func(op IOOp, class PathClass, bytes int64, duration time.Duration, success bool)

type bucketAtomics struct {
	count     int64
	bytes     int64
	elapsedNS int64
}

const (
	numOps     = 8
	numClasses = 4
	numResults = 2 // 0 = error, 1 = success
	numBuckets = numOps * numClasses * numResults
)

var (
	ioMetricsEnabled int32
	ioBuckets        [numBuckets]bucketAtomics
	callbackMu       sync.RWMutex
	nextCBID         uint64
	ioCallbacks      = make(map[uint64]IOCallback)
)

func init() {
	envVal := strings.TrimSpace(zqkenv.FileutilMetrics().Get())
	switch envVal {
	case "1", "true", "TRUE":
		atomic.StoreInt32(&ioMetricsEnabled, 1)
		return
	case "0", "false", "FALSE":
		atomic.StoreInt32(&ioMetricsEnabled, 0)
		return
	}

	// Default off under test root or test runs; on for daemon and interactive CLI
	if zqkenv.IsInTest() || zqkenv.TestRoot().Get() != "" {
		atomic.StoreInt32(&ioMetricsEnabled, 0)
	} else {
		atomic.StoreInt32(&ioMetricsEnabled, 1)
	}
}

// SetIOMetricsEnabled enables or disables fileutil I/O metrics collection.
func SetIOMetricsEnabled(enabled bool) {
	if enabled {
		atomic.StoreInt32(&ioMetricsEnabled, 1)
	} else {
		atomic.StoreInt32(&ioMetricsEnabled, 0)
	}
}

// IsIOMetricsEnabled returns true if I/O metrics collection is active.
func IsIOMetricsEnabled() bool {
	return atomic.LoadInt32(&ioMetricsEnabled) == 1
}

// RegisterIOCallback registers a callback invoked on I/O events, returning an unregister func.
func RegisterIOCallback(cb IOCallback) func() {
	if cb == nil {
		return func() {}
	}
	callbackMu.Lock()
	id := nextCBID
	nextCBID++
	ioCallbacks[id] = cb
	callbackMu.Unlock()

	return func() {
		callbackMu.Lock()
		delete(ioCallbacks, id)
		callbackMu.Unlock()
	}
}

// ClassifyPath returns the coarse PathClass for a given path without retaining it.
func ClassifyPath(path string) PathClass {
	if path == "" {
		return ClassOther
	}
	clean := filepath.ToSlash(path)
	pDataDir := projectDataDirName()
	if strings.Contains(clean, "/"+pDataDir+"/process") || strings.HasPrefix(clean, pDataDir+"/process") ||
		strings.Contains(clean, "/.zqk/process") || strings.HasPrefix(clean, ".zqk/process") {
		return ClassProcess
	}
	if strings.Contains(clean, "/"+pDataDir+"-state") || strings.HasPrefix(clean, pDataDir+"-state") ||
		strings.Contains(clean, "/"+pDataDir+"/state") || strings.HasPrefix(clean, pDataDir+"/state") ||
		strings.Contains(clean, "/.zqk-state") || strings.HasPrefix(clean, ".zqk-state") ||
		strings.Contains(clean, "/.zqk/state") || strings.HasPrefix(clean, ".zqk/state") {
		return ClassState
	}
	if strings.Contains(clean, "/tmp") || strings.Contains(clean, "\\tmp") ||
		strings.HasPrefix(clean, "/tmp") || strings.Contains(clean, ".tmp") ||
		strings.HasPrefix(filepath.Base(clean), ".tmp") {
		return ClassTmp
	}
	return ClassOther
}

func opIndex(op IOOp) int {
	switch op {
	case OpRead:
		return 0
	case OpStat:
		return 1
	case OpExists:
		return 2
	case OpWrite:
		return 3
	case OpRename:
		return 4
	case OpRemove:
		return 5
	case OpMkdir:
		return 6
	case OpReadDir:
		return 7
	default:
		return 0
	}
}

func classIndex(class PathClass) int {
	switch class {
	case ClassProcess:
		return 0
	case ClassState:
		return 1
	case ClassTmp:
		return 2
	case ClassOther:
		return 3
	default:
		return 3
	}
}

func bucketIndex(op IOOp, class PathClass, success bool) int {
	res := 0
	if success {
		res = 1
	}
	return (opIndex(op)*numClasses+classIndex(class))*numResults + res
}

// RecordIO records metrics for a completed filesystem operation.
func RecordIO(op IOOp, path string, bytes int64, duration time.Duration, success bool) {
	if atomic.LoadInt32(&ioMetricsEnabled) == 0 {
		return
	}
	class := ClassifyPath(path)
	idx := bucketIndex(op, class, success)
	b := &ioBuckets[idx]

	atomic.AddInt64(&b.count, 1)
	if bytes > 0 {
		atomic.AddInt64(&b.bytes, bytes)
	}
	if ns := duration.Nanoseconds(); ns > 0 {
		atomic.AddInt64(&b.elapsedNS, ns)
	}

	callbackMu.RLock()
	var cbs []IOCallback
	if len(ioCallbacks) > 0 {
		cbs = make([]IOCallback, 0, len(ioCallbacks))
		for _, cb := range ioCallbacks {
			cbs = append(cbs, cb)
		}
	}
	callbackMu.RUnlock()
	for _, cb := range cbs {
		cb(op, class, bytes, duration, success)
	}
}

func nowIfMetricsEnabled() time.Time {
	if atomic.LoadInt32(&ioMetricsEnabled) == 1 {
		return time.Now()
	}
	return time.Time{}
}

func recordIOOp(op IOOp, path string, bytes int64, t0 time.Time, err error) {
	if t0.IsZero() || atomic.LoadInt32(&ioMetricsEnabled) == 0 {
		return
	}
	RecordIO(op, path, bytes, time.Since(t0), err == nil)
}

// ResetIOMetrics zeroes all atomic I/O counters.
func ResetIOMetrics() {
	for i := range ioBuckets {
		atomic.StoreInt64(&ioBuckets[i].count, 0)
		atomic.StoreInt64(&ioBuckets[i].bytes, 0)
		atomic.StoreInt64(&ioBuckets[i].elapsedNS, 0)
	}
}

// GetIOMetricsSnapshot produces an immutable point-in-time aggregation of I/O counters.
func GetIOMetricsSnapshot() IOMetricsSnapshot {
	snap := IOMetricsSnapshot{
		Timestamp: time.Now().UTC(),
		Enabled:   IsIOMetricsEnabled(),
		ByOp:      make(map[IOOp]IOStats, numOps),
		ByClass:   make(map[PathClass]IOStats, numClasses),
		Breakdown: make(map[string]IOStats, numBuckets),
	}

	for _, op := range allOps {
		snap.ByOp[op] = IOStats{}
	}
	for _, class := range allClasses {
		snap.ByClass[class] = IOStats{}
	}

	for _, op := range allOps {
		for _, class := range allClasses {
			for _, success := range []bool{false, true} {
				idx := bucketIndex(op, class, success)
				b := &ioBuckets[idx]
				count := atomic.LoadInt64(&b.count)
				bytesVal := atomic.LoadInt64(&b.bytes)
				elapsed := atomic.LoadInt64(&b.elapsedNS)

				if count == 0 && bytesVal == 0 && elapsed == 0 {
					continue
				}

				statusStr := "error"
				if success {
					statusStr = "success"
				}
				key := string(op) + ":" + string(class) + ":" + statusStr
				snap.Breakdown[key] = IOStats{
					Count:     count,
					Bytes:     bytesVal,
					ElapsedNS: elapsed,
				}

				snap.TotalOps += count
				snap.TotalBytes += bytesVal
				snap.TotalElapsedNS += elapsed

				opStats := snap.ByOp[op]
				opStats.Count += count
				opStats.Bytes += bytesVal
				opStats.ElapsedNS += elapsed
				snap.ByOp[op] = opStats

				classStats := snap.ByClass[class]
				classStats.Count += count
				classStats.Bytes += bytesVal
				classStats.ElapsedNS += elapsed
				snap.ByClass[class] = classStats
			}
		}
	}

	return snap
}
