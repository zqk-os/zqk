package system

import (
	"context"
	"fmt"
	"time"

	"github.com/lanceman/zqk/internal/cli"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/zqktime"
	"github.com/spf13/cobra"
)

// runSnapshotScenario executes the snapshot-scenario command
func runSnapshotScenario(cmd *cobra.Command, args []string) error {
	ctx := pkgctx.NewSystemContext()
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	// Get CLI context and project root
	cliCtx := cli.GetContext(cmd)
	projectRoot, err := getProjectRootForSnapshot(cliCtx)
	if err != nil {
		return err
	}

	// Parse flags
	flags := parseSnapshotScenarioFlags(cmd)

	// Validate inputs
	if err := validateSnapshotInputs(flags); err != nil {
		return err
	}

	// Resolve target directory
	flags.Target = resolveTargetDirectory(flags.Target)

	out := cli.CommandOutputWriter(cmd, ctx)

	// Output configuration
	if flags.Verbose {
		outputSnapshotConfiguration(flags, out)
	}

	// Handle dry run
	if flags.DryRun {
		handleDryRun(flags, out)
		return nil
	}

	// Setup storage
	underlyingStorage, snapshotManager, err := setupSnapshotStorage(ctx, projectRoot)
	if err != nil {
		return err
	}

	return executeSnapshotWorkflow(ctx, underlyingStorage, snapshotManager, flags, logger, out)
}

// discoverObjects discovers objects by IDs or kinds
// When discovering by kinds, uses snapable trait's object_query configuration if available
//
//nolint:unparam // Always returns nil error - function is designed to always succeed
func discoverObjects(
	ctx context.Context,
	storageProvider storage.ObjectStorageProvider,
	objectIDs []string,
	kinds []string,
	queryPreset string,
	logger logging.Logger,
) ([]string, error) {
	var allObjectIDs []string

	// Discover by object IDs
	idsFromObjects := discoverObjectsByIDs(ctx, storageProvider, objectIDs, logger)
	allObjectIDs = append(allObjectIDs, idsFromObjects...)

	// Discover by kinds
	idsFromKinds := discoverObjectsByKinds(ctx, storageProvider, kinds, queryPreset, logger)
	allObjectIDs = append(allObjectIDs, idsFromKinds...)

	// Remove duplicates and return
	return removeDuplicateIDs(allObjectIDs), nil
}

// extractObjectsWithHandlers extracts objects from storage with change detection and field handlers
//
//nolint:unparam // Always returns nil error - function is designed to always succeed
func extractObjectsWithHandlers(
	ctx context.Context,
	storageProvider storage.ObjectStorageProvider,
	metadata *storage.SnapshotMetadata,
	changePolicy string,
	logger logging.Logger,
) ([]map[string]any, error) {
	secCtx := pkgctx.NewSystemSecurityContext()
	specLoader := objects.GetGlobalSpecLoader()
	var extractedObjects []map[string]any

	for _, objectID := range metadata.ObjectIDs {
		obj, err := storageProvider.Read(ctx, secCtx, objectID)
		if err != nil {
			logging.Fluent(logger).Warn("Failed to read object").
				String("object_id", objectID).
				WithError(err).
				Log()
			continue
		}

		// Get object kind
		kind, _ := obj[objects.FieldKeyKind].(string)
		if kind == emptyValue {
			logging.Fluent(logger).Warn("Object missing kind, skipping").
				String("object_id", objectID).
				Log()
			continue
		}

		// Check if object has changed (if we have mtime)
		if mtime, ok := metadata.ModificationTimes[objectID]; ok {
			if objMtimeStr, ok := obj[objects.FieldKeyUpdatedAt].(string); ok {
				if objMtime, err := time.Parse("2006-01-02T15:04:05Z", objMtimeStr); err == nil {
					if objMtime.After(mtime) {
						// Object has changed
						switch changePolicy {
						case "reject":
							logging.Fluent(logger).Warn("Object changed, rejecting").
								String("object_id", objectID).
								Log()
							continue
						case "reconstruct":
							// Reconstruct state from change journal
							reconstructed, err := storage.ReconstructStateAtTimestamp(
								ctx,
								storageProvider,
								obj,
								objectID,
								kind,
								metadata.Timestamp,
								logger,
							)
							if err != nil {
								logging.Fluent(logger).Warn("Failed to reconstruct state, using current state").
									String("object_id", objectID).
									WithError(err).
									Log()
								// Fall back to current state
								processedObj, err := applyFieldHandlers(obj, kind, specLoader, logger)
								if err != nil {
									processedObj = obj
								}
								extractedObjects = append(extractedObjects, processedObj)
								continue
							}
							// Use reconstructed state
							logging.Fluent(logger).Info("Reconstructed object state at snapshot timestamp").
								String("object_id", objectID).
								Log()
							obj = reconstructed
						case "include":
							// Use current state
							logging.Fluent(logger).Debug("Object changed, including current state").
								String("object_id", objectID).
								Log()
						}
					}
				}
			}
		}

		// Apply field handlers if object has snapable trait
		processedObj, err := applyFieldHandlers(obj, kind, specLoader, logger)
		if err != nil {
			logging.Fluent(logger).Warn("Failed to apply field handlers").
				String("object_id", objectID).
				WithError(err).
				Log()
			// Continue with unprocessed object
			processedObj = obj
		}

		extractedObjects = append(extractedObjects, processedObj)
	}

	return extractedObjects, nil
}

// applyFieldHandlers applies data handlers to fields based on snapable trait configuration
//
//nolint:unparam // Always returns nil error - function is designed to always succeed
func applyFieldHandlers(
	obj map[string]any,
	kind string,
	specLoader *objects.SpecLoader,
	logger logging.Logger,
) (map[string]any, error) {
	// Load spec to get field definitions
	spec, err := specLoader.LoadSpecWithInheritance(kind)
	if err != nil {
		return obj, nil // Can't load spec, return object as-is
	}

	// Check if object has snapable trait
	if !checkObjectHasSnapableTrait(spec) {
		return obj, nil // Not snapable, return as-is
	}

	// Process each field
	processedObj := make(map[string]any)
	for key, value := range obj {
		// Copy metadata fields as-is
		if isSnapshotMetadataField(key) {
			processedObj[key] = value
			continue
		}

		// Process field based on spec
		processedValue, shouldInclude, _ := processField(key, value, spec, logger)
		if shouldInclude && processedValue != nil {
			processedObj[key] = processedValue
		}
	}

	return processedObj, nil
}

// createScenarioObject creates a scenario object with snapshot metadata
func createScenarioObject(
	ctx context.Context,
	storageProvider storage.ObjectStorageProvider,
	targetDir string,
	metadata *storage.SnapshotMetadata,
	extractedObjects []map[string]any,
	changePolicy string,
	adaptors []string,
	adaptorConfig *storage.AdaptorConfig,
	logger logging.Logger,
) (map[string]any, error) {
	secCtx := pkgctx.NewSystemSecurityContext()

	// Generate scenario ID
	scenarioID := fmt.Sprintf("SCE-%d", time.Now().Unix())

	// Build snapshot hashes map
	snapshotHashes := make(map[string]string)
	for objectID, hash := range metadata.Hashes {
		snapshotHashes[objectID] = hash
	}

	// Create scenario object
	scenarioObj := map[string]any{
		objects.FieldKeyID:                scenarioID,
		objects.FieldKeyKind:              objects.KindScenario,
		objects.FieldKeyTitle:             fmt.Sprintf("Snapshot Scenario - %s", zqktime.NowLayoutUTC(zqktime.LayoutDateTimeSpace)),
		objects.FieldKeyDescription:       fmt.Sprintf("Snapshot captured at %s", metadata.Timestamp.Format(time.RFC3339Nano)),
		"target_directory":                targetDir,
		objects.FieldKeySnapshotTimestamp: metadata.Timestamp.Format(time.RFC3339Nano),
		objects.FieldKeySnapshotHashes:    snapshotHashes,
		objects.FieldKeyChangePolicy:      changePolicy,
		objects.FieldKeyObjectCount:       len(extractedObjects),
		objects.FieldKeyCreatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyCreatedBy:         secCtx.AccountID,
		objects.FieldKeyUpdatedAt:         zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ),
		objects.FieldKeyUpdatedBy:         secCtx.AccountID,
		objects.FieldKeySchemaVersion:     objects.DefaultSchemaVersion,
		objects.FieldKeyStatus:            objects.ObjectStatusProposed,
		objects.FieldKeyOriginProject:     validation.DefaultOriginProject,
		objects.FieldKeyOriginSystem:      validation.DefaultOriginSystem,
	}

	// Add adaptors if specified
	if len(adaptors) > 0 {
		scenarioObj["adaptors"] = adaptors

		// Add adaptor configuration
		if adaptorConfig != nil {
			adaptorConfigMap := make(map[string]any)
			if adaptorConfig.IDPrefix != emptyValue {
				adaptorConfigMap["id_prefix"] = adaptorConfig.IDPrefix
			}
			if adaptorConfig.Namespace != emptyValue {
				adaptorConfigMap[objects.KindNamespace] = adaptorConfig.Namespace
			}
			if len(adaptorConfig.FieldOverrides) > 0 {
				adaptorConfigMap["field_overrides"] = adaptorConfig.FieldOverrides
			}
			adaptorConfigMap["exclude_pii"] = adaptorConfig.ExcludePII
			adaptorConfigMap["preserve_state"] = adaptorConfig.PreserveState

			// Add ID mappings if any
			if len(adaptorConfig.IDMapping) > 0 {
				scenarioObj["id_mappings"] = adaptorConfig.IDMapping
			}

			if len(adaptorConfigMap) > 0 {
				scenarioObj["adaptor_config"] = adaptorConfigMap
			}
		}
	}

	// Create scenario object
	if err := storageProvider.Create(ctx, secCtx, scenarioObj); err != nil {
		return nil, errfmt.Newf("failed to create scenario object").Wrap(err)
	}

	logging.Fluent(logger).Info("Scenario object created").
		String("scenario_id", scenarioID).
		ObjectCount(len(extractedObjects)).
		Log()

	return scenarioObj, nil
}

// getObjectID safely extracts the object ID from an object map
func getObjectID(obj map[string]any) string {
	if id, ok := obj[objects.FieldKeyID].(string); ok {
		return id
	}
	return "unknown"
}
