package storage

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/utils/fileutil"
)

// ObjectIDCachePendingOp is the pending true-up operation kind.
type ObjectIDCachePendingOp string

const (
	ObjectIDCachePendingOpUpdate     ObjectIDCachePendingOp = "update"
	ObjectIDCachePendingOpInvalidate ObjectIDCachePendingOp = "invalidate"
)

// ObjectIDCachePendingEntry records a CAS mutation that still needs object-id-cache true-up.
// TRACK: REDACTED — remove when: CRIT pending+drain verified + claim fixture exits 0.
type ObjectIDCachePendingEntry struct {
	ID       string                 `json:"id"`
	Kind     string                 `json:"kind,omitempty"`
	FilePath string                 `json:"file_path,omitempty"`
	Op       ObjectIDCachePendingOp `json:"op"`
	Reason   string                 `json:"reason,omitempty"`
	Gen      uint64                 `json:"gen"`
	NotedAt  string                 `json:"noted_at,omitempty"`
}

type objectIDCachePendingJournal struct {
	mu         sync.Mutex
	entries    map[string]ObjectIDCachePendingEntry
	gen        uint64
	loadErr    error
	persistErr error
}

var (
	pendingJournalsMu       sync.Mutex
	pendingJournals         = map[string]*objectIDCachePendingJournal{}
	pendingNilHandlerWarned atomic.Bool
)

const objectIDCachePendingFileName = "object-id-cache.pending.json"

func objectIDCachePendingPath(projectRoot string) string {
	return filepath.Join(projectRoot, ".zqk", "cache", objectIDCachePendingFileName)
}

func getObjectIDCachePendingJournal(projectRoot string) *objectIDCachePendingJournal {
	if projectRoot == emptyValue {
		return nil
	}
	pendingJournalsMu.Lock()
	defer pendingJournalsMu.Unlock()
	if j, ok := pendingJournals[projectRoot]; ok {
		return j
	}
	j := &objectIDCachePendingJournal{entries: make(map[string]ObjectIDCachePendingEntry)}
	if err := j.loadFromDisk(projectRoot); err != nil {
		j.loadErr = err
		logObjectIDCachePendingIO(LogEventStorageObjectIDCachePendingLoadFailedWarn, projectRoot, objectIDCachePendingPath(projectRoot), err)
	}
	pendingJournals[projectRoot] = j
	return j
}

func logObjectIDCachePendingIO(event, projectRoot, path string, err error) {
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Warn(event).
		ProjectRoot(projectRoot).
		Path(path).
		WithError(err).
		Log()
}

func (j *objectIDCachePendingJournal) loadFromDisk(projectRoot string) error {
	path := objectIDCachePendingPath(projectRoot)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return nil
		}
		return err
	}
	var file struct {
		Generation uint64                               `json:"generation"`
		Entries    map[string]ObjectIDCachePendingEntry `json:"entries"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}
	j.gen = file.Generation
	if file.Entries == nil {
		j.entries = make(map[string]ObjectIDCachePendingEntry)
	} else {
		j.entries = file.Entries
	}
	return nil
}

func (j *objectIDCachePendingJournal) persistLocked(projectRoot string) error {
	path := objectIDCachePendingPath(projectRoot)
	if j.loadErr != nil {
		logObjectIDCachePendingIO(LogEventStorageObjectIDCachePendingPersistSkippedLoadErrWarn, projectRoot, path, j.loadErr)
		return j.loadErr
	}
	if err := fileutil.EnsureDir(filepath.Dir(path)); err != nil {
		j.persistErr = err
		logObjectIDCachePendingIO(LogEventStorageObjectIDCachePendingPersistFailedWarn, projectRoot, path, err)
		return err
	}
	payload := struct {
		Generation uint64                               `json:"generation"`
		Entries    map[string]ObjectIDCachePendingEntry `json:"entries"`
	}{
		Generation: j.gen,
		Entries:    j.entries,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		j.persistErr = err
		logObjectIDCachePendingIO(LogEventStorageObjectIDCachePendingPersistFailedWarn, projectRoot, path, err)
		return err
	}
	if err := fileutil.WriteStandardFile(path, data); err != nil {
		j.persistErr = err
		logObjectIDCachePendingIO(LogEventStorageObjectIDCachePendingPersistFailedWarn, projectRoot, path, err)
		return err
	}
	j.persistErr = nil
	return nil
}

// NoteObjectIDCachePending records that object-id-cache must be trued for id.
// Draft-plane paths are refused (CAS-only). No-op when projectRoot/id empty.
func NoteObjectIDCachePending(projectRoot, op, id, kind, filePath, reason string) {
	if projectRoot == emptyValue || id == emptyValue {
		return
	}
	if filePath != emptyValue && IsObjectDraftPlanePath(projectRoot, filePath) {
		return
	}
	j := getObjectIDCachePendingJournal(projectRoot)
	if j == nil {
		return
	}
	pendingOp := ObjectIDCachePendingOp(op)
	if pendingOp != ObjectIDCachePendingOpUpdate && pendingOp != ObjectIDCachePendingOpInvalidate {
		pendingOp = ObjectIDCachePendingOpUpdate
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	j.gen++
	j.entries[id] = ObjectIDCachePendingEntry{
		ID:       id,
		Kind:     kind,
		FilePath: filePath,
		Op:       pendingOp,
		Reason:   reason,
		Gen:      j.gen,
		NotedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	pendingN := len(j.entries)
	if err := j.persistLocked(projectRoot); err != nil {
		return
	}
	// Burst pending ⇒ next system check must refresh object-id-cache and
	// invalidate those pending validation IDs (not wipe the whole validation cache).
	// TRACK: REDACTED
	if pendingN >= SignificantCacheChangePendingThreshold {
		NoteSignificantCacheChangeDetail(projectRoot, SignificantChangeReasonPendingBurst, pendingN, 0)
	}
}

// IsObjectIDCachePending reports whether id has an outstanding cache true-up.
func IsObjectIDCachePending(projectRoot, id string) bool {
	if projectRoot == emptyValue || id == emptyValue {
		return false
	}
	j := getObjectIDCachePendingJournal(projectRoot)
	if j == nil {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	_, ok := j.entries[id]
	return ok
}

// ListObjectIDCachePending returns a copy of outstanding pending entries.
func ListObjectIDCachePending(projectRoot string) []ObjectIDCachePendingEntry {
	j := getObjectIDCachePendingJournal(projectRoot)
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]ObjectIDCachePendingEntry, 0, len(j.entries))
	for _, e := range j.entries {
		out = append(out, e)
	}
	return out
}

// ClearObjectIDCachePending removes a pending entry after successful cache true-up.
func ClearObjectIDCachePending(projectRoot, id string) {
	if projectRoot == emptyValue || id == emptyValue {
		return
	}
	j := getObjectIDCachePendingJournal(projectRoot)
	if j == nil {
		return
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if _, ok := j.entries[id]; !ok {
		return
	}
	delete(j.entries, id)
	if err := j.persistLocked(projectRoot); err != nil {
		return
	}
}

// ObjectIDCachePendingLastIOError returns the last load or persist I/O error for this root.
// Used by tests and operators to detect a silent journal that failed closed.
func ObjectIDCachePendingLastIOError(projectRoot string) error {
	j := getObjectIDCachePendingJournal(projectRoot)
	if j == nil {
		return nil
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.persistErr != nil {
		return j.persistErr
	}
	return j.loadErr
}

// ObjectIDCachePendingGeneration returns the monotonic generation for the journal.
func ObjectIDCachePendingGeneration(projectRoot string) uint64 {
	j := getObjectIDCachePendingJournal(projectRoot)
	if j == nil {
		return 0
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.gen
}

// warnOnceNilCacheHandler logs once per process when CAS post-sync notes pending without a CLI handler.
func warnOnceNilCacheHandler(objectID, kind string) {
	if !pendingNilHandlerWarned.CompareAndSwap(false, true) {
		return
	}
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	StorageLog(logger).Warn(LogEventStorageObjectCASRegisterCachePostSyncFailedWarn).
		ObjectID(objectID).
		Kind(kind).
		String("reason", "cacheOperationHandler_nil_pending_journaled").
		Log()
}

// ResetObjectIDCachePendingForTest clears in-memory journals (tests only).
func ResetObjectIDCachePendingForTest() {
	pendingJournalsMu.Lock()
	defer pendingJournalsMu.Unlock()
	pendingJournals = map[string]*objectIDCachePendingJournal{}
	pendingNilHandlerWarned.Store(false)
}
