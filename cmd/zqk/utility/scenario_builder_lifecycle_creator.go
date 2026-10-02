package utility

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// createLifecycleObjects creates lifecycle objects from YAML files
func (sb *ScenarioBuilder) createLifecycleObjects(ctx context.Context) error {
	lifecyclesDir := filepath.Join(sb.config.TargetDir, paths.ProcessInternalLifecyclesDir)

	// Check if lifecycles directory exists
	if _, err := fileutil.Stat(lifecyclesDir); fileutil.IsNotExist(err) {
		logging.FluentEvent(sb.logger).Debug("Lifecycles directory not found, skipping lifecycle object creation").
			String("dir", lifecyclesDir).
			Log()
		return nil
	}

	// Scan for lifecycle files
	entries, err := fileutil.ReadDir(lifecyclesDir)
	if err != nil {
		return errfmt.Newf("failed to read lifecycles directory").Wrap(err)
	}

	created := 0
	skipped := 0
	errors := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, "_lifecycle.yaml") && !strings.HasSuffix(name, "_lifecycle.yml") {
			continue
		}
		// Skip backup files
		if strings.HasSuffix(name, ".bak") {
			continue
		}

		lifecyclePath := filepath.Join(lifecyclesDir, name)

		// Extract object_type from filename
		objectType := strings.TrimSuffix(name, "_lifecycle.yaml")
		objectType = strings.TrimSuffix(objectType, "_lifecycle.yml")

		// Create lifecycle object (use 001 format for ID)
		if err := sb.createLifecycleObjectFromFile(ctx, lifecyclePath, objectType, "001"); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				skipped++
				continue
			}
			// Emit warning for first few errors
			if errors < 3 {
				sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
					fmt.Sprintf("Failed to create lifecycle for %s: %v", objectType, err),
					map[string]any{objects.FieldKeyObjectType: objectType, "file": name, "error": err.Error()})
			}
			logging.FluentEvent(sb.logger).Warn("Failed to create lifecycle object").
				String("file", name).
				String("object_type", objectType).
				WithError(err).
				Log()
			errors++
			continue
		}

		created++
	}

	if created > 0 || skipped > 0 || errors > 0 {
		resultMsg := fmt.Sprintf("Lifecycle objects: %d created", created)
		fields := map[string]any{"created": created}
		if skipped > 0 {
			resultMsg += fmt.Sprintf(", %d skipped (existing)", skipped)
			fields[objects.FieldKeySkipped] = skipped
		}
		if errors > 0 {
			resultMsg += fmt.Sprintf(", %d errors", errors)
			fields["errors"] = errors
		}
		sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress, resultMsg, fields)
	}

	return nil
}

// createLifecycleObjectFromFile creates a lifecycle object from a YAML file
func (sb *ScenarioBuilder) createLifecycleObjectFromFile(ctx context.Context, lifecyclePath, objectType, version string) error {
	lifecycle, err := objects.LoadLifecycleFile(lifecyclePath)
	if err != nil {
		return err
	}

	// Use object_type from file if available
	if lifecycle.ObjectType != emptyValue {
		objectType = lifecycle.ObjectType
	}

	// Generate lifecycle object ID
	// Format: LIFECYCLE-{ABBREV}-{NUMERIC} where ABBREV is the object type abbreviation
	// Pattern: ^LIFECYCLE-[A-Z]+(-[A-Z]+)*-\d{3,}$
	// For example: account -> LIFECYCLE-ACC-001, backlog_item -> LIFECYCLE-BLI-001
	abbrev := getObjectTypeAbbreviationForLifecycle(objectType)
	lifecycleID := fmt.Sprintf("LIFECYCLE-%s-%s", abbrev, version)

	// Check if object already exists
	_, err = sb.storage.Read(ctx, sb.secCtx, lifecycleID)
	if err == nil {
		// Object already exists, skip
		return errfmt.Errorf("lifecycle object already exists: %s", lifecycleID)
	}

	// Convert lifecycle struct to object map using instance builder
	lifecycleObj, err := lifecycleToObjectMap(lifecycle, objectType, version)
	if err != nil {
		return errfmt.Newf("failed to convert lifecycle to object").Wrap(err)
	}

	// Create lifecycle object
	if err := sb.storage.Create(ctx, sb.secCtx, lifecycleObj); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "duplicate") {
			return errfmt.Errorf("lifecycle object already exists: %s", lifecycleID)
		}
		return errfmt.Newf("failed to create lifecycle object").Wrap(err)
	}

	return nil
}

// lifecycleToObjectMap converts a Lifecycle struct to a lifecycle object map
func lifecycleToObjectMap(lifecycle *objects.Lifecycle, objectType, version string) (map[string]any, error) {
	builder := instance_builders.NewForKind(objects.KindLifecycle, scenarioBuilderSchemaV2)

	// Set ID (use abbreviation format)
	abbrev := getObjectTypeAbbreviationForLifecycle(objectType)
	lifecycleID := fmt.Sprintf("LIFECYCLE-%s-%s", abbrev, version)
	builder.SetID(lifecycleID)

	// Set object_type and source_type
	builder.SetField(objects.FieldKeyObjectType, objectType)
	builder.SetField(objects.FieldKeySourceType, scenarioBuilderSourceBuiltIn)

	// Set title (required by base_object) - derive from object_type
	builder.SetField(objects.FieldKeyTitle, objects.DeriveLifecycleTitle(objectType))

	// Set status (required by base_object) - lifecycle objects use base_object lifecycle
	// Valid statuses: "proposed" (initial), "approved", "in_progress", "implemented", "archived", "error"
	// Default to "proposed" which is the initial status for base_object
	builder.SetStatus(scenarioBuilderStatusProposed)

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

	// Convert statuses
	if len(lifecycle.Statuses) > 0 {
		statuses := make([]any, len(lifecycle.Statuses))
		for i, status := range lifecycle.Statuses {
			statusMap := map[string]any{
				"value":                 status.Value,
				objects.FieldKeyDisplay: status.Display,
			}
			if status.Origin {
				statusMap[objects.FieldKeyOrigin] = status.Origin
			}
			if status.Terminal {
				statusMap["terminal"] = status.Terminal
			}
			if status.Archive {
				statusMap["archive"] = status.Archive
			}
			if status.System {
				statusMap["system"] = status.System
			}
			if len(status.Preconditions) > 0 {
				statusMap["preconditions"] = status.Preconditions
			}
			if status.Description != emptyValue {
				statusMap[objects.FieldKeyDescription] = status.Description
			}
			statuses[i] = statusMap
		}
		// Use SetField directly since statuses is []any (array of objects), not []string
		builder.SetField(objects.FieldKeyStatuses, statuses)
	}

	// Convert transitions
	if len(lifecycle.Transitions) > 0 {
		transitions := make([]any, len(lifecycle.Transitions))
		for i, transition := range lifecycle.Transitions {
			transitionMap := map[string]any{
				"from":                      transition.From,
				"to":                        transition.To,
				objects.FieldKeyDescription: transition.Description,
				"manual":                    transition.Manual,
				"auto":                      transition.Auto,
			}
			if len(transition.Preconditions) > 0 {
				transitionMap["preconditions"] = transition.Preconditions
			}
			transitions[i] = transitionMap
		}
		// Use SetField directly since transitions is []any (array of objects), not []string
		builder.SetField(objects.FieldKeyTransitions, transitions)
	}

	// Convert percent_complete
	if lifecycle.PercentComplete.Method != emptyValue || len(lifecycle.PercentComplete.DefaultByStatus) > 0 || len(lifecycle.PercentComplete.MilestoneBased) > 0 {
		percentComplete := map[string]any{
			objects.FieldKeyMethod: lifecycle.PercentComplete.Method,
		}
		if len(lifecycle.PercentComplete.DefaultByStatus) > 0 {
			percentComplete["default_by_status"] = lifecycle.PercentComplete.DefaultByStatus
		}
		if len(lifecycle.PercentComplete.MilestoneBased) > 0 {
			percentComplete["milestone_based"] = lifecycle.PercentComplete.MilestoneBased
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

// getObjectTypeAbbreviationForLifecycle converts object_type to abbreviation for lifecycle IDs
func getObjectTypeAbbreviationForLifecycle(objectType string) string {
	// Map common object types to their abbreviations (matching existing lifecycle objects)
	abbrevMap := map[string]string{
		objects.KindAccount:            "ACC",
		objects.KindWorkstream:         "WS",
		objects.KindGoal:               "GOAL",
		objects.KindMilestone:          "MIL",
		objects.KindCriteria:           "CRIT",
		objects.KindRequirement:        "REQ",
		scenarioBuilderKindBacklogItem: "BLI",
		objects.KindDocEntry:           "DOC",
		objects.KindBaseObject:         "BASE",
	}

	if abbrev, ok := abbrevMap[objectType]; ok {
		return abbrev
	}

	// Default: convert underscores to hyphens and uppercase
	// e.g., "test_case" -> "TEST-CASE"
	upper := strings.ToUpper(strings.ReplaceAll(objectType, "_", "-"))
	return upper
}
