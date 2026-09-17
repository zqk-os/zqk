package utility

import (
	"context"
	"fmt"
	"strings"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// generateScenarioID generates a scenario ID from name
func generateScenarioID(scenarioName string) string {
	return fmt.Sprintf("SCN-%s", strings.ToUpper(strings.ReplaceAll(scenarioName, "-", "")))
}

// findExistingScenarioByIDOrName finds an existing scenario by ID or name
func (sb *ScenarioBuilder) findExistingScenarioByIDOrName(ctx context.Context, scenarioID, scenarioName string) (map[string]any, string, bool) {
	// If scenarioID is provided and valid, try to read it
	if scenarioID != emptyValue {
		existingScenario, err := sb.storage.Read(ctx, sb.secCtx, scenarioID)
		if err == nil {
			return existingScenario, scenarioID, true
		}
	}

	// Otherwise, search by title
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind: objects.KindScenario,
		Filters: map[string]any{
			objects.FieldKeyTitle: map[string]any{"$contains": scenarioName},
		},
		Limit: 1,
	}
	result, listErr := sb.storage.List(ctx, sb.secCtx, storageCtx, filter)
	if listErr == nil && len(result.Objects) > 0 {
		existingScenario := result.Objects[0]
		if id, ok := existingScenario[objects.FieldKeyID].(string); ok {
			return existingScenario, id, true
		}
	}

	return nil, "", false
}

// getInitialStatusForScenario gets the initial status for a scenario
func getInitialStatusForScenario() string {
	lifecycleLoader := objects.GetGlobalLifecycleLoader()
	lifecycle, _ := lifecycleLoader.LoadLifecycle(scenarioBuilderKindScenario) //nolint:errcheck
	initialStatus := scenarioBuilderStatusProposed
	if lifecycle != nil && len(lifecycle.Statuses) > 0 {
		for _, status := range lifecycle.Statuses {
			if status.Origin {
				initialStatus = status.Value
				break
			}
		}
	}
	return initialStatus
}

// buildNewScenarioObject builds a new scenario object
// Note: ID is not set - storage will auto-generate it with proper format (SCN-###)
func (sb *ScenarioBuilder) buildNewScenarioObject(scenarioName, scenarioID, initialStatus string) map[string]any {
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	scenarioObj := map[string]any{
		objects.FieldKeyKind:                 scenarioBuilderKindScenario,
		objects.FieldKeyTitle:                fmt.Sprintf("Test Scenario: %s", scenarioName),
		objects.FieldKeyDescription:          fmt.Sprintf("Test scenario for %s", scenarioName),
		objects.FieldKeyObjective:            fmt.Sprintf("Automated test scenario for %s", scenarioName),
		objects.FieldKeyStatus:               initialStatus,
		objects.FieldKeyDataGenerationConfig: sb.buildDataGenerationConfig(),
		objects.FieldKeyCreatedAt:            now,
		objects.FieldKeyCreatedBy:            scenarioBuilderAccountSystem,
		objects.FieldKeyUpdatedAt:            now,
		objects.FieldKeyUpdatedBy:            scenarioBuilderAccountSystem,
		objects.FieldKeySchemaVersion:        scenarioBuilderSchemaV2,
		objects.FieldKeyOriginProject:        scenarioBuilderOriginZQK,
		objects.FieldKeyOriginSystem:         scenarioBuilderOriginZQK,
		objects.FieldKeyNamespaceID:          "zqk:kernel",
	}

	// Only set ID if explicitly provided (for updates)
	// For new scenarios, let storage auto-generate with proper format
	if scenarioID != emptyValue {
		scenarioObj[objects.FieldKeyID] = scenarioID
	}

	return scenarioObj
}

// createNewScenario creates a new scenario object
func (sb *ScenarioBuilder) createNewScenario(ctx context.Context, scenarioName, scenarioID string) error {
	initialStatus := getInitialStatusForScenario()
	scenarioObj := sb.buildNewScenarioObject(scenarioName, scenarioID, initialStatus)

	if err := sb.storage.Create(ctx, sb.secCtx, scenarioObj); err != nil {
		if strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "object already exists") {
			// If creation fails due to conflict, try to find existing scenario
			existingScenario, _, exists := sb.findExistingScenarioByIDOrName(ctx, "", scenarioName)
			if exists {
				return sb.updateExistingScenario(ctx, existingScenario)
			}
			return errfmt.Newf("scenario already exists but could not be found").Wrap(err)
		}
		return errfmt.Newf("failed to create scenario object").Wrap(err)
	}

	// Get the auto-generated ID from the created object
	createdID := "auto-generated"
	if id, ok := scenarioObj[objects.FieldKeyID].(string); ok && id != emptyValue {
		createdID = id
	} else {
		// Try to read it back to get the generated ID
		// This is a workaround - ideally storage.Create would return the created object
		_, foundID, exists := sb.findExistingScenarioByIDOrName(ctx, "", scenarioName)
		if exists {
			createdID = foundID
		}
	}

	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		fmt.Sprintf("Created scenario object %s", createdID),
		map[string]any{"scenario_id": createdID})
	return nil
}

// updateScenarioOnConflict updates scenario when create fails due to conflict
func (sb *ScenarioBuilder) updateScenarioOnConflict(ctx context.Context, scenarioName, scenarioID string) error {
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	updates := map[string]any{
		objects.FieldKeyTitle:                fmt.Sprintf("Test Scenario: %s", scenarioName),
		objects.FieldKeyDescription:          fmt.Sprintf("Test scenario for %s", scenarioName),
		objects.FieldKeyObjective:            fmt.Sprintf("Automated test scenario for %s", scenarioName),
		objects.FieldKeyDataGenerationConfig: sb.buildDataGenerationConfig(),
		objects.FieldKeyUpdatedAt:            now,
		objects.FieldKeyUpdatedBy:            scenarioBuilderAccountSystem,
	}
	if err := sb.storage.Update(ctx, sb.secCtx, scenarioID, updates); err != nil {
		return errfmt.Newf("failed to update scenario object").Wrap(err)
	}
	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		fmt.Sprintf("Updated scenario object %s", scenarioID),
		map[string]any{"scenario_id": scenarioID})
	return nil
}

// updateExistingScenario updates an existing scenario object
func (sb *ScenarioBuilder) updateExistingScenario(ctx context.Context, existingScenario map[string]any) error {
	now := zqktime.NowLayoutUTC(zqktime.LayoutObjectDateTimeZ)
	updates := map[string]any{
		objects.FieldKeyDataGenerationConfig: sb.buildDataGenerationConfig(),
		objects.FieldKeyUpdatedAt:            now,
		objects.FieldKeyUpdatedBy:            scenarioBuilderAccountSystem,
	}
	scenarioID := existingScenario[objects.FieldKeyID].(string)
	if err := sb.storage.Update(ctx, sb.secCtx, scenarioID, updates); err != nil {
		return errfmt.Newf("failed to update scenario object").Wrap(err)
	}
	sb.emitCoordinatorEvent(ctx, ScenarioBuilderProfileName, scenarioBuilderStatusProgress,
		"Updated scenario object",
		map[string]any{"scenario_id": scenarioID})
	return nil
}
