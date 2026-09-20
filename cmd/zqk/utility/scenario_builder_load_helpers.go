package utility

import (
	"context"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
)

// readScenarioObject reads and validates a scenario object
func readScenarioObject(ctx context.Context, sb *ScenarioBuilder, scenarioID string) (map[string]any, error) {
	scenarioObj, err := sb.storage.Read(ctx, sb.secCtx, scenarioID)
	if err != nil {
		return nil, errfmt.Newf("failed to read scenario object").Wrap(err)
	}
	return scenarioObj, nil
}

// extractDataGenerationConfig extracts and validates data generation config
func extractDataGenerationConfig(scenarioObj map[string]any, scenarioID string) (map[string]any, error) {
	configData, ok := scenarioObj[objects.FieldKeyDataGenerationConfig]
	if !ok {
		return nil, errfmt.Errorf("scenario object %s does not have data_generation_config", scenarioID)
	}

	configMap, ok := configData.(map[string]any)
	if !ok {
		return nil, errfmt.Errorf("data_generation_config must be an object")
	}

	return configMap, nil
}

// convertToInt converts a value to int (handles both int and float64)
func convertToInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

// loadKindsFromConfig loads kinds from config map
func loadKindsFromConfig(configMap map[string]any) []string {
	if kinds, ok := configMap["kinds"].([]any); ok {
		result := make([]string, len(kinds))
		for i, k := range kinds {
			if str, ok := k.(string); ok {
				result[i] = str
			}
		}
		return result
	}
	return nil
}

// loadCountsFromConfig loads counts from config map
func loadCountsFromConfig(configMap map[string]any) map[string]int {
	if counts, ok := configMap["counts"].(map[string]any); ok {
		result := make(map[string]int)
		for kind, count := range counts {
			if intVal, ok := convertToInt(count); ok {
				result[kind] = intVal
			}
		}
		return result
	}
	return nil
}

// loadDiversityFromConfig loads diversity from config map
func loadDiversityFromConfig(configMap map[string]any) map[string]int {
	if diversity, ok := configMap["diversity"].(map[string]any); ok {
		result := make(map[string]int)
		for kind, div := range diversity {
			if intVal, ok := convertToInt(div); ok {
				result[kind] = intVal
			}
		}
		return result
	}
	return nil
}

// loadDefaultCountFromConfig loads default count from config map
func loadDefaultCountFromConfig(configMap map[string]any) int {
	if intVal, ok := convertToInt(configMap["default_count"]); ok {
		return intVal
	}
	return 0
}

// loadDefaultDiversityFromConfig loads default diversity from config map
func loadDefaultDiversityFromConfig(configMap map[string]any) int {
	if intVal, ok := convertToInt(configMap["default_diversity"]); ok {
		return intVal
	}
	return 0
}

// loadLinkProbabilityFromConfig loads link probability from config map
func loadLinkProbabilityFromConfig(configMap map[string]any) float64 {
	if lp, ok := configMap["link_probability"].(float64); ok {
		return lp
	}
	return 0
}

// loadTimeRangeFromConfig loads time range from config map
func loadTimeRangeFromConfig(configMap map[string]any) time.Duration {
	if tr, ok := configMap["time_range"].(string); ok {
		if duration, err := time.ParseDuration(tr); err == nil {
			return duration
		}
	}
	return 0
}

// loadStartTimeFromConfig loads start time from config map
func loadStartTimeFromConfig(configMap map[string]any) time.Time {
	if st, ok := configMap["start_time"].(string); ok {
		if startTime, err := time.Parse(time.RFC3339, st); err == nil {
			return startTime
		}
	}
	return time.Time{}
}

// applyConfigToBuilder applies loaded config to builder
func applyConfigToBuilder(sb *ScenarioBuilder, configMap map[string]any) {
	// Load kinds
	if kinds := loadKindsFromConfig(configMap); kinds != nil {
		sb.config.Kinds = kinds
	}

	// Load counts
	if counts := loadCountsFromConfig(configMap); counts != nil {
		sb.config.Counts = counts
	}

	// Load default count
	sb.config.DefaultCount = loadDefaultCountFromConfig(configMap)

	// Load diversity
	if diversity := loadDiversityFromConfig(configMap); diversity != nil {
		sb.config.Diversity = diversity
	}

	// Load default diversity
	sb.config.DefaultDiversity = loadDefaultDiversityFromConfig(configMap)

	// Load link probability
	sb.config.LinkProbability = loadLinkProbabilityFromConfig(configMap)

	// Load time range
	sb.config.TimeRange = loadTimeRangeFromConfig(configMap)

	// Load start time
	sb.config.StartTime = loadStartTimeFromConfig(configMap)
}
