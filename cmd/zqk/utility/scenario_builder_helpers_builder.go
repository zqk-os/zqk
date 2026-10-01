package utility

import (
	"github.com/zqk-os/zqk/pkg/datacell"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"

	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/cmd/zqk/system"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/storage"
)

// createScenarioBuilder creates and initializes a scenario builder using the Processor pattern
func createScenarioBuilder(cmd *cobra.Command, targetDir string) (*ScenarioBuilder, error) {
	// Ensure target directory exists
	if err := fileutil.MkdirAll(targetDir, paths.DirPerm755); err != nil {
		return nil, errfmt.Newf("failed to create target directory").Wrap(err)
	}

	// Ensure basic directory structure exists before creating storage factory
	// Storage factory requires .zqk/process directory to exist
	processDir := datacell.ProcessPrimaryDir(targetDir)
	if err := fileutil.MkdirAll(processDir, paths.DirPerm755); err != nil {
		return nil, errfmt.Newf("failed to create process directory").Wrap(err)
	}

	// Create processor (handles context, storage, security, logging with proper precedence)
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return nil, errfmt.Newf("failed to create processor").Wrap(err)
	}

	// For scenario builder, we need storage for the target directory, not the current project
	// Create storage for target directory
	storageFactory, err := storage.NewStorageFactory(cmd.Context(), targetDir)
	if err != nil {
		return nil, errfmt.Newf("failed to create storage factory").Wrap(err)
	}
	storageProvider := storageFactory.GetStorage()

	// Create coordinator for output routing (including audit events for scenario builder operations)
	// Get project root for audit router (use target directory as project root for scenario)
	auditRouter := coordination.NewStorageAuditRouter(targetDir, storageProvider)
	coordinator := coordination.NewCoordinator(coordination.CoordinatorConfig{
		LoggingRouter:     &coordination.DefaultLoggingRouter{},
		AuditRouter:       auditRouter, // Enable audit events for scenario builder operations
		MetricsRouter:     nil,         // Metrics can be added later if needed
		OperationalRouter: &coordination.DefaultOperationalRouter{},
	})

	// Create scenario builder with processor's components (Processor pattern)
	// Coordinator is required - all events must flow through it
	builder := NewScenarioBuilder(targetDir, storageProvider, coordinator)
	builder.logger = proc.Logger()          // Use processor's logger (EventLogger)
	builder.secCtx = proc.SecurityContext() // Use processor's security context
	builder.config.TargetDir = targetDir

	// Subscribe to scenario builder events for progress tracking and logging
	// Get profile from processor CLI context (respects --context flag and config)
	// Default to scenario_builder profile since we have a dedicated profile for it
	profile := ScenarioBuilderProfileName
	if cliCtx := proc.Context(); cliCtx != nil && cliCtx.Profile != emptyValue {
		// Use profile from CLI context (set via --context flag or config)
		profile = cliCtx.Profile
	} else if procOpCtx := proc.OperationContext(); procOpCtx != nil {
		// Fallback to operation context's logging profile
		if loggingCtx := pkgctx.GetLoggingContext(procOpCtx); loggingCtx != nil {
			profile = string(loggingCtx.Profile)
		}
	}
	subscriber := NewScenarioBuilderEventSubscriber(profile)
	coordinator.Subscribe(subscriber)

	// Initialize object ID cache with project root so it knows where to save
	// This ensures the cache can persist updates and be used for reference validation
	objectIDCache := system.GetGlobalObjectIDCache()
	// Load cache (or initialize if it doesn't exist) - this sets the cache directory
	_, _ = objectIDCache.LoadCache(targetDir) // Best effort - cache may not exist yet
	// Ensure cache directory exists
	cacheDir := filepath.Join(targetDir, paths.ProjectDataDir, paths.CacheDir)
	_ = fileutil.MkdirAll(cacheDir, paths.DirPerm755) // Best effort

	// Note: Cache checker is set in scenario_builder_data_loader.go during BuildFromDataFile
	// to enable comprehensive cache-based reference validation (checks idStream, referenceCache, and ObjectIDCache)
	// We don't set it here to avoid overwriting the more comprehensive checker

	// Register cache operation handler to ensure Object ID Cache is updated when objects are created
	// This is critical for reference validation in system check to work correctly
	// Works for both greenfield (clean project) and existing project scenarios
	storage.SetCacheOperationHandler(func(cacheCtx *pkgctx.CacheContext) error {
		switch cacheCtx.Operation {
		case pkgctx.CacheOperationUpdate:
			// Skip cache update if file path is empty (CAS objects will be updated by PostSyncCallback)
			// This prevents errors when file path isn't known yet (e.g., CAS hash-based filenames)
			if cacheCtx.FilePath == emptyValue {
				// CAS callback will provide the correct file path later
				return nil
			}
			if err := system.UpdateObjectIDCache(cacheCtx.NewID, cacheCtx.Kind, cacheCtx.FilePath); err != nil {
				// Log error but don't fail object creation
				builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
					"Cache update failed",
					map[string]any{
						"object_id":          cacheCtx.NewID,
						objects.FieldKeyKind: cacheCtx.Kind,
						"error":              err,
					})
				return err
			}
			storage.ClearObjectIDCachePending(targetDir, cacheCtx.NewID)
			// Save cache to disk after update (best effort - don't fail object creation if save fails)
			// Only save every 10 updates to avoid excessive I/O
			if err := objectIDCache.SaveCache(targetDir); err != nil {
				// Log but don't fail - cache is updated in memory
				builder.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
					"Failed to save object ID cache",
					map[string]any{
						"object_id":          cacheCtx.NewID,
						objects.FieldKeyKind: cacheCtx.Kind,
						"error":              err,
					})
			}
			return nil
		case pkgctx.CacheOperationInvalidate:
			system.InvalidateObjectIDCache(cacheCtx.OldID)
			storage.ClearObjectIDCachePending(targetDir, cacheCtx.OldID)
			// Save cache after invalidation (best effort)
			_ = objectIDCache.SaveCache(targetDir)
			return nil
		case pkgctx.CacheOperationInvalidateAndUpdate:
			system.InvalidateObjectIDCache(cacheCtx.OldID)
			storage.ClearObjectIDCachePending(targetDir, cacheCtx.OldID)
			if err := system.UpdateObjectIDCache(cacheCtx.NewID, cacheCtx.Kind, cacheCtx.FilePath); err != nil {
				return err
			}
			storage.ClearObjectIDCachePending(targetDir, cacheCtx.NewID)
			// Save cache after update (best effort)
			_ = objectIDCache.SaveCache(targetDir)
			return nil
		default:
			return nil // No operation needed
		}
	})

	return builder, nil
}
