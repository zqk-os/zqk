package cli

import (
	"os"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// Logger interface for data loading operations
type DataLoaderLogger interface {
	LogDebug(msg string, fields ...logging.Field)
	LogError(msg string, err error, fields ...logging.Field)
	LogWarning(msg string, fields ...logging.Field)
}

// LoadObjectData loads object data from file, inline data, stdin, or field flags
// Returns the object data and the file path (if loaded from file)
func LoadObjectData(cmd *cobra.Command, logger DataLoaderLogger) (objData map[string]any, filePath string, err error) {
	filePath, _ = cmd.Flags().GetString("file")
	dataStr, _ := cmd.Flags().GetString("data")
	fields, _ := cmd.Flags().GetStringArray("field")

	var fieldUpdates map[string]any
	if len(fields) > 0 {
		fieldUpdates = make(map[string]any)
		for _, fieldStr := range fields {
			parts := strings.SplitN(fieldStr, "=", 2)
			if len(parts) == 2 {
				fieldUpdates[parts[0]] = ParseFieldValue(parts[1])
			}
		}
	}

	if filePath != emptyValue {
		objData, filePath, err = LoadObjectDataFromFile(filePath, logger)
	} else if dataStr != emptyValue {
		objData, filePath, err = LoadObjectDataFromString(dataStr, logger)
	} else if len(fields) == 0 {
		objData, filePath, err = LoadObjectDataFromStdin(logger)
	} else {
		objData = make(map[string]any)
	}

	if err == nil && len(fieldUpdates) > 0 {
		if objData == nil {
			objData = make(map[string]any)
		}
		for k, v := range fieldUpdates {
			objData[k] = v
		}
	}

	if len(objData) == 0 && err == nil {
		return nil, "", errfmt.Errorf("no data provided (use --file, --data, --field, or pipe from stdin)")
	}

	return objData, filePath, err
}

// LoadObjectDataFromFile loads object data from a file
func LoadObjectDataFromFile(filePath string, logger DataLoaderLogger) (objData map[string]any, filePathOut string, err error) {
	if r, ok := logging.TryFluentEvent(logger); ok {
		r.Debug("Reading object data from file").File(filePath).Log()
	} else {
		logger.LogDebug("Reading object data from file", logging.String("file", filePath))
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		if r, ok := logging.TryFluentEvent(logger); ok {
			r.Error("Failed to read file", err).File(filePath).Log()
		} else {
			logger.LogError("Failed to read file", err, logging.String("file", filePath))
		}
		return nil, filePath, errfmt.Newf("failed to read file").Wrap(err)
	}
	objData = make(map[string]any)
	if err := yaml.Unmarshal(data, &objData); err != nil {
		if r, ok := logging.TryFluentEvent(logger); ok {
			r.Error("Failed to parse YAML from file", err).File(filePath).Log()
		} else {
			logger.LogError("Failed to parse YAML from file", err, logging.String("file", filePath))
		}
		return nil, filePath, errfmt.Newf("failed to parse YAML").Wrap(err)
	}
	return objData, filePath, nil
}

// LoadObjectDataFromString loads object data from inline string
func LoadObjectDataFromString(dataStr string, logger DataLoaderLogger) (objData map[string]any, filePath string, err error) {
	logger.LogDebug("Parsing inline object data")
	objData = make(map[string]any)
	if err := yaml.Unmarshal([]byte(dataStr), &objData); err != nil {
		logger.LogError("Failed to parse inline YAML", err)
		return nil, "", errfmt.Newf("failed to parse YAML").Wrap(err)
	}
	return objData, "", nil
}

// LoadObjectDataFromStdin loads object data from stdin
func LoadObjectDataFromStdin(logger DataLoaderLogger) (objData map[string]any, filePath string, err error) {
	logger.LogDebug("Reading object data from stdin")

	// Prevent hang if stdin is a terminal and no data was piped
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		logger.LogWarning("No data provided for object creation (stdin is terminal)")
		return nil, "", errfmt.Errorf("no data provided (use --file, --data, or pipe from stdin)")
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
		logger.LogWarning("No data provided for object creation")
		return nil, "", errfmt.Errorf("no data provided (use --file, --data, or pipe from stdin)")
	}
	objData = make(map[string]any)
	if err := yaml.Unmarshal([]byte(stdinData.String()), &objData); err != nil {
		logger.LogError("Failed to parse YAML from stdin", err)
		return nil, "", errfmt.Newf("failed to parse YAML from stdin").Wrap(err)
	}
	return objData, "", nil
}
