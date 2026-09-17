package utility

import (
	"context"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"gopkg.in/yaml.v3"
)

// CopyContext holds context for copying objects
type CopyContext struct {
	SourceStorage storage.ObjectStorageProvider
	IDValidator   *validation.IDValidator
	CopiedObjects map[string]string // original ID -> new ID
}

// validateCopyConfig validates the copy configuration
func validateCopyConfig(config *ScenarioCopyConfig) error {
	if len(config.ObjectIDs) == 0 {
		return errfmt.Errorf("no object IDs specified")
	}
	return nil
}

// setupCopyContext sets up storage and ID validator for copying
func setupCopyContext(ctx context.Context, config *ScenarioCopyConfig, logger *logging.EventLogger) (*CopyContext, error) {
	// Create storage for source project
	sourceStorageFactory, err := storage.NewStorageFactory(ctx, config.SourceProjectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to create source storage factory").Wrap(err)
	}
	sourceStorage := sourceStorageFactory.GetStorage()

	// Get ID validator for ID remapping
	idValidator := validation.GetIDValidator()
	if err := idValidator.LoadPatterns(); err != nil {
		return nil, errfmt.Newf("failed to load ID patterns").Wrap(err)
	}

	return &CopyContext{
		SourceStorage: sourceStorage,
		IDValidator:   idValidator,
		CopiedObjects: make(map[string]string),
	}, nil
}

// inferKindFromID infers the kind from an object ID
func inferKindFromID(objectID string, idValidator *validation.IDValidator, logger *logging.EventLogger) string {
	kind := idValidator.InferKindFromID(objectID)
	if kind == emptyValue {
		logging.FluentEvent(logger).Warn("Could not infer kind from ID").
			String("object_id", objectID).
			Log()
	}
	return kind
}

// readObjectFromSource reads an object from source storage
func readObjectFromSource(ctx context.Context, sourceStorage storage.ObjectStorageProvider, objectID string, logger *logging.EventLogger) (map[string]any, error) {
	obj, err := sourceStorage.Read(ctx, pkgctx.NewSystemSecurityContext(), objectID)
	if err != nil {
		logging.FluentEvent(logger).Warn("Failed to read object from source").
			String("object_id", objectID).
			WithError(err).
			Log()
		return nil, err
	}
	return obj, nil
}

// handleHashChange handles hash change based on change policy
func handleHashChange(ctx context.Context, sb *ScenarioBuilder, sourceStorage storage.ObjectStorageProvider, objectID, kind string, snapshotHash, currentHash string, config *ScenarioCopyConfig, obj map[string]any) (map[string]any, bool) {
	// Handle change based on policy
	switch config.ChangePolicy {
	case "reject":
		logging.FluentEvent(sb.logger).Warn("Rejecting modified object").
			String("object_id", objectID).
			Log()
		return nil, false // Skip this object
	case "reconstruct":
		// Try to reconstruct state from change journal
		reconstructedObj, reconErr := sb.reconstructObjectState(ctx, sourceStorage, objectID, kind, snapshotHash, config.SourceProjectRoot)
		if reconErr != nil {
			logging.FluentEvent(sb.logger).Warn("Failed to reconstruct object state, using current state").
				String("object_id", objectID).
				WithError(reconErr).
				Log()
			// Use current state (already read above)
		} else {
			logging.FluentEvent(sb.logger).Info("Reconstructed object state from change journal").
				String("object_id", objectID).
				Log()
			return reconstructedObj, true
		}
	case "include", "":
		// Default: include current state (object was modified)
		logging.FluentEvent(sb.logger).Info("Including modified object (current state)").
			String("object_id", objectID).
			Log()
	}
	return obj, true
}

// verifyObjectHash verifies object hash and handles changes
func verifyObjectHash(ctx context.Context, sb *ScenarioBuilder, sourceStorage storage.ObjectStorageProvider, objectID, kind string, config *ScenarioCopyConfig, obj map[string]any) (map[string]any, bool) {
	snapshotHash, wasCaptured := config.SnapshotHashes[objectID]
	if !wasCaptured {
		return obj, true // No snapshot hash, proceed with current object
	}

	currentHash, err := sb.getObjectHash(ctx, sourceStorage, objectID, kind, config.SourceProjectRoot)
	if err != nil || currentHash == snapshotHash {
		return obj, true // Hash matches or error, proceed
	}

	// Hash changed - object was modified after snapshot
	logging.FluentEvent(sb.logger).Warn("Object hash changed since snapshot").
		String("object_id", objectID).
		String("snapshot_hash", snapshotHash).
		String("current_hash", currentHash).
		Log()

	return handleHashChange(ctx, sb, sourceStorage, objectID, kind, snapshotHash, currentHash, config, obj)
}

// loadSpecForKind loads the spec for a kind
func loadSpecForKind(kind string, logger *logging.EventLogger) *objects.Spec {
	specLoader := objects.GetGlobalSpecLoader()
	spec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml")
	if err != nil {
		logging.FluentEvent(logger).Warn("Failed to load spec for kind").
			String("kind", kind).
			WithError(err).
			Log()
		return nil
	}
	return spec
}

// processObjectForCopy processes an object for copying (filter PII, generate ID, etc.)
func processObjectForCopy(ctx context.Context, sb *ScenarioBuilder, obj map[string]any, objectID, kind string, spec *objects.Spec, config *ScenarioCopyConfig, idValidator *validation.IDValidator) (map[string]any, string, error) {
	// Filter PII and apply overrides
	filteredObj := sb.filterPIIAndApplyOverrides(obj, kind, spec, config)

	// Calculate hash of original object (before overrides) for mapping
	originalData, _ := yaml.Marshal(obj) //nolint:errcheck // Marshal error would result in empty hash, acceptable for comparison
	originalHash := storage.CalculateSHA256Hash(originalData)

	// Calculate hash of filtered object (after overrides) for mapping
	filteredData, _ := yaml.Marshal(filteredObj) //nolint:errcheck // Marshal error would result in empty hash, acceptable for comparison
	newHash := storage.CalculateSHA256Hash(filteredData)

	// Store hash mapping (original -> new)
	if config.HashMappings == nil {
		config.HashMappings = make(map[string]string)
	}
	config.HashMappings[originalHash] = newHash

	// Generate new ID for scenario
	newID := sb.generateScenarioID(objectID, kind, config, idValidator)
	filteredObj[objects.FieldKeyID] = newID

	// Update namespace if override specified
	if config.NamespaceOverride != emptyValue {
		filteredObj[objects.FieldKeyNamespaceID] = config.NamespaceOverride
	} else {
		// Default to test namespace
		filteredObj[objects.FieldKeyNamespaceID] = "test:scenario"
	}

	// Update origin fields to indicate this is test data
	filteredObj[objects.FieldKeyOriginProject] = "test-scenario"
	filteredObj[objects.FieldKeyOriginSystem] = validation.DefaultOriginSystem

	return filteredObj, newID, nil
}

// copySingleObject copies a single object
func copySingleObject(ctx context.Context, sb *ScenarioBuilder, copyCtx *CopyContext, objectID string, config *ScenarioCopyConfig) error {
	// Infer kind from ID
	kind := inferKindFromID(objectID, copyCtx.IDValidator, sb.logger)
	if kind == emptyValue {
		return nil // Already logged warning
	}

	// Read object from source
	obj, err := readObjectFromSource(ctx, copyCtx.SourceStorage, objectID, sb.logger)
	if err != nil {
		return nil // Already logged warning, continue with next object
	}

	// Verify hash and handle changes
	obj, shouldContinue := verifyObjectHash(ctx, sb, copyCtx.SourceStorage, objectID, kind, config, obj)
	if !shouldContinue {
		return nil // Object rejected or skipped
	}

	// Load spec to identify PII fields
	spec := loadSpecForKind(kind, sb.logger)

	// Process object for copying
	filteredObj, newID, err := processObjectForCopy(ctx, sb, obj, objectID, kind, spec, config, copyCtx.IDValidator)
	if err != nil {
		logging.FluentEvent(sb.logger).Warn("Failed to process object for copy").
			String("object_id", objectID).
			WithError(err).
			Log()
		return nil // Continue with next object
	}

	// Track ID mapping
	copyCtx.CopiedObjects[objectID] = newID

	// Create object in target scenario
	if err := sb.storage.Create(ctx, sb.secCtx, filteredObj); err != nil {
		logging.FluentEvent(sb.logger).Warn("Failed to create object in scenario").
			String("object_id", objectID).
			String("new_id", newID).
			WithError(err).
			Log()
		return nil // Continue with next object
	}

	sb.generated[kind]++
	return nil
}

// updateScenarioObject updates the scenario object with metadata
func updateScenarioObject(ctx context.Context, sb *ScenarioBuilder, config *ScenarioCopyConfig) {
	if sb.scenarioID == emptyValue {
		return
	}

	updates := map[string]any{
		objects.FieldKeySourceObjectIDs: config.ObjectIDs,
		objects.FieldKeySnapshotHashes:  config.SnapshotHashes, // Object ID -> hash at snapshot time
		objects.FieldKeyHashMappings:    config.HashMappings,   // Original hash -> new hash
		objects.FieldKeyUpdatedAt:       "",
		objects.FieldKeyUpdatedBy:       "ACC-1785920548450214012-68b850c0",
	}
	if err := sb.storage.Update(ctx, sb.secCtx, sb.scenarioID, updates); err != nil {
		logging.FluentEvent(sb.logger).Warn("Failed to update scenario with source object IDs and hash mappings").
			String("scenario_id", sb.scenarioID).
			WithError(err).
			Log()
	}
}

// calculateTotalCopied calculates total number of copied objects
func calculateTotalCopied(generated map[string]int) int {
	totalCopied := 0
	for _, count := range generated {
		totalCopied += count
	}
	return totalCopied
}
