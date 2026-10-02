package system

import (
	"context"

	"github.com/zqk-os/zqk/pkg/cliapp"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/storage"
)

// createContextWithLoggingProfile delegates to the canonical cli.CreateContextWithLoggingProfile.
func createContextWithLoggingProfile(ctx context.Context, profile string) context.Context {
	return cli.CreateContextWithLoggingProfile(ctx, profile)
}

// setupSystemCoordinator bootstraps a context and coordinator for system operations.
// Returns (ctx, coordinator, true) if projectRoot is non-empty, or (ctx, nil, false) otherwise.
func setupSystemCoordinator(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	profile string,
	withMetrics bool,
) (context.Context, *coordination.Coordinator, bool) {
	projectRoot = ProjectRootOrResolveDot(projectRoot)
	if projectRoot == emptyValue {
		return ctx, nil, false
	}

	ctx = createContextWithLoggingProfile(ctx, profile)
	auditRouter := coordination.NewAuditRouter(projectRoot, storageProvider)

	var metricsRouter coordination.MetricsRouter
	if withMetrics && storageProvider != nil {
		metricsPipeline := metrics.MetricPipelineForProject(storageProvider, projectRoot)
		metricsRouter = coordination.NewMetricPipelineRouter(metricsPipeline)
	}

	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter,
		MetricsRouter:     metricsRouter,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	return ctx, coordinator, true
}
