// Package lifecycle: process-level registry for lifecycle event WAL per project root.
// Enables the lifecycle hook (in root) to append events without holding storage.

package lifecycle

import (
	"sync"

	"github.com/lanceman/zqk/pkg/errfmt"
)

var (
	walCache   = make(map[string]*LifecycleEventWAL)
	walCacheMu sync.RWMutex
)

// GetOrCreateLifecycleWAL returns the lifecycle event WAL for projectRoot, creating it if needed.
// Caller must not hold locks that could block WAL append. Close is not required for process lifetime;
// the process typically exits after CLI/scheduler run.
func GetOrCreateLifecycleWAL(projectRoot string) (*LifecycleEventWAL, error) {
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("lifecycle WAL requires non-empty project root")
	}
	walCacheMu.RLock()
	w := walCache[projectRoot]
	walCacheMu.RUnlock()
	if w != nil {
		return w, nil
	}
	walCacheMu.Lock()
	defer walCacheMu.Unlock()
	if w := walCache[projectRoot]; w != nil {
		return w, nil
	}
	w, err := NewLifecycleEventWAL(projectRoot)
	if err != nil {
		return nil, err
	}
	walCache[projectRoot] = w
	return w, nil
}

// AppendStatusTransition appends a status_transition event to the lifecycle WAL for projectRoot.
// No-op if projectRoot is empty or WAL cannot be created (e.g. one-shot CLI without .zqk).
func AppendStatusTransition(projectRoot, kind, id, fromStatus, toStatus string) {
	if projectRoot == emptyValue {
		return
	}
	wal, err := GetOrCreateLifecycleWAL(projectRoot)
	if err != nil {
		return
	}
	ev := &LifecycleEvent{
		EventType:  EventTypeStatusTransition,
		Kind:       kind,
		ID:         id,
		FromStatus: fromStatus,
		ToStatus:   toStatus,
	}
	_ = wal.Append(ev)
	_ = wal.Sync()
}
