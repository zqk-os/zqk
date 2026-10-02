package system

import (
	"context"

	"github.com/zqk-os/zqk/pkg/coordination"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/metrics"
	"github.com/zqk-os/zqk/pkg/storage"
)

// createContextWithLoggingProfile creates a context with LoggingContext embedded from profile string
// This ensures coordinator logging events respect --context profile settings
func createContextWithLoggingProfile(ctx context.Context, profile string) context.Context {
	if ctx == nil {
		ctx = pkgctx.NewSystemContext()
	}

	if profile == emptyValue {
		profile = systemProfileHuman // Default
	}

	// Convert profile string to LoggingProfile enum
	var loggingCtx *pkgctx.LoggingContext
	switch profile {
	case systemProfileMCP:
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileMCP)
	case systemProfileSystem:
		loggingCtx = pkgctx.NewSystemLoggingContext()
	case systemProfileAIAgent:
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileAIAgent)
	case systemProfileDebug:
		loggingCtx = pkgctx.NewLoggingContext(pkgctx.ProfileDebug)
	case systemProfileHuman, "":
		loggingCtx = pkgctx.NewHumanLoggingContext()
	default:
		loggingCtx = pkgctx.NewHumanLoggingContext()
	}

	return pkgctx.WithLoggingContext(ctx, loggingCtx)
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
	auditRouter := coordination.NewStorageAuditRouter(projectRoot, storageProvider)

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
