// Package lifecycle provides a lifecycle event WAL and listener that accumulate
// events, evaluate transition criteria, and trigger transitions (cache-then-disk).
//
// See docs/architecture/LIFECYCLE_EVENT_LISTENER_AND_CRITERIA.md.
package lifecycle

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"
	"time"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
	"github.com/lanceman/zqk/pkg/walutil"
)

const (
	lifecycleWALFileName       = "lifecycle_events.wal"
	lifecycleWALCheckpointExt  = ".checkpoint"
	legacyLifecycleWALFileName = "lifecycle_events" // pre-rename; migrated on first open
	// maxLifecycleWALLineSize limits line length when reading (single JSON object per line).
	maxLifecycleWALLineSize  = 64 * 1024
	errWALNeedsProjectRoot   = "lifecycle event WAL requires non-empty project root"
	walDirPerm               = 0o755
	errRenameLifecycleWALFmt = "migrate lifecycle WAL: rename %s -> %s: %w"
	errCreateLifecycleDirFmt = "create lifecycle WAL dir: %w"
	scopeEntrySeparator      = ','
	scopeKeyValueSeparator   = '='

	// Idle poll backoff for lifecycle WAL consumers (listener / synthesis / QA).
	WALIdlePollMin = 100 * time.Millisecond
	WALIdlePollMax = 2 * time.Second
)

// NextWALIdlePoll doubles prev up to WALIdlePollMax (floor WALIdlePollMin).
func NextWALIdlePoll(prev time.Duration) time.Duration {
	if prev < WALIdlePollMin {
		return WALIdlePollMin
	}
	next := prev * 2
	if next > WALIdlePollMax {
		return WALIdlePollMax
	}
	return next
}

// EventType identifies the kind of lifecycle event.
type EventType string

const (
	// EventTypeStatusTransition records an object status change (kind, id, from_status, to_status).
	EventTypeStatusTransition EventType = "status_transition"
	// EventTypeCriterionSatisfied records that a criterion was satisfied (criterion_id, scope).
	EventTypeCriterionSatisfied EventType = "criterion_satisfied"
)

// LifecycleEvent is one durable record in the lifecycle event WAL.
type LifecycleEvent struct {
	Seq         int64             `json:"seq"`
	Ts          time.Time         `json:"ts"`
	EventType   EventType         `json:"event_type"`
	Kind        string            `json:"kind,omitempty"`
	ID          string            `json:"id,omitempty"`
	FromStatus  string            `json:"from_status,omitempty"`
	ToStatus    string            `json:"to_status,omitempty"`
	CriterionID string            `json:"criterion_id,omitempty"`
	Scope       map[string]string `json:"scope,omitempty"`
}

// scopeKey returns a stable string key for scope (for dedup and rule matching).
func (e *LifecycleEvent) scopeKey() string {
	if len(e.Scope) == 0 {
		return emptyValue
	}
	// Deterministic key: sorted keys, k=v pairs
	return scopeMapToKey(e.Scope)
}

func scopeMapToKey(m map[string]string) string {
	if len(m) == 0 {
		return emptyValue
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b []byte
	for _, k := range keys {
		if len(b) > 0 {
			b = append(b, scopeEntrySeparator)
		}
		b = append(b, k...)
		b = append(b, scopeKeyValueSeparator)
		b = append(b, m[k]...)
	}
	return string(b)
}

// LifecycleEventWAL is an append-only WAL for lifecycle events.
// Thread-safe: Append and Sync may be called concurrently; Replay is for a single reader.
type LifecycleEventWAL struct {
	w           *walutil.JSONLineWAL[LifecycleEvent]
	projectRoot string
}

// migrateLifecycleWALToCanonicalNames renames legacy lifecycle_events / lifecycle_events.checkpoint to
// lifecycle_events.wal / lifecycle_events.wal.checkpoint if the canonical files do not exist. Idempotent.
func migrateLifecycleWALToCanonicalNames(projectRoot string) error {
	if projectRoot == emptyValue {
		return nil
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir)
	canonical := filepath.Join(dir, lifecycleWALFileName)
	legacy := filepath.Join(dir, legacyLifecycleWALFileName)
	if _, err := fileutil.Stat(canonical); err == nil {
		return nil
	}
	if _, err := fileutil.Stat(legacy); err != nil {
		return nil
	}
	if err := fileutil.Rename(legacy, canonical); err != nil {
		return errfmt.Errorf(errRenameLifecycleWALFmt, legacyLifecycleWALFileName, lifecycleWALFileName, err)
	}
	ckLegacy := legacy + lifecycleWALCheckpointExt
	ckCanonical := canonical + lifecycleWALCheckpointExt
	if _, err := fileutil.Stat(ckLegacy); err == nil {
		_ = fileutil.Rename(ckLegacy, ckCanonical)
	}
	return nil
}

// NewLifecycleEventWAL creates or opens the lifecycle event WAL under projectRoot/.zqk/wal/lifecycle_events.wal.
// Migrates legacy lifecycle_events / lifecycle_events.checkpoint to canonical names if present.
func NewLifecycleEventWAL(projectRoot string) (*LifecycleEventWAL, error) {
	if projectRoot == emptyValue {
		return nil, errors.New(errWALNeedsProjectRoot)
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.WalDir)
	if err := fileutil.MkdirAll(dir, walDirPerm); err != nil {
		return nil, errfmt.Errorf(errCreateLifecycleDirFmt, err)
	}
	if err := migrateLifecycleWALToCanonicalNames(projectRoot); err != nil {
		return nil, err
	}
	jw, err := walutil.OpenJSONLineWAL[LifecycleEvent](projectRoot, lifecycleWALFileName, lifecycleWALCheckpointExt, maxLifecycleWALLineSize)
	if err != nil {
		return nil, err
	}
	return &LifecycleEventWAL{w: jw, projectRoot: projectRoot}, nil
}

// Append appends one event and assigns Seq. Caller may call Sync for durability.
func (w *LifecycleEventWAL) Append(ev *LifecycleEvent) error {
	if ev == nil {
		return nil
	}
	copy := *ev
	return w.w.Append(func(seq int64, ts time.Time) LifecycleEvent {
		copy.Seq = seq
		copy.Ts = ts
		return copy
	}, maxLifecycleWALLineSize)
}

// Sync flushes the buffer and syncs the file to disk.
func (w *LifecycleEventWAL) Sync() error {
	return w.w.Sync()
}

// Close closes the WAL file. No further Append or Sync after Close.
func (w *LifecycleEventWAL) Close() error {
	if w.w == nil {
		return nil
	}
	return w.w.Close()
}

// ReplayFrom reads events from the WAL starting at seqAfter (exclusive).
// Prefer ReplayFromCursor when retaining a byte offset across polls.
func (w *LifecycleEventWAL) ReplayFrom(seqAfter int64, fn func(ev *LifecycleEvent) error) error {
	_, err := w.ReplayFromCursor(walutil.ReplayCursor{Seq: seqAfter}, fn)
	return err
}

// ReplayFromCursor resumes at cursor.Offset and advances seq/offset for the next poll.
func (w *LifecycleEventWAL) ReplayFromCursor(cursor walutil.ReplayCursor, fn func(ev *LifecycleEvent) error) (walutil.ReplayCursor, error) {
	stats, err := walutil.ReplayFromCursor[LifecycleEvent](
		w.w.Path(),
		maxLifecycleWALLineSize,
		cursor,
		func(b []byte) (*LifecycleEvent, error) {
			var ev LifecycleEvent
			if err := json.Unmarshal(b, &ev); err != nil {
				return nil, err
			}
			return &ev, nil
		},
		func(ev *LifecycleEvent) int64 { return ev.Seq },
		fn,
	)
	return stats.Cursor, err
}

// Path returns the WAL file path (for checkpoint storage and tests).
func (w *LifecycleEventWAL) Path() string {
	return w.w.Path()
}

// CheckpointPath returns the checkpoint file path.
func (w *LifecycleEventWAL) CheckpointPath() string {
	return w.w.CheckpointPath()
}
