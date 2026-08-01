package coordination

import (
	"github.com/lanceman/zqk/pkg/metrics"
	"github.com/lanceman/zqk/pkg/storage"
)

// NewCLINotifierWithCoordinator creates a CLINotifier with mandatory coordinator integration
// This helper function is in the coordination package to avoid import cycles
// projectRoot and storageProvider are required - coordinator is mandatory
func NewCLINotifierWithCoordinator(
	verbose, quiet bool,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	operationID, operationType, profile string,
) storage.OperationNotifier {
	if projectRoot == emptyValue {
		panic("NewCLINotifierWithCoordinator requires projectRoot")
	}
	if storageProvider == nil {
		panic("NewCLINotifierWithCoordinator requires storageProvider")
	}

	// Create coordinator with routers (mandatory)
	auditRouter := NewStorageAuditRouter(projectRoot, storageProvider)
	metricsPipeline := metrics.MetricPipelineForProject(storageProvider, projectRoot)
	metricsRouter := NewMetricPipelineRouter(metricsPipeline)

	coordinator := NewCoordinator(CoordinatorConfig{
		LoggingRouter:     &DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     metricsRouter,
		OperationalRouter: &DefaultOperationalRouter{},
	})

	// Create progress helper
	helper := NewProgressHelper(
		coordinator,
		projectRoot,
		operationID,
		operationType,
		profile,
	)

	// Create adapter
	adapter := NewCLINotifierAdapter(helper)

	// Create CLINotifier with adapter (mandatory)
	return storage.NewCLINotifier(verbose, quiet, adapter)
}
