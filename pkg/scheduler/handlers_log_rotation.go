package scheduler

import (
	"context"
	"time"

	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/storage"
)

type LogRotatorHandler struct {
	projectRoot string
	logger      logging.Logger
}

// NewLogRotatorHandler creates a new log rotator handler
func NewLogRotatorHandler(projectRoot string, logger logging.Logger) LogRotatorHandlerInterface {
	return &LogRotatorHandler{projectRoot: projectRoot, logger: logger}
}

func (h *LogRotatorHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	rotator := NewLogRotator(h.projectRoot+"/.zqk/logs", h.logger)
	if err := rotator.Rotate(ctx, 7*24*time.Hour); err != nil {
		return err
	}

	// Stream Segment Garbage Collection
	for _, kind := range storage.StreamStorageEnabledKindsList() {
		_ = storage.CompactStreamRegistryForKind(h.projectRoot, kind)
	}
	storage.GCOrphanedStreamSegmentsForAllKinds(h.projectRoot)

	return nil
}
