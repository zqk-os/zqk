package utility

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/validation"
	"gopkg.in/yaml.v3"
)

// ScenarioCopyConfig holds configuration for copying objects from main project
type ScenarioCopyConfig struct {
	SourceProjectRoot string            // Main project root (where objects are copied from)
	TargetDir         string            // Target scenario directory
	ObjectIDs         []string          // Object IDs to copy
	IDPrefixOverride  string            // Override ID prefix for scenario (e.g., "TEST-")
	NamespaceOverride string            // Override namespace (e.g., "test:scenario")
	PIIFields         map[string]bool   // Fields to exclude (PII filtering)
	FieldOverrides    map[string]any    // Field value overrides (e.g., {"status": "proposed"})
	PreserveState     bool              // Whether to preserve object state (status, timestamps, etc.)
	HashMappings      map[string]string // Original hash -> new hash mapping (for integrity verification)
	SnapshotHashes    map[string]string // Object ID -> hash at snapshot time (for change detection)
	ChangePolicy      string            // Policy for handling changes: "reject", "include", "reconstruct"
}

// CaptureSnapshotHashes captures the hash of each object at snapshot time
// This is called before copying to establish the baseline state
func (sb *ScenarioBuilder) CaptureSnapshotHashes(ctx context.Context, config *ScenarioCopyConfig) error {
	logging.FluentEvent(sb.logger).Info("Capturing snapshot hashes").
		String("source", config.SourceProjectRoot).
		Int("object_count", len(config.ObjectIDs)).
		Log()

	// Create storage for source project
	sourceStorageFactory, err := storage.NewStorageFactory(ctx, config.SourceProjectRoot)
	if err != nil {
		return errfmt.Newf("failed to create source storage factory").Wrap(err)
	}
	sourceStorage := sourceStorageFactory.GetStorage()

	// Get ID validator
	idValidator := validation.GetIDValidator()
	if err := idValidator.LoadPatterns(); err != nil {
		return errfmt.Newf("failed to load ID patterns").Wrap(err)
	}

	// Initialize hash maps
	if config.SnapshotHashes == nil {
		config.SnapshotHashes = make(map[string]string)
	}
	if config.HashMappings == nil {
		config.HashMappings = make(map[string]string)
	}

	// Capture hash for each object
	for _, objectID := range config.ObjectIDs {
		// Infer kind from ID
		kind := idValidator.InferKindFromID(objectID)
		if kind == emptyValue {
			logging.FluentEvent(sb.logger).Warn("Could not infer kind from ID").
				String("object_id", objectID).
				Log()
			continue
		}

		// Get hash from CAS if available
		hash, err := sb.getObjectHash(ctx, sourceStorage, objectID, kind, config.SourceProjectRoot)
		if err != nil {
			logging.FluentEvent(sb.logger).Warn("Failed to get hash for object").
				String("object_id", objectID).
				WithError(err).
				Log()
			// Continue - will calculate hash from content during copy
			continue
		}

		config.SnapshotHashes[objectID] = hash
		logging.FluentEvent(sb.logger).Debug("Captured snapshot hash").
			String("object_id", objectID).
			String("hash", hash).
			Log()
	}

	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		fmt.Sprintf("Captured snapshot hashes for %d objects", len(config.SnapshotHashes)),
		map[string]any{objects.FieldKeyObjectCount: len(config.SnapshotHashes)})
	return nil
}

// getObjectHash gets the hash for an object by reading it and calculating hash from content
// This works for both CAS and non-CAS storage
func (sb *ScenarioBuilder) getObjectHash(ctx context.Context, sourceStorage storage.ObjectStorageProvider, objectID, _, _ string) (string, error) {
	// Read object and calculate hash from content
	obj, err := sourceStorage.Read(ctx, pkgctx.NewSystemSecurityContext(), objectID)
	if err != nil {
		return "", errfmt.Newf("failed to read object").Wrap(err)
	}

	// Marshal to YAML and calculate hash
	data, err := yaml.Marshal(obj)
	if err != nil {
		return "", errfmt.Newf("failed to marshal object").Wrap(err)
	}

	return storage.CalculateSHA256Hash(data), nil
}

// CopyObjectsFromProject copies objects from the main project to the scenario
func (sb *ScenarioBuilder) CopyObjectsFromProject(ctx context.Context, config *ScenarioCopyConfig) error {
	// Validate config
	if err := validateCopyConfig(config); err != nil {
		return err
	}

	logging.FluentEvent(sb.logger).Info("Copying objects from project").
		String("source", config.SourceProjectRoot).
		String("target", config.TargetDir).
		Int("count", len(config.ObjectIDs)).
		Log()

	// Capture snapshot hashes first (before any changes can occur)
	if err := sb.CaptureSnapshotHashes(ctx, config); err != nil {
		logging.FluentEvent(sb.logger).Warn("Failed to capture snapshot hashes, continuing without hash verification").
			WithError(err).
			Log()
	}

	// Setup copy context
	copyCtx, err := setupCopyContext(ctx, config, sb.logger)
	if err != nil {
		return err
	}

	// Copy each object
	for _, objectID := range config.ObjectIDs {
		if err := copySingleObject(ctx, sb, copyCtx, objectID, config); err != nil {
			// Error already logged, continue with next object
			continue
		}
	}

	// Update references in copied objects
	if err := sb.updateReferences(ctx, copyCtx.CopiedObjects, config); err != nil {
		logging.FluentEvent(sb.logger).Warn("Failed to update references").
			WithError(err).
			Log()
		// Don't fail - references are best effort
	}

	// Output summary
	totalCopied := calculateTotalCopied(sb.generated)
	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		fmt.Sprintf("Copied %d objects from project to scenario", totalCopied),
		map[string]any{"total_copied": totalCopied})

	// Update scenario object with metadata
	updateScenarioObject(ctx, sb, config)

	return nil
}

// reconstructObjectState attempts to reconstruct object state at a specific hash using change journal
// This walks backwards through change journal entries to find the state at the target hash
func (sb *ScenarioBuilder) reconstructObjectState(ctx context.Context, sourceStorage storage.ObjectStorageProvider, objectID, _, targetHash, _ string) (map[string]any, error) {
	logging.FluentEvent(sb.logger).Debug("Attempting to reconstruct object state from change journal").
		String("object_id", objectID).
		String("target_hash", targetHash).
		Log()

	// Read current object state
	currentObj, err := sourceStorage.Read(ctx, pkgctx.NewSystemSecurityContext(), objectID)
	if err != nil {
		return nil, errfmt.Newf("failed to read current object").Wrap(err)
	}

	// Calculate current hash
	currentData, _ := yaml.Marshal(currentObj) //nolint:errcheck // Marshal error would result in empty hash, acceptable for comparison
	currentHash := storage.CalculateSHA256Hash(currentData)

	// If current hash matches target, return current object
	if currentHash == targetHash {
		return currentObj, nil
	}

	// TODO: Implement full change journal reconstruction
	// This would involve:
	// 1. Query change journal entries for this object (object_ref = "kind:objectID")
	// 2. Walk backwards through entries, applying reverse changes
	// 3. Calculate hash after each reverse change
	// 4. Stop when hash matches target hash
	// 5. Return reconstructed state

	// For now, return error to indicate reconstruction is not yet fully implemented
	// This allows the system to fall back to current state
	return nil, errfmt.Errorf("change journal reconstruction not yet fully implemented - would need to walk change journal entries backwards from current state to target hash")
}

// filterPIIAndApplyOverrides filters PII fields and applies overrides
func (sb *ScenarioBuilder) filterPIIAndApplyOverrides(obj map[string]any, kind string, spec *objects.Spec, config *ScenarioCopyConfig) map[string]any {
	filtered := make(map[string]any)

	// Get exclusion rules
	alwaysExclude := getAlwaysExcludeFields()
	piiFields := identifyPIIFieldsFromSpec(spec)
	commonPIIPatterns := getCommonPIIPatterns()

	// Copy fields, filtering PII
	for key, value := range obj {
		// Skip if should be excluded
		if shouldExcludeField(key, alwaysExclude, piiFields, commonPIIPatterns) {
			continue
		}

		// Apply field override
		filtered[key] = applyFieldOverride(key, value, config)
	}

	// Apply preserve state logic
	applyPreserveStateLogic(filtered, config)

	return filtered
}

// generateScenarioID generates a new ID for the scenario object
func (sb *ScenarioBuilder) generateScenarioID(originalID, kind string, config *ScenarioCopyConfig, idValidator *validation.IDValidator) string {
	// If ID prefix override is specified, use it
	if config.IDPrefixOverride != emptyValue {
		// Extract sequence number from original ID
		parts := strings.Split(originalID, "-")
		if len(parts) > 1 {
			sequence := parts[len(parts)-1]
			return fmt.Sprintf("%s%s", config.IDPrefixOverride, sequence)
		}
		// Fallback: use original ID with prefix
		return fmt.Sprintf("%s%s", config.IDPrefixOverride, strings.TrimPrefix(originalID, idValidator.GetValidPrefixes(kind)[0]))
	}

	// Default: add "TEST-" prefix
	prefixes := idValidator.GetValidPrefixes(kind)
	if len(prefixes) > 0 {
		originalPrefix := prefixes[0]
		// Extract sequence
		sequence := strings.TrimPrefix(originalID, originalPrefix)
		return fmt.Sprintf("TEST-%s", sequence)
	}

	// Fallback: use original ID with TEST- prefix
	return fmt.Sprintf("TEST-%s", originalID)
}

// updateReferences updates references in copied objects to point to new IDs
//
//nolint:unparam // Always returns nil error - function is designed to always succeed
func (sb *ScenarioBuilder) updateReferences(ctx context.Context, idMapping map[string]string, _ *ScenarioCopyConfig) error {
	// This is a simplified implementation
	// In a full implementation, we would:
	// 1. Scan all copied objects for reference fields
	// 2. Update references to point to new IDs
	// 3. Handle nested references (e.g., in lists, maps)

	logging.FluentEvent(sb.logger).Debug("Updating references in copied objects").
		Int("mappings", len(idMapping)).
		Log()

	// Common reference field patterns
	referencePatterns := []string{
		"_ref", "_refs", "_id", "_ids",
		"goal_ref", "workstream_ref", "milestone_ref",
		"parent_ref", "child_refs",
	}

	// For each copied object, update references
	for _, newID := range idMapping {
		// Read the copied object
		obj, err := sb.storage.Read(ctx, sb.secCtx, newID)
		if err != nil {
			continue
		}

		updated := false
		// Update reference fields
		for key, value := range obj {
			keyLower := strings.ToLower(key)
			for _, pattern := range referencePatterns {
				if strings.Contains(keyLower, pattern) {
					// Update reference value
					switch val := value.(type) {
					case string:
						if mappedID, exists := idMapping[val]; exists {
							obj[key] = mappedID
							updated = true
						}
					case []any:
						listValue := val
						// Handle list of references
						updatedList := false
						for i, item := range listValue {
							if strItem, ok := item.(string); ok {
								if mappedID, exists := idMapping[strItem]; exists {
									listValue[i] = mappedID
									updatedList = true
								}
							}
						}
						if updatedList {
							obj[key] = listValue
							updated = true
						}
					}
					break
				}
			}
		}

		// Update object if references were changed
		if updated {
			if err := sb.storage.Update(ctx, sb.secCtx, newID, obj); err != nil {
				logging.FluentEvent(sb.logger).Warn("Failed to update references in object").
					String("object_id", newID).
					WithError(err).
					Log()
			}
		}
	}

	return nil
}
