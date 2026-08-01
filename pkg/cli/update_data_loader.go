package cli

import (
	"encoding/json"
	"maps"
	"os"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	updateFieldFlagName = "field"
	updateFileFlagName  = "file"
	updateDataFlagName  = "data"
)

// UpdateDataLoaderLogger interface for logging in update data loading
type UpdateDataLoaderLogger interface {
	LogError(msg string, err error, fields ...logging.Field)
	LogDebug(msg string, fields ...logging.Field)
}

// ParseFieldFlag parses a field flag string (field=value or field:value)
// Returns field name, field value, and error
func ParseFieldFlag(fieldStr string, logger UpdateDataLoaderLogger) (fieldName string, fieldValue string, err error) {
	fieldPart, valuePart, ok := strings.Cut(fieldStr, "=")
	if !ok {
		// Try colon separator
		fieldPart, valuePart, ok = strings.Cut(fieldStr, ":")
		if !ok {
			if logger != nil {
				parseErr := errfmt.Errorf("expected field=value or field:value")
				if r, okFe := logging.TryFluentEvent(logger); okFe {
					r.Error("Invalid field format", parseErr).String(updateFieldFlagName, fieldStr).Log()
				} else {
					logger.LogError("Invalid field format", parseErr, logging.String(updateFieldFlagName, fieldStr))
				}
			}
			return "", "", errfmt.Errorf("invalid field format: %s (expected field=value or field:value)", fieldStr)
		}
	}
	fieldName = strings.TrimSpace(fieldPart)
	fieldValue = strings.TrimSpace(valuePart)
	return fieldName, fieldValue, nil
}

// ParseFieldValue parses a field value: try JSON first, otherwise treat the whole value as a plain string.
// Matches internal/cli.FieldParser.ParseFieldValue so object update and internal update behave the same.
func ParseFieldValue(fieldValue string) any {
	if fieldValue == emptyValue {
		return ""
	}
	trimmed := strings.TrimSpace(fieldValue)
	if trimmed == emptyValue {
		return ""
	}
	var parsedValue any
	if err := json.Unmarshal([]byte(trimmed), &parsedValue); err != nil {
		return fieldValue
	}
	return parsedValue
}

// BuildUpdatesFromFieldFlags builds updates map from --field flags
func BuildUpdatesFromFieldFlags(cmd *cobra.Command, logger UpdateDataLoaderLogger) (map[string]any, error) {
	updates := make(map[string]any)
	fields, err := cmd.Flags().GetStringArray(updateFieldFlagName)
	if err != nil {
		fields = []string{}
	}

	for _, fieldStr := range fields {
		fieldName, fieldValue, err := ParseFieldFlag(fieldStr, logger)
		if err != nil {
			return nil, err
		}
		updates[fieldName] = ParseFieldValue(fieldValue)
	}
	return updates, nil
}

// LoadUpdatesFromFile loads updates from a file
// Returns nil, nil if file flag is not set (not an error)
func LoadUpdatesFromFile(cmd *cobra.Command, logger UpdateDataLoaderLogger) (map[string]any, error) {
	filePath, err := cmd.Flags().GetString(updateFileFlagName)
	if err != nil {
		filePath = ""
	}

	if filePath == emptyValue {
		return nil, nil
	}

	if logger != nil {
		logger.LogDebug("Reading update data from file", logging.String("file", filePath))
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		if logger != nil {
			logger.LogError("Failed to read file", err, logging.String("file", filePath))
		}
		return nil, errfmt.Newf("failed to read file").Wrap(err)
	}

	var fileUpdates map[string]any
	if err := yaml.Unmarshal(data, &fileUpdates); err != nil {
		if logger != nil {
			if r, ok := logging.TryFluentEvent(logger); ok {
				r.Error("Failed to parse YAML from file", err).File(filePath).Log()
			} else {
				logger.LogError("Failed to parse YAML from file", err, logging.String("file", filePath))
			}
		}
		return nil, errfmt.Newf("failed to parse YAML").Wrap(err)
	}
	return fileUpdates, nil
}

// LoadUpdatesFromData loads updates from inline data (--data flag)
// Returns nil, nil if data flag is not set (not an error)
func LoadUpdatesFromData(cmd *cobra.Command, logger UpdateDataLoaderLogger) (map[string]any, error) {
	dataStr, err := cmd.Flags().GetString(updateDataFlagName)
	if err != nil {
		dataStr = ""
	}

	if dataStr == emptyValue {
		return nil, nil
	}

	if logger != nil {
		logger.LogDebug("Parsing inline update data")
	}
	var dataUpdates map[string]any
	if err := yaml.Unmarshal([]byte(dataStr), &dataUpdates); err != nil {
		if logger != nil {
			logger.LogError("Failed to parse inline YAML", err)
		}
		return nil, errfmt.Newf("failed to parse YAML").Wrap(err)
	}
	return dataUpdates, nil
}

// LoadUpdatesFromStdin loads updates from stdin
// Returns nil, nil if stdin is empty (not an error)
func LoadUpdatesFromStdin(logger UpdateDataLoaderLogger) (map[string]any, error) {
	if logger != nil {
		logger.LogDebug("Reading update data from stdin")
	}
	var stdinData strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := os.Stdin.Read(buf)
		if n > 0 {
			stdinData.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
	if stdinData.Len() == 0 {
		return nil, nil
	}
	var stdinUpdates map[string]any
	if err := yaml.Unmarshal([]byte(stdinData.String()), &stdinUpdates); err != nil {
		if logger != nil {
			logger.LogError("Failed to parse YAML from stdin", err)
		}
		return nil, errfmt.Newf("failed to parse YAML from stdin").Wrap(err)
	}
	return stdinUpdates, nil
}

// BuildUpdatesMap builds the complete updates map from all sources
// Priority: --field flags, then --file, then --data, then stdin (if no field flags)
// Returns error if no updates are provided
func BuildUpdatesMap(cmd *cobra.Command, logger UpdateDataLoaderLogger) (map[string]any, error) {
	updates := make(map[string]any)

	// Build from --field flags
	fieldUpdates, err := BuildUpdatesFromFieldFlags(cmd, logger)
	if err != nil {
		return nil, err
	}
	maps.Copy(updates, fieldUpdates)

	// Load from file (takes precedence if provided)
	fileUpdates, err := LoadUpdatesFromFile(cmd, logger)
	if err != nil {
		return nil, err
	}
	if fileUpdates != nil {
		maps.Copy(updates, fileUpdates)
		// File updates found, return early (don't check data/stdin)
		return updates, nil
	}

	// Load from --data flag
	dataUpdates, err := LoadUpdatesFromData(cmd, logger)
	if err != nil {
		return nil, err
	}
	if dataUpdates != nil {
		maps.Copy(updates, dataUpdates)
		// Data updates found, return early (don't check stdin)
		return updates, nil
	}

	// Try stdin only if no field flags were provided
	fields, _ := cmd.Flags().GetStringArray(updateFieldFlagName)
	if len(fields) == 0 {
		stdinUpdates, err := LoadUpdatesFromStdin(logger)
		if err != nil {
			return nil, err
		}
		maps.Copy(updates, stdinUpdates)
	}

	if len(updates) == 0 {
		return nil, errfmt.Errorf("no updates provided (use --file, --data, --field, or pipe from stdin)")
	}

	return updates, nil
}
