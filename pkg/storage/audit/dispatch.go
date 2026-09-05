package audit

import (
	"context"
	"maps"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
)

// DispatchDeps is the storage-owned wiring Dispatch needs. Buffer, IDs, and
// store implementations stay in package storage.
type DispatchDeps struct {
	Store           EventStore
	Buffer          Buffer
	IDs             IDAllocator
	Metrics         CreationRecorder
	UsedCAS         bool
	IsAlreadyExists func(error) bool
}

// DispatchResult is the best-effort outcome of buffer-or-persist. Callers log
// and update caches; Dispatch never fails the originating write.
type DispatchResult struct {
	Buffered          bool
	BufferAddErr      error
	BufferBuildErr    error
	ImmediateBuildErr error
	Persist           PersistResult
	Instance          map[string]any
}

// Dispatch tries aggregation, then EventStore persist with ID retry.
func Dispatch(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	builder *bldr_instance_v1.AuditEventInstanceBuilder,
	auditID string,
	options *EventOptions,
	deps DispatchDeps,
) DispatchResult {
	var out DispatchResult
	if builder == nil || options == nil {
		return out
	}

	if deps.Buffer != nil && deps.Buffer.ShouldAggregate(BufferCheckMap(options)) {
		instance, err := builder.Build()
		if err != nil {
			out.BufferBuildErr = err
			return out
		}
		eventMap := make(map[string]any, len(instance))
		maps.Copy(eventMap, instance)
		if err := deps.Buffer.AddEvent(eventMap); err != nil {
			out.BufferAddErr = err
			out.Instance = instance
		} else {
			RecordBuffered(ctx, deps.Metrics, options.EventType)
			if options.OnBuffered != nil {
				options.OnBuffered()
			}
			out.Buffered = true
			out.Instance = instance
			return out
		}
	}

	instance := out.Instance
	if instance == nil {
		built, err := builder.Build()
		if err != nil {
			out.ImmediateBuildErr = err
			return out
		}
		instance = built
		out.Instance = instance
	}
	start := time.Now()
	out.Persist = PersistWithIDRetry(ctx, deps.Store, secCtx, instance, deps.IsAlreadyExists, retryInstanceWithNewID(instance, deps.IDs, auditID))
	RecordPersistOutcome(ctx, deps.Metrics, options.EventType, deps.UsedCAS, time.Since(start), out.Persist)
	return out
}

// DispatchKind is the create-path branch after Dispatch. Callers still log
// BufferAddErr separately because add-fail falls through to persist.
type DispatchKind int

const (
	DispatchBufferBuildFailed DispatchKind = iota
	DispatchImmediateBuildFailed
	DispatchBuffered
	DispatchRetrySucceeded
	DispatchPersisted
	DispatchFailed
)

// ClassifyDispatch picks the CreateAuditEventWithBuilder outcome.
func ClassifyDispatch(r DispatchResult) DispatchKind {
	if r.BufferBuildErr != nil {
		return DispatchBufferBuildFailed
	}
	if r.ImmediateBuildErr != nil {
		return DispatchImmediateBuildFailed
	}
	if r.Buffered {
		return DispatchBuffered
	}
	if r.Persist.Retried && r.Persist.Err == nil {
		return DispatchRetrySucceeded
	}
	if r.Persist.Err == nil || r.Persist.AlreadyExists {
		return DispatchPersisted
	}
	return DispatchFailed
}

// PersistLogKind is how CreateAuditEventWithBuilder logs a persist error.
type PersistLogKind int

const (
	PersistLogNone PersistLogKind = iota
	PersistLogDuplicate
	PersistLogError
)

// ClassifyPersistLog is the duplicate-vs-error branch after persist.
func ClassifyPersistLog(r DispatchResult) PersistLogKind {
	if r.Persist.Err == nil {
		return PersistLogNone
	}
	if r.Persist.AlreadyExists {
		return PersistLogDuplicate
	}
	return PersistLogError
}

// CacheableEventID is the instance id to write into the high-volume cache.
// Empty when the create path should not touch the cache (buffer, retry, fail).
func CacheableEventID(r DispatchResult) string {
	if ClassifyDispatch(r) != DispatchPersisted || r.Instance == nil {
		return ""
	}
	if r.Persist.Err != nil && !r.Persist.AlreadyExists {
		return ""
	}
	return objects.GetString(r.Instance, FieldID)
}

func retryInstanceWithNewID(instance map[string]any, ids IDAllocator, currentID string) func() (map[string]any, error) {
	return func() (map[string]any, error) {
		if ids == nil || instance == nil {
			return nil, nil
		}
		newID, err := ids.GenerateNextID()
		if err != nil || newID == currentID {
			return nil, err
		}
		next := maps.Clone(instance)
		next[FieldID] = newID
		return next, nil
	}
}
