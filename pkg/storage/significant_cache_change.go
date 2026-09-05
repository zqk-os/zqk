package storage

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"time"

	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// SignificantCacheChangePendingThreshold is the outstanding object-id-cache pending
// entry count that implies a bulk CAS mutation. The next system check must
// rebuild object-id-cache. It must NOT wipe the whole validation cache — only
// the pending IDs (same cadence as id-cache). Full validation clear stays
// break-glass (--clear-cache), state restore, and kind-wide invalidation.
//
// Single-object create/update must stay incremental (PRE_CHANGE_CHECKLIST §2–3).
//
// TRACK: REDACTED — remove when: claim fixture proves
// check auto refresh + pending-id invalidation after pending burst + state-restore.
const SignificantCacheChangePendingThreshold = 5

const (
	SignificantChangeReasonPendingBurst     = "object_id_cache_pending_burst"
	SignificantChangeReasonPendingDrain     = "object_id_cache_pending_drain"
	SignificantChangeReasonKindInvalidation = "object_id_cache_kind_invalidation"
	SignificantChangeReasonStateRestore     = "system_state_restore"
)

// SignificantChangePreservesValidationCache is true when the marker means
// rebuild object-id-cache / invalidate pending validation entries, not
// ClearCache of the entire validation state.
func SignificantChangePreservesValidationCache(reason string) bool {
	switch reason {
	case SignificantChangeReasonPendingBurst, SignificantChangeReasonPendingDrain:
		return true
	default:
		return false
	}
}

const significantCacheChangeFileName = "significant_cache_change.json"

// SignificantCacheChange marks that caches may be incoherent with process data
// (bulk restore, pending burst, kind-wide invalidation). System check consumes
// the marker: pending burst/drain refresh object-id-cache and invalidate those
// IDs in the validation cache; restore and kind-wide invalidation still apply
// --clear-cache + --refresh-cache.
type SignificantCacheChange struct {
	Version   string `json:"version"`
	Reason    string `json:"reason"`
	NotedAt   string `json:"noted_at"`
	Project   string `json:"project_root,omitempty"`
	PendingN  int    `json:"pending_count,omitempty"`
	Mutations int    `json:"mutation_count,omitempty"`
}

var (
	significantCacheChangeMu sync.Mutex
)

func significantCacheChangePath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.CacheDir, significantCacheChangeFileName)
}

// NoteSignificantCacheChange records that the next system check should rebuild
// object-id-cache. Validation cache is wiped only for reasons that do not
// preserve it (see SignificantChangePreservesValidationCache). Idempotent:
// later notes keep the latest reason/timestamp.
func NoteSignificantCacheChange(projectRoot, reason string) {
	NoteSignificantCacheChangeDetail(projectRoot, reason, 0, 0)
}

// NoteSignificantCacheChangeDetail is NoteSignificantCacheChange with optional counts.
func NoteSignificantCacheChangeDetail(projectRoot, reason string, pendingCount, mutationCount int) {
	if projectRoot == emptyValue || reason == emptyValue {
		return
	}
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = abs
	}
	significantCacheChangeMu.Lock()
	defer significantCacheChangeMu.Unlock()

	path := significantCacheChangePath(projectRoot)
	if err := fileutil.MkdirAll(filepath.Dir(path), paths.DirPerm755); err != nil {
		return
	}
	marker := SignificantCacheChange{
		Version:   "1.0.0",
		Reason:    reason,
		NotedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Project:   projectRoot,
		PendingN:  pendingCount,
		Mutations: mutationCount,
	}
	data, err := json.MarshalIndent(marker, "", "  ")
	if err != nil {
		return
	}
	data = append(data, '\n')
	_ = fileutil.WriteFile(path, data, paths.FilePerm644)
}

// PeekSignificantCacheChange returns the marker without consuming it.
func PeekSignificantCacheChange(projectRoot string) (SignificantCacheChange, bool) {
	if projectRoot == emptyValue {
		return SignificantCacheChange{}, false
	}
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = abs
	}
	significantCacheChangeMu.Lock()
	defer significantCacheChangeMu.Unlock()

	data, err := fileutil.ReadFile(significantCacheChangePath(projectRoot))
	if err != nil {
		return SignificantCacheChange{}, false
	}
	var marker SignificantCacheChange
	if err := json.Unmarshal(data, &marker); err != nil || marker.Reason == emptyValue {
		return SignificantCacheChange{}, false
	}
	return marker, true
}

// ConsumeSignificantCacheChange returns and removes the marker so a subsequent
// check does not keep forcing full clear+rebuild.
func ConsumeSignificantCacheChange(projectRoot string) (SignificantCacheChange, bool) {
	if projectRoot == emptyValue {
		return SignificantCacheChange{}, false
	}
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = abs
	}
	significantCacheChangeMu.Lock()
	defer significantCacheChangeMu.Unlock()

	path := significantCacheChangePath(projectRoot)
	data, err := fileutil.ReadFile(path)
	if err != nil {
		return SignificantCacheChange{}, false
	}
	var marker SignificantCacheChange
	if err := json.Unmarshal(data, &marker); err != nil || marker.Reason == emptyValue {
		_ = fileutil.Remove(path)
		return SignificantCacheChange{}, false
	}
	_ = fileutil.Remove(path)
	return marker, true
}

// ResetSignificantCacheChangeForTest clears the marker file (tests only).
func ResetSignificantCacheChangeForTest(projectRoot string) {
	if projectRoot == emptyValue {
		return
	}
	if abs, err := filepath.Abs(projectRoot); err == nil {
		projectRoot = abs
	}
	significantCacheChangeMu.Lock()
	defer significantCacheChangeMu.Unlock()
	_ = fileutil.Remove(significantCacheChangePath(projectRoot))
}
