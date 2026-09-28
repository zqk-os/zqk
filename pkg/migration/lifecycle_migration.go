package migration

import (
	"context"
	"fmt"
	"maps"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	lifecycleEnum "github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/lifecycle"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
)

// lifecycleMigKey* are nested lifecycle YAML map keys (statuses[], transitions[], percent_complete).
const (
	lifecycleMigKeyArchive         = "archive"
	lifecycleMigKeyAuto            = "auto"
	lifecycleMigKeyDefaultByStatus = "default_by_status"
	lifecycleMigKeyFrom            = "from"
	lifecycleMigKeyOrigin          = "origin"
	lifecycleMigKeyManual          = "manual"
	lifecycleMigKeyMethod          = "method"
	lifecycleMigKeyMilestoneBased  = "milestone_based"
	lifecycleMigKeyPreconditions   = "preconditions"
	lifecycleMigKeyPostconditions  = "postconditions"
	lifecycleMigKeySystem          = "system"
	lifecycleMigKeyTerminal        = "terminal"
	lifecycleMigKeyTo              = "to"
	lifecycleMigKeyValue           = "value"
)

// lifecycleMigrationHelper provides helper functions for lifecycle migrations
type lifecycleMigrationHelper struct {
	storageProvider storage.ObjectStorageProvider
	projectRoot     string
	logger          logging.Logger
}

// migrateLifecycleFile migrates a single lifecycle file to a lifecycle object
// This is extracted from migrate_lifecycles.go to be reusable
//
//nolint:unparam // force is reserved for future CLI/automation; current call sites pass false.
func (h *lifecycleMigrationHelper) migrateLifecycleFile(
	ctx context.Context,
	lifecyclePath string,
	objectType string,
	version string,
	force bool,
	dryRun bool,
) error {
	secCtx := pkgctx.NewSystemSecurityContext()

	// Load lifecycle file
	data, err := fileutil.ReadFile(lifecyclePath)
	if err != nil {
		return errfmt.Newf("failed to read lifecycle file").Wrap(err)
	}

	var lifecycle objects.Lifecycle
	if err := yaml.Unmarshal(data, &lifecycle); err != nil {
		return errfmt.Newf("failed to parse lifecycle file").Wrap(err)
	}

	// Validate that object_type is set (either from file or parameter)
	if lifecycle.ObjectType == emptyValue && objectType == emptyValue {
		return errfmt.Errorf("object_type is required but not found in file or filename")
	}

	// Use object_type from file if available, otherwise use filename
	if lifecycle.ObjectType != emptyValue {
		objectType = lifecycle.ObjectType
	}

	// Generate lifecycle object ID
	lifecycleID := fmt.Sprintf("LIFECYCLE-%s-%s", objectType, version)

	// Check if object already exists
	if !force {
		// Retry on ID patterns loading error (race condition during concurrent operations)
		var err error
		maxRetries := 5
		retryDelay := 50 * time.Millisecond
		for retry := 0; retry < maxRetries; retry++ {
			_, err = h.storageProvider.Read(ctx, secCtx, lifecycleID)
			if err == nil {
				logging.Fluent(h.logger).Debug("Lifecycle object already exists, skipping").
					ObjectID(lifecycleID).
					String("object_type", objectType).
					Log()
				return nil
			}
			// If error is "not found", that's expected (object doesn't exist yet)
			if err == storage.ErrObjectNotFound || strings.Contains(err.Error(), "not found") {
				break // Object doesn't exist, proceed with creation
			}
			// Retry on ID patterns loading error
			if strings.Contains(err.Error(), "ID patterns are currently loading") {
				if retry < maxRetries-1 {
					time.Sleep(retryDelay)
					retryDelay *= 2 // Exponential backoff
					continue
				}
			}
			// Other errors - fail
			logging.Fluent(h.logger).Error("Failed to check if lifecycle object exists", err).
				ObjectID(lifecycleID).
				String("object_type", objectType).
				Log()
			return errfmt.Newf("failed to check if lifecycle object exists").Wrap(err)
		}
	}

	if dryRun {
		logging.Fluent(h.logger).Info("DRY RUN: Would create lifecycle object").
			ObjectID(lifecycleID).
			String("object_type", objectType).
			File(lifecyclePath).
			Log()
		return nil
	}

	// Convert lifecycle struct to object map using instance builder
	lifecycleObj, err := h.lifecycleToObject(&lifecycle, objectType, version)
	if err != nil {
		return errfmt.Newf("failed to convert lifecycle to object").Wrap(err)
	}

	// Create lifecycle object
	if err := h.storageProvider.Create(ctx, secCtx, lifecycleObj); err != nil {
		if err == storage.ErrObjectExists {
			if force {
				logging.Fluent(h.logger).Warn("Lifecycle object already exists (built-in objects are immutable, skipping)").
					ObjectID(lifecycleID).
					String("object_type", objectType).
					Log()
				return nil
			}
			return nil // Skip existing objects
		}
		return errfmt.Newf("failed to create lifecycle object").Wrap(err)
	}

	logging.Fluent(h.logger).Info("Created lifecycle object").
		ObjectID(lifecycleID).
		String("object_type", objectType).
		Log()

	return nil
}

// lifecycleToObjectFromFile loads a lifecycle file and converts it to an object
func (h *lifecycleMigrationHelper) lifecycleToObjectFromFile(filePath, objectType, version string) (map[string]any, error) {
	// Load lifecycle file
	data, err := fileutil.ReadFile(filePath)
	if err != nil {
		return nil, errfmt.Newf("failed to read lifecycle file").Wrap(err)
	}

	var lifecycle objects.Lifecycle
	if err := yaml.Unmarshal(data, &lifecycle); err != nil {
		return nil, errfmt.Newf("failed to parse lifecycle file").Wrap(err)
	}

	// Use object_type from file if available, otherwise use parameter
	if lifecycle.ObjectType != emptyValue {
		objectType = lifecycle.ObjectType
	}

	return h.lifecycleToObject(&lifecycle, objectType, version)
}

// lifecycleToObject converts a Lifecycle struct to a lifecycle object map using the instance builder
// getObjectTypeAbbreviation converts an object_type to an uppercase abbreviation
// for use in lifecycle IDs (e.g., "backlog_item" -> "BLI", "base_object" -> "BASE")
func getObjectTypeAbbreviation(objectType string) string {
	// Try to get prefix from ID prefix config
	config := validation.GetGlobalIDPrefixesConfig()
	if config != nil {
		prefixes := config.GetPrefixesForKind(objectType)
		if len(prefixes) > 0 {
			// Use the first (primary) prefix, strip trailing "-"
			prefix := prefixes[0]
			prefix = strings.TrimSuffix(prefix, "-")
			// Handle multi-part prefixes like "STRAT-PLAN" - use the full prefix
			return prefix
		}
	}

	// Fallback: Generate abbreviation from object_type
	// Remove common suffixes and use first part
	if strings.HasSuffix(objectType, "_item") {
		base := strings.TrimSuffix(objectType, "_item")
		if len(base) >= 3 {
			return strings.ToUpper(base[:3])
		}
		return strings.ToUpper(base)
	}

	if strings.HasSuffix(objectType, "_metric") {
		base := strings.TrimSuffix(objectType, "_metric")
		parts := strings.Split(base, "_")
		if len(parts) > 0 && len(parts[0]) >= 3 {
			return strings.ToUpper(parts[0][:3])
		}
	}

	// Handle compound words - use first part, up to 4-5 chars if it makes sense
	parts := strings.Split(objectType, "_")
	if len(parts) > 0 {
		firstPart := strings.ToUpper(parts[0])
		// For short words, use full word; for longer, use first 4-5 chars
		if len(firstPart) <= 5 {
			return firstPart
		}
		return firstPart[:5]
	}

	// Last resort: use first 4 uppercase chars
	return strings.ToUpper(objectType[:min(4, len(objectType))])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (h *lifecycleMigrationHelper) lifecycleToObject(lifecycle *objects.Lifecycle, objectType string, _ string) (map[string]any, error) {
	builder := instance_builders.NewForKind(objects.KindLifecycle, objects.DefaultSchemaVersion)

	// Set ID using standardized format: LIFECYCLE-{ABBR}-001
	// This matches the pattern ^[A-Z]+-\d{3,}$ for validation
	abbreviation := getObjectTypeAbbreviation(objectType)
	lifecycleID := fmt.Sprintf("LIFECYCLE-%s-001", abbreviation)
	builder.SetID(lifecycleID)

	// Set title (required by base_object) - derive from object_type
	// Convert object_type like "base_object" to "Base Object Lifecycle"
	titleParts := strings.Split(strings.ReplaceAll(objectType, "_", " "), " ")
	for i, part := range titleParts {
		if len(part) > 0 {
			titleParts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
		}
	}
	title := strings.Join(titleParts, " ") + " Lifecycle"
	builder.SetField(objects.FieldKeyTitle, title)

	// Set status (required by base_object) - lifecycle objects default to "approved"
	// Find initial status from lifecycle statuses if available
	status := "approved"
	if len(lifecycle.Statuses) > 0 {
		for _, s := range lifecycle.Statuses {
			if s.Origin {
				status = s.Value
				break
			}
		}
	}
	builder.SetStatus(status)

	// Set object_type and source_type
	builder.SetField(objects.FieldKeyObjectType, objectType).
		SetField(objects.FieldKeySourceType, string(lifecycleEnum.SourceTypeBuiltIn))

	// Set extends if present
	if lifecycle.Extends != emptyValue {
		builder.SetField(objects.FieldKeyExtends, lifecycle.Extends)
	}

	// Convert status_mapping
	if len(lifecycle.StatusMapping) > 0 {
		statusMapping := make(map[string]any)
		for k, v := range lifecycle.StatusMapping {
			statusMapping[k] = v
		}
		builder.SetField(objects.FieldKeyStatusMapping, statusMapping)
	}

	// Convert statuses ([]objects.Status -> []any)
	if len(lifecycle.Statuses) > 0 {
		statuses := make([]any, len(lifecycle.Statuses))
		for i, status := range lifecycle.Statuses {
			statusMap := map[string]any{
				lifecycleMigKeyValue:    status.Value,
				objects.FieldKeyDisplay: status.Display,
			}
			if status.Origin {
				statusMap[lifecycleMigKeyOrigin] = status.Origin
			}
			if status.Terminal {
				statusMap[lifecycleMigKeyTerminal] = status.Terminal
			}
			if status.Archive {
				statusMap[lifecycleMigKeyArchive] = status.Archive
			}
			if status.System {
				statusMap[lifecycleMigKeySystem] = status.System
			}
			if len(status.Preconditions) > 0 {
				statusMap[lifecycleMigKeyPreconditions] = status.Preconditions
			}
			if status.Description != emptyValue {
				statusMap[objects.FieldKeyDescription] = status.Description
			}
			statuses[i] = statusMap
		}
		// Use SetField directly since statuses is []any (array of objects), not []string
		builder.SetField(objects.FieldKeyStatuses, statuses)
	}

	// Convert transitions ([]objects.Transition -> []any)
	if len(lifecycle.Transitions) > 0 {
		transitions := make([]any, len(lifecycle.Transitions))
		for i, transition := range lifecycle.Transitions {
			transitionMap := map[string]any{
				lifecycleMigKeyFrom:         transition.From,
				lifecycleMigKeyTo:           transition.To,
				objects.FieldKeyDescription: transition.Description,
				lifecycleMigKeyManual:       transition.Manual,
				lifecycleMigKeyAuto:         transition.Auto,
			}
			if len(transition.Preconditions) > 0 {
				transitionMap[lifecycleMigKeyPreconditions] = transition.Preconditions
			}
			if len(transition.Postconditions) > 0 {
				transitionMap[lifecycleMigKeyPostconditions] = transition.Postconditions
			}
			transitions[i] = transitionMap
		}
		// Use SetField directly since transitions is []any (array of objects), not []string
		builder.SetField(objects.FieldKeyTransitions, transitions)
	}

	// Convert percent_complete
	if lifecycle.PercentComplete.Method != emptyValue || len(lifecycle.PercentComplete.DefaultByStatus) > 0 || len(lifecycle.PercentComplete.MilestoneBased) > 0 {
		percentComplete := map[string]any{
			lifecycleMigKeyMethod: lifecycle.PercentComplete.Method,
		}
		if len(lifecycle.PercentComplete.DefaultByStatus) > 0 {
			percentComplete[lifecycleMigKeyDefaultByStatus] = lifecycle.PercentComplete.DefaultByStatus
		}
		if len(lifecycle.PercentComplete.MilestoneBased) > 0 {
			percentComplete[lifecycleMigKeyMilestoneBased] = lifecycle.PercentComplete.MilestoneBased
		}
		builder.SetField(objects.FieldKeyPercentComplete, percentComplete)
	}

	// Build the object
	obj, err := builder.Build()
	if err != nil {
		return nil, errfmt.Newf("failed to build lifecycle object").Wrap(err)
	}

	return obj, nil
}

// transformLifecycleID transforms a lifecycle object's ID from old format to new format
// Old format: LIFECYCLE-{object_type}-{version} (e.g., LIFECYCLE-backlog_item-v1_0_0)
// New format: LIFECYCLE-{ABBR}-001 (e.g., LIFECYCLE-BLI-001)
// Also ensures required base_object fields (title, status) are present
func (h *lifecycleMigrationHelper) transformLifecycleID(obj map[string]any) (map[string]any, error) {
	// Create a copy to avoid modifying the original
	transformed := make(map[string]any)
	maps.Copy(transformed, obj)

	// Extract object_type from the object
	objectType, ok := transformed[objects.FieldKeyObjectType].(string)
	if !ok || objectType == emptyValue {
		return nil, errfmt.Errorf("object missing object_type field")
	}

	// Generate new ID using abbreviation
	abbreviation := getObjectTypeAbbreviation(objectType)
	newID := fmt.Sprintf("LIFECYCLE-%s-001", abbreviation)

	// Get old ID for logging
	oldID := ""
	if id, ok := obj[objects.FieldKeyID].(string); ok {
		oldID = id
	}

	// Update the ID field
	transformed[objects.FieldKeyID] = newID

	// Ensure required base_object fields are present
	// Set title if missing (required by base_object)
	if _, hasTitle := transformed[objects.FieldKeyTitle]; !hasTitle {
		// Derive title from object_type: "base_object" -> "Base Object Lifecycle"
		titleParts := strings.Split(strings.ReplaceAll(objectType, "_", " "), " ")
		for i, part := range titleParts {
			if len(part) > 0 {
				titleParts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
			}
		}
		title := strings.Join(titleParts, " ") + " Lifecycle"
		transformed[objects.FieldKeyTitle] = title
	}

	// Set status if missing (required by base_object)
	// Lifecycle objects use base_object lifecycle, not the lifecycle they define
	// Valid statuses: "proposed", "approved", "in_progress", "implemented", "archived", "error"
	if _, hasStatus := transformed[objects.FieldKeyStatus]; !hasStatus {
		// Default to "approved" for lifecycle objects (they extend base_object)
		transformed[objects.FieldKeyStatus] = "approved"
	} else {
		// If status exists, validate it's a valid base_object status
		// If not, override to "approved"
		status, ok := transformed[objects.FieldKeyStatus].(string)
		if !ok {
			transformed[objects.FieldKeyStatus] = "approved"
		} else {
			validStatuses := map[string]bool{
				"proposed":    true,
				"approved":    true,
				"in_progress": true,
				"implemented": true,
				"archived":    true,
				"error":       true,
			}
			if !validStatuses[status] {
				// Invalid status for lifecycle object - override to "approved"
				transformed[objects.FieldKeyStatus] = "approved"
			}
		}
	}

	logging.Fluent(h.logger).Debug("Transformed lifecycle ID").
		String("object_type", objectType).
		String("old_id", oldID).
		String("new_id", newID).
		Log()

	return transformed, nil
}
