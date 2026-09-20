package utility

import (
	"strings"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	instancebuilders "github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// prepareObjectFromDataFile prepares an object from data file with required fields
func (sb *ScenarioBuilder) prepareObjectFromDataFile(obj map[string]any, kind string) error {
	// Use instance builder to properly set fields and status
	schemaVersion := objects.DefaultSchemaVersion
	if sv, ok := obj[objects.FieldKeySchemaVersion].(string); ok {
		schemaVersion = sv
	} else {
		obj[objects.FieldKeySchemaVersion] = schemaVersion
	}

	// Get instance builder from registry
	registry := instancebuilders.GetGlobalRegistry()
	builder, err := registry.GetBuilder(kind, schemaVersion)
	if err != nil {
		// If builder not available, just set fields directly
		// Use background context since we don't have ctx in this function
		sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
			"Failed to get instance builder, setting fields directly",
			map[string]any{
				objects.FieldKeyKind: kind,
				"error":              err,
			})
		return sb.setFieldsDirectly(obj, kind)
	}

	// Load lifecycle to get valid statuses
	// Use storage instance's lifecycle loader to ensure correct project context
	var lifecycleLoader *objects.LifecycleLoader
	if fileStorage, ok := sb.storage.(*storagepkg.FileObjectStorage); ok {
		// Get lifecycle loader from storage instance (uses correct project's lifecycle directory)
		lifecycleLoader = fileStorage.GetLifecycleLoader()
	}
	if lifecycleLoader == nil {
		// Fallback to global loader if storage doesn't expose it
		lifecycleLoader = objects.GetGlobalLifecycleLoader()
	}
	lifecycle, err := lifecycleLoader.LoadLifecycle(kind)
	if err != nil {
		// Use background context since we don't have ctx in this function
		sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
			"Failed to load lifecycle, using defaults",
			map[string]any{
				objects.FieldKeyKind: kind,
				"error":              err,
			})
		lifecycle = nil // Will use defaults below
	}

	// Get valid statuses from lifecycle, prioritizing initial statuses
	validStatuses := []string{"proposed"} // Default fallback
	if lifecycle != nil && len(lifecycle.Statuses) > 0 {
		validStatuses = make([]string, 0, len(lifecycle.Statuses))
		// First, add all origin statuses (these have no preconditions)
		for _, status := range lifecycle.Statuses {
			if status.Origin {
				validStatuses = append(validStatuses, status.Value)
			}
		}
		// If no initial statuses found, use the first status
		if len(validStatuses) == 0 && len(lifecycle.Statuses) > 0 {
			validStatuses = append(validStatuses, lifecycle.Statuses[0].Value)
		}
	}

	// Determine status to use
	var statusValue string
	if status, ok := obj[objects.FieldKeyStatus].(string); ok && status != emptyValue {
		statusValue = status
	} else {
		// Use initial status from lifecycle
		statusValue = validStatuses[0]
	}

	// Handle account ID: leave unset so storage generates ACC-* (full cutover).
	// TRACK: BLI-1785905134201010000-07393484
	if kind == objects.KindAccount {
		if id, ok := obj[objects.FieldKeyID].(string); ok && strings.HasPrefix(id, "account:") {
			delete(obj, objects.FieldKeyID)
		}
	}

	// Check if object has ID - instance builder requires ID, but storage can auto-generate
	id, hasID := obj[objects.FieldKeyID].(string)
	if hasID && id != emptyValue {
		// Object has ID - use instance builder for validation and field setting
		builder.SetID(id)

		// Set all fields
		for key, value := range obj {
			if key == objects.FieldKeyID || key == objects.FieldKeyKind || key == objects.FieldKeySchemaVersion || key == objects.FieldKeyStatus {
				continue // Skip these, handled separately
			}
			builder.SetField(key, value)
		}

		// Set status using builder method (validates against lifecycle)
		builder.SetStatus(statusValue)

		// Build instance
		builtObj, err := builder.Build()
		if err != nil {
			// Build error - try fallback but validate before creating
			sb.emitCoordinatorEvent(pkgctx.NewSystemContext(), ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
				"Failed to build instance, using direct field setting",
				map[string]any{
					objects.FieldKeyKind: kind,
					"error":              err.Error(),
				})
			// Fall through to setFieldsDirectly
		} else {
			// Successfully built - use built object
			// Copy built fields back to obj
			for k, v := range builtObj {
				obj[k] = v
			}
			// Ensure kind is set
			obj[objects.FieldKeyKind] = kind
			// Ensure required fields are set
			if err := sb.setFieldsDirectly(obj, kind); err != nil {
				return err
			}
			return nil
		}
	}
	// No ID - skip instance builder (storage will auto-generate ID via ensureObjectID)
	// Just ensure basic fields are set and use direct field setting

	// Final validation: ensure kind is set (critical for storage)
	if obj[objects.FieldKeyKind] == nil || obj[objects.FieldKeyKind] == emptyValue {
		obj[objects.FieldKeyKind] = kind
	}

	// Final validation before returning - ensure object is valid
	if err := sb.validateObjectBeforeCreation(obj, kind); err != nil {
		return errfmt.Newf("object validation failed after preparation").Wrap(err)
	}

	return nil
}

// setFieldsDirectly sets required fields directly in the object map
func (sb *ScenarioBuilder) setFieldsDirectly(obj map[string]any, kind string) error {
	// Set schema version if not present
	if _, ok := obj[objects.FieldKeySchemaVersion]; !ok {
		obj[objects.FieldKeySchemaVersion] = objects.DefaultSchemaVersion
	}

	// Set origin fields if not present
	if _, ok := obj[objects.FieldKeyOriginProject]; !ok {
		obj[objects.FieldKeyOriginProject] = "zqk"
	}
	if _, ok := obj[objects.FieldKeyOriginSystem]; !ok {
		obj[objects.FieldKeyOriginSystem] = "zqk"
	}
	if _, ok := obj[objects.FieldKeyNamespaceID]; !ok {
		obj[objects.FieldKeyNamespaceID] = "zqk:kernel"
	}

	// Set created_by/updated_by if not present
	if _, ok := obj[objects.FieldKeyCreatedBy]; !ok {
		obj[objects.FieldKeyCreatedBy] = pkgctx.SystemAccountID
	}
	if _, ok := obj[objects.FieldKeyUpdatedBy]; !ok {
		obj[objects.FieldKeyUpdatedBy] = pkgctx.SystemAccountID
	}

	// Set timestamps if not present
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	if _, ok := obj[objects.FieldKeyCreatedAt]; !ok {
		obj[objects.FieldKeyCreatedAt] = now
	}
	if _, ok := obj[objects.FieldKeyUpdatedAt]; !ok {
		obj[objects.FieldKeyUpdatedAt] = now
	}

	// Don't set ID - let storage auto-generate it
	// But don't delete it if it's already set (for reference objects)
	// Accounts: strip legacy account:* ids so storage assigns ACC-*.
	// TRACK: BLI-1785905134201010000-07393484
	if kind == objects.KindAccount {
		if id, ok := obj[objects.FieldKeyID].(string); ok && strings.HasPrefix(id, "account:") {
			delete(obj, objects.FieldKeyID)
		}
	}

	if _, hasID := obj[objects.FieldKeyID]; hasID && obj[objects.FieldKeyID] == emptyValue {
		delete(obj, objects.FieldKeyID)
	}

	return nil
}

// validateObjectBeforeCreation validates that an object has minimum required fields before creation
// Returns error if object is missing critical fields that would cause creation to fail
func (sb *ScenarioBuilder) validateObjectBeforeCreation(obj map[string]any, kind string) error {
	// Kind is always required
	if objKind, ok := obj[objects.FieldKeyKind].(string); !ok || objKind == emptyValue {
		if kind != emptyValue {
			obj[objects.FieldKeyKind] = kind
		} else {
			return errfmt.Errorf("object missing required field: kind")
		}
	}

	// For most objects, ID is required (storage will auto-generate if missing for some kinds)
	// But we should validate that if ID is present, it's not empty
	if id, hasID := obj[objects.FieldKeyID]; hasID {
		if idStr, ok := id.(string); ok && idStr == emptyValue {
			// Empty ID string - remove it so storage can auto-generate
			delete(obj, objects.FieldKeyID)
		}
	}

	// Accounts use ACC-* ids from storage generation — do not invent account:* ids.
	// TRACK: BLI-1785905134201010000-07393484
	if kind == objects.KindAccount {
		if id, ok := obj[objects.FieldKeyID].(string); ok && strings.HasPrefix(id, "account:") {
			delete(obj, objects.FieldKeyID)
		}
	}

	// Schema version should be set
	if _, ok := obj[objects.FieldKeySchemaVersion]; !ok {
		obj[objects.FieldKeySchemaVersion] = objects.DefaultSchemaVersion
	}

	return nil
}
