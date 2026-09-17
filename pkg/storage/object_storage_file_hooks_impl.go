package storage

import (
	"context"
	"fmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

func SetLifecycleHookHandler(handler func(context.Context, string, string, string, map[string]any) error) {
	lifecycleHookHandler = handler
}

func SetChangeNotificationHandler(handler func(context.Context, string, string, string, map[string]any) error) {
	changeNotificationHandler = handler
}

// executeChangeNotification invokes the change notification handler for create/update/delete (BLI-643)
// and dispatches the mutation shockwave across InvalidationShockwaveBus synchronously.
func executeChangeNotification(ctx context.Context, operation, kind, id string, objectData map[string]any) {
	executeChangeNotificationWithPath(ctx, operation, kind, id, "", objectData)
}

// executeChangeNotificationWithPath synchronously broadcasts the mutation event to InvalidationShockwaveBus
// and invokes changeNotificationHandler asynchronously.
func executeChangeNotificationWithPath(ctx context.Context, operation, kind, id, path string, objectData map[string]any) {
	executeChangeNotificationWithHashes(ctx, operation, kind, id, path, "", objectData)
}

// executeChangeNotificationWithHashes synchronously broadcasts the mutation event with explicit oldHash
// to InvalidationShockwaveBus and invokes changeNotificationHandler asynchronously.
func executeChangeNotificationWithHashes(ctx context.Context, operation, kind, id, path, oldHash string, objectData map[string]any) {
	broadcastInvalidationShockwaveWithOldHash(ctx, operation, kind, id, path, oldHash, objectData)

	if changeNotificationHandler == nil {
		return
	}
	// Run asynchronously so storage path is not blocked (same pattern as lifecycle trigger)
	bud := goroutinelabels.DefaultBudget()
	changeBuilder := goroutinelabels.NewGoroutine(ConstStreamChangeNotification, fmt.Sprintf("%s %s %s", operation, kind, id))
	if bud != nil {
		changeBuilder = changeBuilder.WithBudget(bud)
	}
	changeBuilder.StartSimple(func() {
		if err := changeNotificationHandler(ctx, operation, kind, id, objectData); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			StorageLog(logger).Debug(LogEventStorageObjectFileChangeNotifyHandlerErrDebug).
				String("operation", operation).
				Kind(kind).
				ObjectID(id).
				WithError(err).
				Log()
		}
	})
}
