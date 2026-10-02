package system

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/storage"
)

// emitIOQueueStateChangeEventViaCoordinator emits I/O queue state change events via the coordination system.
func emitIOQueueStateChangeEventViaCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	changeType string,
) {
	emitStateChangeEventViaCoordinator(
		ctx,
		projectRoot,
		storageProvider,
		fmt.Sprintf("io_queue_state_%s", changeType),
		fmt.Sprintf("I/O queue state change: %s", changeType),
		changeType,
		"io_queue_state",
		"io_queue_state_change",
		"io_queue_state_change_emit",
		fmt.Sprintf("emitting I/O queue state change %s", changeType),
	)
}
