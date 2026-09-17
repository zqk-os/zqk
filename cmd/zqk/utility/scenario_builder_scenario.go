package utility

import (
	"context"
	"strings"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/storage"
)

// createOrUpdateScenarioObject creates or updates a scenario object with data generation config
func (sb *ScenarioBuilder) createOrUpdateScenarioObject(ctx context.Context, scenarioName string, existingScenarioID string) error {
	// Don't generate ID - let storage auto-generate it with proper format
	// The generateScenarioID function creates invalid IDs (e.g., SCN-AIJOBSEARCHTOOL)
	// which don't match the required format (SCN-###)

	existingScenario, _, scenarioExists := sb.findExistingScenarioByIDOrName(ctx, existingScenarioID, scenarioName)

	if !scenarioExists {
		// Pass empty string to let storage auto-generate ID
		return sb.createNewScenario(ctx, scenarioName, "")
	}

	return sb.updateExistingScenario(ctx, existingScenario)
}

// buildDataGenerationConfig builds the data generation config from builder config
func (sb *ScenarioBuilder) buildDataGenerationConfig() map[string]any {
	config := map[string]any{
		"kinds":             sb.config.Kinds,
		"counts":            sb.config.Counts,
		"default_count":     sb.config.DefaultCount,
		"diversity":         sb.config.Diversity,
		"default_diversity": sb.config.DefaultDiversity,
		"link_probability":  sb.config.LinkProbability,
		"time_range":        sb.config.TimeRange.String(),
		"start_time":        sb.config.StartTime.Format(time.RFC3339),
	}
	return config
}

// LoadFromScenarioObject loads configuration from a scenario object
func (sb *ScenarioBuilder) LoadFromScenarioObject(ctx context.Context, scenarioID string) error {
	// Read scenario object
	scenarioObj, err := readScenarioObject(ctx, sb, scenarioID)
	if err != nil {
		return err
	}

	// Extract data generation config
	configMap, err := extractDataGenerationConfig(scenarioObj, scenarioID)
	if err != nil {
		return err
	}

	// Apply config to builder
	applyConfigToBuilder(sb, configMap)

	return nil
}

// findScenarioByName finds a scenario object by name (title or ID)
func (sb *ScenarioBuilder) findScenarioByName(ctx context.Context, scenarioName string) (string, error) {
	// Try to list scenarios and find by title
	storageCtx := pkgctx.NewStorageContext()
	filter := storage.ListFilter{
		Kind: objects.KindScenario,
		Filters: map[string]any{
			objects.FieldKeyTitle: map[string]any{"$contains": scenarioName},
		},
		Limit: 10,
	}

	result, err := sb.storage.List(ctx, sb.secCtx, storageCtx, filter)
	if err != nil {
		return "", errfmt.Newf("failed to list scenarios").Wrap(err)
	}

	// Find exact match
	for _, obj := range result.Objects {
		if title, ok := obj[objects.FieldKeyTitle].(string); ok {
			if strings.Contains(strings.ToLower(title), strings.ToLower(scenarioName)) {
				if id, ok := obj[objects.FieldKeyID].(string); ok {
					return id, nil
				}
			}
		}
	}

	return "", errfmt.Errorf("scenario not found: %s", scenarioName)
}
