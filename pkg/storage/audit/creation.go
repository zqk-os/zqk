package audit

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

var (
	eventCreationMu         sync.RWMutex
	eventCreationGoroutines = make(map[int64]int)
	globalFallbackDepth     int32
)

type auditCycleContextKey struct{}

// WithCreatingEvent returns a context marked as actively creating an audit event.
func WithCreatingEvent(ctx context.Context) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, auditCycleContextKey{}, struct{}{})
}

// IsCreatingEventContext reports whether either the context or the current goroutine
// is marked as actively creating an audit event.
func IsCreatingEventContext(ctx context.Context) bool {
	if ctx != nil && ctx.Value(auditCycleContextKey{}) != nil {
		return true
	}
	return IsCreatingEvent()
}

func curGoroutineID() int64 {
	var buf [64]byte
	n := runtime.Stack(buf[:], false)
	s := string(buf[:n])
	if strings.HasPrefix(s, "goroutine ") {
		rest := s[len("goroutine "):]
		idx := strings.IndexByte(rest, ' ')
		if idx > 0 {
			if id, err := strconv.ParseInt(rest[:idx], 10, 64); err == nil {
				return id
			}
		}
	}
	return 0
}

// IsCreatingEvent reports whether the current goroutine stack is already
// creating an audit event (cycle guard).
func IsCreatingEvent() bool {
	gid := curGoroutineID()
	if gid == 0 {
		return atomic.LoadInt32(&globalFallbackDepth) > 0
	}
	eventCreationMu.RLock()
	defer eventCreationMu.RUnlock()
	return eventCreationGoroutines[gid] > 0
}

// BeginEventCreation marks the start of audit event creation for the current goroutine.
// The returned function must be deferred.
func BeginEventCreation() func() {
	gid := curGoroutineID()
	if gid == 0 {
		atomic.AddInt32(&globalFallbackDepth, 1)
		return func() {
			atomic.AddInt32(&globalFallbackDepth, -1)
		}
	}
	eventCreationMu.Lock()
	eventCreationGoroutines[gid]++
	eventCreationMu.Unlock()
	return func() {
		eventCreationMu.Lock()
		if eventCreationGoroutines[gid] <= 1 {
			delete(eventCreationGoroutines, gid)
		} else {
			eventCreationGoroutines[gid]--
		}
		eventCreationMu.Unlock()
	}
}
