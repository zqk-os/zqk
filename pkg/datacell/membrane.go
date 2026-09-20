package datacell

import (
	"context"
)

// Membrane and coordinator contracts (REQ-DATACELL-001, DATA_CELL_MODEL “Cell manager / nucleus”).
//
// Read models: correlate rebuilds with the spec origin plane invalidation generation — the same
// monotonic idea as objects.SpecLoader.SpecCacheRevision and materialized spec_index
// BuilderSpecCacheRevision / GlobalSpecCacheRevision (see SPEC_ORIGIN_PLANE.md). After power loss,
// durable storage (streams, CAS, WAL) is authoritative; in-memory graphs reload and compare
// BuiltAtSpecCacheRevision to the current loader revision before serving queries.
//
// Write path: a CellCoordinator is the enqueue surface for stewardship work (compaction, summary
// refresh, cache invalidation). Implementations may bridge to scheduler jobs, in-process queues, or
// no-ops until wired.

// CellReadModel is a derived, possibly in-memory view that must be rebuilt when the spec cache
// generation advances. Implementations store the revision they were built against.
type CellReadModel interface {
	BuiltAtSpecCacheRevision() uint64
}

// ReadModelIsStale reports whether the read model should rebuild before serving queries, given the
// current SpecLoader.SpecCacheRevision() (or equivalent process-wide generation).
func ReadModelIsStale(m CellReadModel, currentSpecCacheRevision uint64) bool {
	if m == nil {
		return true
	}
	return m.BuiltAtSpecCacheRevision() != currentSpecCacheRevision
}

// MaxSpecRevision returns the larger of two revision counters (e.g. when comparing builder vs
// global fields on a materialized spec_index snapshot).
func MaxSpecRevision(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}

// MaintenanceOp names work queued by a CellCoordinator. Values are semantic; metrics and scheduler
// wiring map them to concrete jobs later.
const (
	MaintenanceOpInvalidateCache = "invalidate_cache"
	MaintenanceOpRefreshSummary  = "refresh_summary"
	MaintenanceOpCompactSegments = "compact_segments"
	// MaintenanceOpStreamStewardKind queues observability for one stream-backed kind after
	// PostRetentionStreamKindStewardship (segments) or runtime-delta stewardship (see STREAM_KIND_STEWARDSHIP.md).
	MaintenanceOpStreamStewardKind = "stream_steward_kind"
	MaintenanceOpRetentionSweep    = "retention_sweep"
)

// MaintenanceOp is a single stewardship action for one cell boundary.
type MaintenanceOp struct {
	Name string
	// Detail is optional context for logs/metrics (not an RPC payload).
	Detail string
}

// CellCoordinator serializes or schedules maintenance for one cell so scans, cache population,
// and user commands do not fight arbitrary direct nucleus I/O (REQ-DATACELL-001).
type CellCoordinator interface {
	Enqueue(ctx context.Context, op MaintenanceOp) error
}

// CellMembrane is the public boundary for one logical cell: profile + coordinator; nucleus paths
// (segments, CAS, overlays) stay behind implementations that honor this API.
type CellMembrane interface {
	StorageProfile() StorageProfile
	Coordinator() CellCoordinator
}

// NoopCellCoordinator discards maintenance ops (for tests and until a cell is fully wired).
// For observable enqueue without a queue yet, wrap with [LoggingCellCoordinator] (optional Inner chain).
type NoopCellCoordinator struct{}

// Enqueue implements [CellCoordinator] as a no-op.
func (NoopCellCoordinator) Enqueue(ctx context.Context, op MaintenanceOp) error {
	_, _ = ctx, op
	return nil
}

// MinimalMembrane is a concrete [CellMembrane] holding profile and coordinator. If Coord is nil,
// Coordinator returns a [NoopCellCoordinator].
type MinimalMembrane struct {
	Profile StorageProfile
	Coord   CellCoordinator
}

// StorageProfile implements [CellMembrane].
func (m *MinimalMembrane) StorageProfile() StorageProfile {
	if m == nil {
		return ""
	}
	return m.Profile
}

// Coordinator implements [CellMembrane].
func (m *MinimalMembrane) Coordinator() CellCoordinator {
	if m == nil || m.Coord == nil {
		return NoopCellCoordinator{}
	}
	return m.Coord
}
