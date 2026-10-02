package cli

import (
	"context"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/storage"
)

// InitAuditCoordination wraps the context with the given logging profile, creates a
// storage-backed coordinator, and initializes an audit metadata map seeded from initial metadata.
func InitAuditCoordination(
	ctx context.Context,
	projectRoot string,
	storageProvider storage.ObjectStorageProvider,
	profile string,
	metadata map[string]any,
) (context.Context, *coordination.Coordinator, map[string]any) {
	ctx = CreateContextWithLoggingProfile(ctx, profile)
	coordinator := coordination.NewStorageCoordinator(projectRoot, storageProvider)
	auditMetadata := make(map[string]any, len(metadata))
	for k, v := range metadata {
		auditMetadata[k] = v
	}
	return ctx, coordinator, auditMetadata
}

// InitListCoordination initializes context and a coordinator for list operations.
// Returns ok=false if projectRoot is empty or ".".
func InitListCoordination(
	ctx context.Context,
	projectRoot string,
	profile string,
) (context.Context, *coordination.Coordinator, bool) {
	if projectRoot == "" || projectRoot == "." {
		return ctx, nil, false
	}
	ctx = CreateContextWithLoggingProfile(ctx, profile)
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       nil,
		MetricsRouter:     nil,
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})
	return ctx, coordinator, true
}
