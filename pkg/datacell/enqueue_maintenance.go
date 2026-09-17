package datacell

import (
	"context"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// StewardEnqueueDetailMaxBytes caps optional MaintenanceOp.Detail on the steward JSONL queue (line size bound;
// aligns with scheduler cache-invalidation stewardship detail truncation).
const StewardEnqueueDetailMaxBytes = 512

// Error format strings for unknown StorageProfile (package consts avoid AST dry_violation magic-string hits).
const (
	errFmtEnqueueUnknownStorageProfile     = "enqueue steward maintenance: unknown storage_profile %q"
	errFmtCoordinatorUnknownStorageProfile = "steward maintenance coordinator: unknown storage_profile %q"
)

func truncateStewardEnqueueDetail(detail string) string {
	if len(detail) <= StewardEnqueueDetailMaxBytes {
		return detail
	}
	return detail[:StewardEnqueueDetailMaxBytes]
}

// EnqueueStewardMaintenance persists one steward enqueue JSONL record for profile and op — the pkg/datacell side
// of [CellCoordinator].Enqueue when the persistence backend is steward_enqueue.jsonl (same backing store as
// pkg/scheduler.StewardEnqueueCoordinator). Use from stewardship packages that must not depend on pkg/scheduler
// (e.g. pkg/storage). Optional logger emits the same stable event as the scheduler coordinator ([LogEventStewardEnqueue]).
func EnqueueStewardMaintenance(ctx context.Context, projectRoot string, profile StorageProfile, op MaintenanceOp, logger logging.Logger) error {
	if ctx != nil {
		_ = ctx // reserved for future deadlines / tracing on coordinator implementations
	}
	if !profile.IsKnown() {
		return errfmt.Errorf(errFmtEnqueueUnknownStorageProfile, profile)
	}
	op.Detail = truncateStewardEnqueueDetail(op.Detail)
	rec := BuildStewardEnqueueRecord(profile, op)
	if err := AppendStewardEnqueueRecord(projectRoot, rec); err != nil {
		return err
	}
	if logger != nil {
		path := StewardEnqueueJSONLPath(projectRoot)
		logging.Fluent(logger).Info(LogEventStewardEnqueue).
			StorageProfileName(string(profile)).
			String("op", op.Name).
			String("detail", op.Detail).
			Path(path).
			Log()
	}
	return nil
}

// StewardMaintenanceCoordinator implements [CellCoordinator] by delegating to [EnqueueStewardMaintenance]
// for a fixed storage profile (same JSONL contract as pkg/scheduler.StewardEnqueueCoordinator without Inner chaining).
type StewardMaintenanceCoordinator struct {
	ProjectRoot string
	Profile     StorageProfile
	Logger      logging.Logger
}

// NewStewardMaintenanceCoordinator returns a [StewardMaintenanceCoordinator] for profile, or an error when profile is unknown.
func NewStewardMaintenanceCoordinator(projectRoot string, profile StorageProfile, logger logging.Logger) (*StewardMaintenanceCoordinator, error) {
	if !profile.IsKnown() {
		return nil, errfmt.Errorf(errFmtCoordinatorUnknownStorageProfile, profile)
	}
	return &StewardMaintenanceCoordinator{ProjectRoot: projectRoot, Profile: profile, Logger: logger}, nil
}

// Enqueue implements [CellCoordinator].
func (c *StewardMaintenanceCoordinator) Enqueue(ctx context.Context, op MaintenanceOp) error {
	if c == nil {
		return nil
	}
	return EnqueueStewardMaintenance(ctx, c.ProjectRoot, c.Profile, op, c.Logger)
}
