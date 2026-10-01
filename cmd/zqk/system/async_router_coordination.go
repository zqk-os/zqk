package system

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/systemcheck/asynccheck"
)

// emitAsyncRouterEventViaCoordinator emits async router events via the coordination system
func emitAsyncRouterEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider any,
	workerID string,
	eventType string,
	status string,
	workerCount int,
	processedCount int,
	failedCount int,
	duration time.Duration,
) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	asynccheck.EmitAsyncRouterEventViaCoordinator(
		ctx, projectRoot, storageProvider, workerID, eventType, status,
		workerCount, processedCount, failedCount, duration,
	)
}
