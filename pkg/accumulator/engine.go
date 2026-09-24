package accumulator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/lifecycle"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/walutil"
)

// Engine orchestrates zero-cost reading, atomic serialization, WAL tailing,
// and debounced asynchronous circuit-breaker recovery for an Accumulator.
type Engine[T any] struct {
	spec          AccumulatorSpec
	acc           Accumulator[T]
	mu            sync.RWMutex
	isReconciling uint32
	lastUpdated   time.Time
}

// NewEngine creates and validates a new generic Accumulator Engine.
func NewEngine[T any](spec AccumulatorSpec, acc Accumulator[T]) (*Engine[T], error) {
	if acc == nil {
		return nil, fmt.Errorf("accumulator implementation cannot be nil")
	}

	name := spec.Name
	if name == "" {
		name = acc.Name()
	}
	if name == "" {
		return nil, fmt.Errorf("accumulator name must not be empty")
	}
	spec.Name = name

	if spec.SchemaVersion == "" {
		spec.SchemaVersion = DefaultSchemaVersion
	}
	if spec.StalenessTolerance <= 0 {
		spec.StalenessTolerance = DefaultStalenessTolerance
	}
	if spec.RebuildTimeout <= 0 {
		spec.RebuildTimeout = DefaultRebuildTimeout
	}
	if spec.PollInterval <= 0 {
		spec.PollInterval = DefaultPollInterval
	}

	if spec.StoragePath == "" {
		if spec.ProjectRoot == "" {
			spec.StoragePath = filepath.Join(paths.ProjectDataDir, paths.StateDir, fmt.Sprintf("%s_lite.json", name))
		} else {
			spec.StoragePath = filepath.Join(spec.ProjectRoot, paths.ProjectDataDir, paths.StateDir, fmt.Sprintf("%s_lite.json", name))
		}
	}

	return &Engine[T]{
		spec: spec,
		acc:  acc,
	}, nil
}

// Spec returns the configured AccumulatorSpec.
func (e *Engine[T]) Spec() AccumulatorSpec {
	return e.spec
}

// StoragePath returns the canonical path to the materialized JSON file.
func (e *Engine[T]) StoragePath() string {
	return e.spec.StoragePath
}

// SetLastUpdated explicitly overrides the watermark timestamp.
func (e *Engine[T]) SetLastUpdated(t time.Time) {
	e.mu.Lock()
	e.lastUpdated = t
	e.mu.Unlock()
}

// LastUpdated returns the timestamp of the last recorded state mutation or save.
func (e *Engine[T]) LastUpdated() time.Time {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.lastUpdated
}

// BuildEnvelope renders the current domain state into a standardized Envelope.
func (e *Engine[T]) BuildEnvelope() *Envelope[T] {
	e.mu.RLock()
	payload := e.acc.BuildPayload()
	matAt := e.lastUpdated
	e.mu.RUnlock()

	if matAt.IsZero() {
		matAt = time.Now().UTC()
	}

	return &Envelope[T]{
		SchemaVersion:  e.spec.SchemaVersion,
		MaterializedAt: matAt,
		Payload:        payload,
	}
}

// SaveToLiteFile atomically flushes the current state to the configured storage path.
func (e *Engine[T]) SaveToLiteFile() error {
	envelope := e.BuildEnvelope()

	targetPath := e.spec.StoragePath
	if err := fileutil.MkdirAll(filepath.Dir(targetPath), paths.DirPerm755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	data, err := json.MarshalIndent(envelope, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", targetPath, time.Now().UnixNano())
	if err := fileutil.WriteFile(tmpFile, data, paths.FilePerm644); err != nil {
		return fmt.Errorf("write temp lite file: %w", err)
	}

	if err := fileutil.Rename(tmpFile, targetPath); err != nil {
		_ = fileutil.Remove(tmpFile)
		return fmt.Errorf("rename lite file: %w", err)
	}

	e.mu.Lock()
	e.lastUpdated = time.Time{}
	e.mu.Unlock()

	return nil
}

// LoadFromLiteFile reads the materialized envelope from disk without accessing storage.
// It supports dual-format unmarshaling:
// 1. Standard Envelope[T] wrapper (schema_version, materialized_at, payload, etc.)
// 2. Legacy flat JSON payload (where domain fields exist at the root level without "payload").
// For flat files, it deserializes directly into T and synthesizes an Envelope[T] wrapper,
// preserving the file's modification timestamp (or materialized_at if present).
func (e *Engine[T]) LoadFromLiteFile() (*Envelope[T], error) {
	targetPath := e.spec.StoragePath
	data, err := fileutil.ReadFile(targetPath)
	if err != nil {
		return nil, err
	}

	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, fmt.Errorf("unmarshal lite file: %w", err)
	}

	// Format 1: Canonical Envelope wrapper
	if rawPayload, ok := rawMap[objects.FieldKeyPayload]; ok && len(rawPayload) > 0 && !bytes.Equal(rawPayload, []byte("null")) {
		var envelope Envelope[T]
		if err := json.Unmarshal(data, &envelope); err == nil {
			return &envelope, nil
		}
	}

	// Format 2: Legacy flat payload
	var payload T
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal lite file (neither envelope nor flat payload): %w", err)
	}

	var schemaVer string
	if rawVer, ok := rawMap[objects.FieldKeySchemaVersion]; ok {
		_ = json.Unmarshal(rawVer, &schemaVer)
	}
	if schemaVer == "" {
		schemaVer = e.spec.SchemaVersion
	}

	var matAt time.Time
	if rawMat, ok := rawMap["materialized_at"]; ok {
		_ = json.Unmarshal(rawMat, &matAt)
	}
	if matAt.IsZero() {
		if fi, err := fileutil.Stat(targetPath); err == nil {
			matAt = fi.ModTime().UTC()
		} else {
			matAt = time.Now().UTC()
		}
	}

	return &Envelope[T]{
		SchemaVersion:  schemaVer,
		MaterializedAt: matAt,
		Payload:        payload,
	}, nil
}

// isQuiescentSince returns true if the lifecycle WAL exists and has not had any
// event appends since matAt (within a 1s clock skew margin).
func (e *Engine[T]) isQuiescentSince(matAt time.Time) bool {
	if e.spec.ProjectRoot == "" || matAt.IsZero() {
		return false
	}
	walPath := filepath.Join(e.spec.ProjectRoot, paths.ProjectDataDir, paths.WalDir, "lifecycle_events.wal")
	fi, err := fileutil.Stat(walPath)
	if err != nil {
		return false
	}
	return !fi.ModTime().After(matAt.Add(1 * time.Second))
}

// GetOrRecoverPayload implements the Zero Hot-Path Scan Invariant.
// Hot path reads the pre-computed lite file in sub-5ms.
// If missing or stale, it serves available state with degraded flags and dispatches
// a non-blocking background reconciler to rebuild without delaying the caller.
func (e *Engine[T]) GetOrRecoverPayload(ctx context.Context, sp storage.ObjectStorageProvider) (*Envelope[T], error) {
	envelope, err := e.LoadFromLiteFile()

	// Case 1: Missing or corrupted file -> Cold boot non-blocking bootstrap
	if err != nil || envelope == nil {
		skeleton := &Envelope[T]{
			SchemaVersion:  e.spec.SchemaVersion,
			MaterializedAt: time.Now().UTC(),
			Stale:          true,
			Recovering:     true,
			DegradedReason: "cold_boot_materialized_projection_missing",
			Payload:        e.acc.DefaultPayload(),
		}
		e.TriggerAsyncRebuild(sp)
		return skeleton, nil
	}

	// Case 2: Watermark staleness exceeded -> Serve last-known-good + non-blocking recovery
	age := time.Since(envelope.MaterializedAt)
	if age > e.spec.StalenessTolerance {
		// Inactivity-resilient watermark touch:
		// If the lifecycle WAL has not received new mutations since envelope.MaterializedAt,
		// the graph is fully synchronized and quiescent. Refresh watermark without triggering
		// a redundant storage scan across CAS objects.
		if e.isQuiescentSince(envelope.MaterializedAt) {
			envelope.MaterializedAt = time.Now().UTC()
			envelope.Stale = false
			envelope.Recovering = false
			envelope.DegradedReason = ""
			e.SetLastUpdated(envelope.MaterializedAt)
			_ = e.SaveToLiteFile()
			return envelope, nil
		}

		envelope.Stale = true
		envelope.Recovering = true
		envelope.DegradedReason = fmt.Sprintf("materialized projection watermark exceeds tolerance (%s > %s)", age.Round(time.Second), e.spec.StalenessTolerance)
		e.TriggerAsyncRebuild(sp)
		return envelope, nil
	}

	// Case 3: Fresh projection (<5ms hot path)
	envelope.Stale = false
	envelope.Recovering = false
	envelope.DegradedReason = ""
	return envelope, nil
}

// TriggerAsyncRebuild launches a debounced background scan to rebuild the projection.
// Returns true if a new rebuild was initiated, false if one is already in flight.
func (e *Engine[T]) TriggerAsyncRebuild(sp storage.ObjectStorageProvider) bool {
	if sp == nil {
		return false
	}

	// Debounce: Ensure only one reconciliation runs concurrently
	if !atomic.CompareAndSwapUint32(&e.isReconciling, 0, 1) {
		return false // Already in-flight
	}

	goroutinelabels.NewGoroutine(
		fmt.Sprintf("%s-accumulator-reconciler", e.spec.Name),
		fmt.Sprintf("asynchronously rebuilding %s accumulator projection", e.spec.Name),
	).StartSimple(func() {
		defer atomic.StoreUint32(&e.isReconciling, 0)

		bgCtx, cancel := context.WithTimeout(context.Background(), e.spec.RebuildTimeout)
		defer cancel()

		e.mu.Lock()
		scanErr := e.acc.ScanFromStorage(bgCtx, sp)
		e.mu.Unlock()

		if scanErr != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
				Error(fmt.Sprintf("async %s projection rebuild failed", e.spec.Name), scanErr).
				Log()
			return
		}

		if saveErr := e.SaveToLiteFile(); saveErr != nil {
			logging.Fluent(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).
				Error(fmt.Sprintf("async %s projection save failed", e.spec.Name), saveErr).
				Log()
			return
		}
	})

	return true
}

// IsReconciling returns true if a background reconciliation scan is actively running.
func (e *Engine[T]) IsReconciling() bool {
	return atomic.LoadUint32(&e.isReconciling) == 1
}

// WaitUntilIdle blocks until no background rebuild is running, or timeout elapses.
// Tests must call this before t.TempDir cleanup: GetOrRecoverPayload may still be
// writing the lite file after the hot-path return.
func (e *Engine[T]) WaitUntilIdle(timeout time.Duration) bool {
	if e == nil {
		return true
	}
	deadline := time.Now().Add(timeout)
	for e.IsReconciling() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	return !e.IsReconciling()
}

// SubscribeWAL starts an incremental background listener on lifecycle_events.wal.
func (e *Engine[T]) SubscribeWAL(ctx context.Context, updateCh chan<- struct{}) {
	if e.spec.ProjectRoot == "" {
		return
	}

	wal, err := lifecycle.GetOrCreateLifecycleWAL(e.spec.ProjectRoot)
	if err != nil {
		return
	}

	var cursor walutil.ReplayCursor
	heartbeatInterval := e.spec.StalenessTolerance / 2
	if heartbeatInterval <= 0 {
		heartbeatInterval = time.Minute
	}
	lastHeartbeat := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		newEvents := 0
		mutated := false

		newCursor, err := wal.ReplayFromCursor(cursor, func(ev *lifecycle.LifecycleEvent) error {
			if ev == nil {
				return nil
			}
			newEvents++
			e.mu.Lock()
			if e.acc.ApplyEvent(ev) {
				mutated = true
			}
			e.mu.Unlock()
			return nil
		})

		if err == nil {
			cursor = newCursor
		}

		now := time.Now()
		if mutated {
			e.SetLastUpdated(now)
			_ = e.SaveToLiteFile()
			lastHeartbeat = now
			if updateCh != nil {
				select {
				case updateCh <- struct{}{}:
				default:
				}
			}
		} else if now.Sub(lastHeartbeat) >= heartbeatInterval {
			// Inactivity-resilient watermark touch:
			// If WAL has been fully replayed and no new mutating events arrived,
			// touch the lite file watermark to prevent false staleness and redundant CAS scans.
			e.SetLastUpdated(now)
			_ = e.SaveToLiteFile()
			lastHeartbeat = now
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(e.spec.PollInterval):
		}
	}
}

// StartBackgroundWALSubscriber helper to launch SubscribeWAL with goroutinelabels tracking.
func (e *Engine[T]) StartBackgroundWALSubscriber(ctx context.Context, updateCh chan<- struct{}) {
	goroutinelabels.NewGoroutine(
		fmt.Sprintf("%s-accumulator-wal-subscriber", e.spec.Name),
		fmt.Sprintf("subscribing to lifecycle events for %s accumulator", e.spec.Name),
	).StartSimple(func() {
		e.SubscribeWAL(ctx, updateCh)
	})
}
