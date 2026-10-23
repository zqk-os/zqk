package scheduler

import (
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// StorageForTest exposes the factory storage provider for facade / registry builders.
func (f *HandlerFactory) StorageForTest() storagepkg.ObjectStorageProvider {
	if f == nil {
		return nil
	}
	return f.storage
}

// SpecLoaderForTest exposes the spec loader.
func (f *HandlerFactory) SpecLoaderForTest() *objects.SpecLoader {
	if f == nil {
		return nil
	}
	return f.specLoader
}

// LifecycleLoaderForTest exposes the lifecycle loader.
func (f *HandlerFactory) LifecycleLoaderForTest() *objects.LifecycleLoader {
	if f == nil {
		return nil
	}
	return f.lifecycleLoader
}

// ProjectRootForTest exposes the project root.
func (f *HandlerFactory) ProjectRootForTest() string {
	if f == nil {
		return ""
	}
	return f.projectRoot
}

// ObjectIDCacheBuilderForTest exposes the optional object ID cache builder.
func (f *HandlerFactory) ObjectIDCacheBuilderForTest() ObjectIDCacheBuilder {
	if f == nil {
		return nil
	}
	return f.objectIDCacheBuilder
}

// ValidationScannerForTest exposes the optional validation scanner.
func (f *HandlerFactory) ValidationScannerForTest() ValidationScanner {
	if f == nil {
		return nil
	}
	return f.validationScanner
}

// LoggerForTest exposes the factory logger.
func (f *HandlerFactory) LoggerForTest() logging.Logger {
	if f == nil {
		return nil
	}
	return f.logger
}

// NotificationContextForTest exposes the notification context.
func (f *HandlerFactory) NotificationContextForTest() NotificationContextInterface {
	if f == nil {
		return nil
	}
	return f.notificationContext
}

// AsyncRouterForTest exposes the async router.
func (f *HandlerFactory) AsyncRouterForTest() *transceiver.AsyncRouter {
	if f == nil {
		return nil
	}
	return f.asyncRouter
}

// SchedulerForTest exposes the bound scheduler (may be nil before BindScheduler).
func (f *HandlerFactory) SchedulerForTest() *Scheduler {
	if f == nil {
		return nil
	}
	return f.scheduler
}

// CreateMetricsCollectionHandlerForTest exposes createMetricsCollectionHandler for facade registry.
func (f *HandlerFactory) CreateMetricsCollectionHandlerForTest(job *ScheduledJob) JobHandler {
	return f.createMetricsCollectionHandler(job)
}

// GetCachedTestCommandDetectorForTest exposes the shared TestCommandDetector cache.
func (f *HandlerFactory) GetCachedTestCommandDetectorForTest() TestCommandDetector {
	return f.getCachedTestCommandDetector()
}

// CreateMetricsCollectionHandlerForTest exposes createMetricsCollectionHandler for facade registry.
func (f *HandlerFactory) CreateMetricsCollectionHandlerForTest(job *ScheduledJob) JobHandler {
	return f.createMetricsCollectionHandler(job)
}

// GetCachedTestCommandDetectorForTest exposes the shared TestCommandDetector cache.
func (f *HandlerFactory) GetCachedTestCommandDetectorForTest() TestCommandDetector {
	return f.getCachedTestCommandDetector()
}
