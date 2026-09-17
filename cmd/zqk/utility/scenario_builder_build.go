package utility

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	instancebuilders "github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// BuildFromObjects builds a scenario by copying objects from the main project
func (sb *ScenarioBuilder) BuildFromObjects(ctx context.Context) error {
	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Building scenario from project objects",
		map[string]any{
			"target_dir":                sb.copyConfig.TargetDir,
			objects.FieldKeyObjectCount: len(sb.copyConfig.ObjectIDs),
		})

	// Ensure target directory exists
	if err := fileutil.MkdirAll(sb.copyConfig.TargetDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create target directory").Wrap(err)
	}

	// Setup basic infrastructure (directory structure)
	// NOTE: We no longer copy files - system loaders will find specs/lifecycles from main project
	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress, "Setting up infrastructure...", nil)
	if err := sb.setupInfrastructure(); err != nil {
		return errfmt.Newf("failed to setup infrastructure").Wrap(err)
	}

	// Copy objects from project
	return sb.CopyObjectsFromProject(ctx, sb.copyConfig)
}

// Build generates the scenario data
func (sb *ScenarioBuilder) Build(ctx context.Context) error {
	// Check if we're copying objects from project
	if sb.copyConfig != nil && len(sb.copyConfig.ObjectIDs) > 0 {
		return sb.BuildFromObjects(ctx)
	}

	// Check if we're loading from data file (handled separately)
	if len(sb.dataFileObjects) > 0 {
		return sb.BuildFromDataFile(ctx)
	}

	if len(sb.config.Kinds) == 0 {
		return errfmt.Errorf("no object kinds specified")
	}

	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Building test scenario",
		map[string]any{
			"target_dir": sb.config.TargetDir,
			"kinds":      len(sb.config.Kinds),
		})

	// Ensure target directory exists
	if err := fileutil.MkdirAll(sb.config.TargetDir, paths.DirPerm755); err != nil {
		return errfmt.Newf("failed to create target directory").Wrap(err)
	}

	// Copy required infrastructure files (specs, lifecycles, configs)
	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress, "Setting up infrastructure (specs, lifecycles, configs)...", nil)
	if err := sb.setupInfrastructure(); err != nil {
		return errfmt.Newf("failed to setup infrastructure").Wrap(err)
	}

	// Create or update scenario object with data generation config
	// Only do this if we didn't load from an existing scenario object
	// (if we loaded from one, it already exists and we don't need to update it)
	if sb.scenarioID == emptyValue {
		scenarioName := filepath.Base(sb.config.TargetDir)
		if err := sb.createOrUpdateScenarioObject(ctx, scenarioName, ""); err != nil {
			return errfmt.Newf("failed to create/update scenario object").Wrap(err)
		}
	} else {
		sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			fmt.Sprintf("Using existing scenario object %s", sb.scenarioID),
			map[string]any{"scenario_id": sb.scenarioID})
	}

	// Generate objects for each kind
	for _, kind := range sb.config.Kinds {
		count := sb.config.Counts[kind]
		if count == 0 {
			count = sb.config.DefaultCount
		}
		diversity := sb.config.Diversity[kind]
		if diversity == 0 {
			diversity = sb.config.DefaultDiversity
		}

		if err := sb.generateObjectsForKind(ctx, kind, count, diversity); err != nil {
			return errfmt.Errorf("failed to generate %s objects: %w", kind, err)
		}
	}

	// Create links between objects if enabled
	if sb.config.LinkProbability > 0 {
		if err := sb.createLinks(ctx); err != nil {
			sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
				"Failed to create links",
				map[string]any{"error": err})
			// Don't fail - links are optional
		}
	}

	totalGenerated := 0
	for kind, count := range sb.generated {
		totalGenerated += count
		sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
			fmt.Sprintf("Generated %d %s objects", count, kind),
			map[string]any{objects.FieldKeyKind: kind, "count": count})
	}

	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusComplete,
		fmt.Sprintf("Scenario build complete: %d total objects across %d kinds", totalGenerated, len(sb.generated)),
		map[string]any{"total_objects": totalGenerated, "total_kinds": len(sb.generated)})

	return nil
}

// generateObjectsForKind generates objects of a specific kind
func (sb *ScenarioBuilder) generateObjectsForKind(ctx context.Context, kind string, count, diversity int) error {
	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		fmt.Sprintf("Generating %d %s objects (diversity: %d)...", count, kind, diversity),
		map[string]any{objects.FieldKeyKind: kind, "count": count, "diversity": diversity})

	// Load spec to understand required fields
	specLoader := objects.GetGlobalSpecLoader()
	spec, err := specLoader.LoadSpecWithInheritance(kind + ".yaml")
	if err != nil {
		return errfmt.Errorf("failed to load spec for %s: %w", kind, err)
	}

	// Load lifecycle to get valid statuses
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	lifecycle, _ := lifecycleLoader.LoadLifecycle(kind) //nolint:errcheck // Use defaults if load fails

	// Get valid statuses from lifecycle, prioritizing initial statuses
	validStatuses := []string{scenarioBuilderStatusExploring} // Default fallback
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

	// Generate objects
	// Note: IDs will be auto-generated by storage.Create() if not provided
	for i := 0; i < count; i++ {
		obj := sb.generateObject(kind, i, diversity, spec, validStatuses)

		// Don't set ID - let storage generate it automatically
		// This ensures IDs are unique and follow the correct format

		// Create object via storage
		if err := sb.storage.Create(ctx, sb.secCtx, obj); err != nil {
			sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusWarning,
				"Failed to create object",
				map[string]any{
					objects.FieldKeyKind: kind,
					"index":              i,
					"error":              err,
				})
			// Continue with next object
			continue
		}

		sb.generated[kind]++

		// Progress update every 10 objects
		if (i+1)%10 == 0 {
			sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
				fmt.Sprintf("Progress: %d/%d %s objects created", i+1, count, kind),
				map[string]any{objects.FieldKeyKind: kind, "current": i + 1, "total": count})
		}
	}

	return nil
}

// generateObject creates a single object with variation based on diversity
func (sb *ScenarioBuilder) generateObject(kind string, index, diversity int, spec *objects.Spec, validStatuses []string) map[string]any {
	// Get schema version from spec
	schemaVersion := objects.DefaultSchemaVersion // Default fallback
	if spec != nil && spec.SchemaVersion != emptyValue {
		schemaVersion = spec.SchemaVersion
	}

	// Get instance builder from registry
	registry := instancebuilders.GetGlobalRegistry()
	builder, err := registry.GetBuilder(kind, schemaVersion)
	if err != nil {
		logging.FluentEvent(sb.logger).Warn("Failed to get instance builder, using minimal object").
			String("kind", kind).
			String("schema_version", schemaVersion).
			WithError(err).
			Log()
		// Return minimal object if builder not available
		return map[string]any{
			objects.FieldKeyKind:          kind,
			objects.FieldKeyTitle:         fixtureScenarioObjectTitle(kind, index+1),
			objects.FieldKeySchemaVersion: schemaVersion,
		}
	}

	// Use instance builder
	// Basic fields
	builder.SetField(objects.FieldKeyTitle, fixtureScenarioObjectTitle(kind, index+1))
	builder.SetField(objects.FieldKeyDescription, fmt.Sprintf("Generated scenario fixture for %s", kind))

	// Status (distribute across valid statuses based on diversity)
	statusIndex := index % len(validStatuses)
	if diversity > 5 {
		// Higher diversity: more variation in status
		statusIndex = (index * diversity) % len(validStatuses)
	}
	builder.SetStatus(validStatuses[statusIndex])

	// Temporal fields (distribute across time range)
	timeOffset := time.Duration(index) * sb.config.TimeRange / time.Duration(sb.config.DefaultCount)
	createdAt := sb.config.StartTime.Add(timeOffset).UTC()
	// Format must be exactly YYYY-MM-DDTHH:MM:SSZ (no milliseconds, no timezone offset)
	builder.SetField(objects.FieldKeyCreatedAt, createdAt.Format("2006-01-02T15:04:05Z"))
	builder.SetField(objects.FieldKeyUpdatedAt, createdAt.Format("2006-01-02T15:04:05Z"))
	builder.SetField(objects.FieldKeyCreatedBy, pkgctx.SystemAccountID)
	builder.SetField(objects.FieldKeyUpdatedBy, pkgctx.SystemAccountID)

	// Schema and origin
	builder.SetField(objects.FieldKeyOriginProject, "zqk")
	builder.SetField(objects.FieldKeyOriginSystem, "zqk")
	builder.SetField(objects.FieldKeyNamespaceID, "zqk:kernel")

	// Add kind-specific fields based on diversity
	sb.addKindSpecificFieldsToBuilder(builder, kind, index, diversity, spec)

	// Set a temporary ID for Build() - storage will auto-generate the real ID
	// Build() requires an ID, but we want storage to generate it with proper format
	tempID := fmt.Sprintf("TEMP-%s-%d", strings.ToUpper(kind), index+1)
	builder.SetField(objects.FieldKeyID, tempID)

	// Build instance
	instance, err := builder.Build()
	if err != nil {
		logging.FluentEvent(sb.logger).Warn("Failed to build instance, using minimal object").
			String("kind", kind).
			WithError(err).
			Log()
		// Return minimal object if build fails
		return map[string]any{
			objects.FieldKeyKind:          kind,
			objects.FieldKeyTitle:         fixtureScenarioObjectTitle(kind, index+1),
			objects.FieldKeySchemaVersion: schemaVersion,
		}
	}

	// Remove temporary ID - let storage auto-generate with proper format
	delete(instance, objects.FieldKeyID)

	return instance
}

// addKindSpecificFieldsToBuilder adds fields specific to each object kind to builder
func (sb *ScenarioBuilder) addKindSpecificFieldsToBuilder(builder instancebuilders.InstanceBuilder, kind string, index, diversity int, _ *objects.Spec) {
	switch kind {
	case objects.KindAccount:
		// Account requires username field
		username := fmt.Sprintf("testuser%d", index+1)
		builder.SetField(objects.FieldKeyUsername, username)
		// Account status is separate from base object status
		accountStatuses := []string{scenarioBuilderStatusActive, scenarioBuilderStatusInactive, scenarioBuilderStatusSuspended}
		builder.SetField(objects.FieldKeyStatus, accountStatuses[index%len(accountStatuses)])
	case scenarioBuilderKindBacklogItem:
		// Use valid priority tiers
		priorityTiers := []string{"P0", "P1", "P2", "P3"}
		builder.SetField(objects.FieldKeyPriorityTier, priorityTiers[index%len(priorityTiers)])
		if diversity > 3 {
			builder.SetField(objects.FieldKeyBenefits, []string{
				fmt.Sprintf("Benefit %d", index+1),
				fmt.Sprintf("Benefit %d", index+2),
			})
			builder.SetField(objects.FieldKeyCriteriaRefs, []string{"CRIT-001"})
			builder.SetField(objects.FieldKeyOwnerRef, "ACC-001")
		}
	case objects.KindGoal:
		// Goal target_date must be in YYYY-MM-DD format
		builder.SetField(objects.FieldKeyTargetDate, time.Now().Add(time.Duration(index*7)*24*time.Hour).Format("2006-01-02"))
	case objects.KindMilestone:
		// Milestone target_date must be in YYYY-MM-DD format
		builder.SetField(objects.FieldKeyTargetDate, time.Now().Add(time.Duration(index*14)*24*time.Hour).Format("2006-01-02"))
	case objects.KindWorkstream:
		// Workstream requires owner_ref and entry_point
		// Use first account (ACC-001) - will be created before workstream
		builder.SetField(objects.FieldKeyOwnerRef, "ACC-001")
		builder.SetField(objects.FieldKeyEntryPoint, fmt.Sprintf("entry-point-%d", index+1))
		builder.SetField(objects.FieldKeyPercentComplete, (index*10)%100)
	case objects.KindRequirement:
		builder.SetField(objects.FieldKeyPriority, fmt.Sprintf("P%d", (index%3)+1))
		// Requirement requires goal_refs (minCount: 1) and criteria_refs (min_length: 1, required: true)
		// Use first goal (GOAL-001) - goals are created before requirements
		builder.SetField(objects.FieldKeyGoalRefs, []string{"GOAL-001"})
		// Use criteria reference - criteria should be created before requirements
		// For now, reference CRIT-001 (will be created if criteria kind is included)
		builder.SetField(objects.FieldKeyCriteriaRefs, []string{"CRIT-001"})
		if diversity > 5 {
			builder.SetField(objects.FieldKeyAcceptanceCriteria, []string{
				fmt.Sprintf("Criterion %d", index+1),
			})
		}
	case objects.KindCriteria:
		// Criteria requires category field
		categories := []string{"functional", "non-functional", "acceptance", "test", "performance", "security", "compliance"}
		builder.SetField(objects.FieldKeyCategory, categories[index%len(categories)])
		// Note: requirement case is complete here, don't add another case below
	case objects.KindTestCase:
		builder.SetField(objects.FieldKeyTestType, []string{"unit", "integration", "e2e"}[(index*3)%3])
		if diversity > 4 {
			builder.SetField(objects.FieldKeyExpectedResult, fmt.Sprintf("Expected result %d", index+1))
		}
	case objects.KindAuditEvent:
		builder.SetField(objects.FieldKeyEventType, []string{"create", "update", "delete"}[(index*3)%3])
		builder.SetField(objects.FieldKeySeverity, []string{"low", "medium", "high"}[(index*3)%3])
		builder.SetField(objects.FieldKeyOperation, fmt.Sprintf("Test operation %d", index+1))
	case objects.KindChangeJournalEntry:
		builder.SetField(objects.FieldKeyChangeType, []string{"create", "update", "delete"}[(index*3)%3])
		builder.SetField(objects.FieldKeyFieldName, fmt.Sprintf("field_%d", index+1))
		builder.SetField(objects.FieldKeyOldValue, fmt.Sprintf("old_%d", index))
		builder.SetField(objects.FieldKeyNewValue, fmt.Sprintf("new_%d", index+1))
	case objects.KindDocEntry:
		// Doc entry requires path and summary fields
		builder.SetField(objects.FieldKeyPath, fmt.Sprintf("docs/test-doc-%d.md", index+1))
		builder.SetField(objects.FieldKeySummary, fmt.Sprintf("Test document entry %d for scenario", index+1))
	}
}

// createLinks creates relationships between generated objects
//
//nolint:unparam // Always returns nil error - function is designed to always succeed
func (sb *ScenarioBuilder) createLinks(_ context.Context) error {
	// This is a simplified implementation
	// In a full implementation, we would:
	// 1. Query generated objects
	// 2. Create links based on LinkProbability
	// 3. Update objects with relationship fields

	logging.FluentEvent(sb.logger).Debug("Creating links between objects").
		String("probability", fmt.Sprintf("%.2f", sb.config.LinkProbability)).
		Log()

	// For now, just log that links would be created
	// Full implementation would require querying and updating objects
	return nil
}

func fixtureScenarioObjectTitle(kind string, seq int) string {
	return fmt.Sprintf("Fixture: %s #%d", kind, seq)
}
