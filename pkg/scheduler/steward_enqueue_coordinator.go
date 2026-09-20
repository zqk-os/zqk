// Package scheduler: coordinators that bridge the scheduler to data-cell stewards.
package scheduler

import (
	"context"

	"github.com/zqk-os/zqk/pkg/datacell"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
)

// StewardEnqueueCoordinator implements [CellCoordinator] by enqueuing maintenance jobs
// into the platform-specific steward queue (best-effort; usually .zqk/steward/*.jsonl).
type StewardEnqueueCoordinator struct {
	ProjectRoot string
	Logger      logging.Logger
	Profile     datacell.StorageProfile
}

// NewStewardEnqueueCoordinator binds a specific storage profile to the steward enqueue path.
func NewStewardEnqueueCoordinator(projectRoot string, profile datacell.StorageProfile, logger logging.Logger) (StewardEnqueueCoordinatorInterface, error) {
	if !profile.IsKnown() {
		return nil, errfmt.Errorf("steward enqueue coordinator: unknown storage_profile %q", profile)
	}
	return &StewardEnqueueCoordinator{
		ProjectRoot: projectRoot,
		Profile:     profile,
		Logger:      logger,
	}, nil
}

// NewStreamStewardEnqueueCoordinator binds [datacell.ProfileStream].
func NewStreamStewardEnqueueCoordinator(projectRoot string, logger logging.Logger) (StewardEnqueueCoordinatorInterface, error) {
	return NewStewardEnqueueCoordinator(projectRoot, datacell.ProfileStream, logger)
}

// NewLightFileStewardEnqueueCoordinator binds [datacell.ProfileLightFile].
func NewLightFileStewardEnqueueCoordinator(projectRoot string, logger logging.Logger) (StewardEnqueueCoordinatorInterface, error) {
	return NewStewardEnqueueCoordinator(projectRoot, datacell.ProfileLightFile, logger)
}

// NewCASEntityStewardEnqueueCoordinator binds [datacell.ProfileCASEntity].
func NewCASEntityStewardEnqueueCoordinator(projectRoot string, logger logging.Logger) (StewardEnqueueCoordinatorInterface, error) {
	return NewStewardEnqueueCoordinator(projectRoot, datacell.ProfileCASEntity, logger)
}

// Enqueue implements [StewardEnqueueCoordinatorInterface].
func (c *StewardEnqueueCoordinator) Enqueue(ctx context.Context, op datacell.MaintenanceOp) error {
	if c == nil {
		return nil
	}
	if err := datacell.EnqueueStewardMaintenance(ctx, c.ProjectRoot, c.Profile, op, c.Logger); err != nil {
		return err
	}
	return nil
}
