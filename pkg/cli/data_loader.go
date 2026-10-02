package cli

import (
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Logger interface for data loading operations
type DataLoaderLogger interface {
	LogDebug(msg string, fields ...logging.Field)
	LogError(msg string, err error, fields ...logging.Field)
	LogWarning(msg string, fields ...logging.Field)
}

func LoadObjectData(cmd *cobra.Command, logger DataLoaderLogger) (objData map[string]any, filePath string, err error) {
	var flagsBag FlagBag
	filePath, dataStr, fields := flagsBag.ReadDataInputFlags(cmd)
	if err := flagsBag.Err(); err != nil {
		return nil, "", err
	}

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
	data, err := fileutil.ReadFile(filePath)
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
	if (stat.Mode() & fileutil.ModeCharDevice) != 0 {
		logger.LogWarning("No data provided for object creation (stdin is terminal)")
		return nil, "", errfmt.Errorf("no data provided (use --file, --data, or pipe from stdin)")
	}

	var stdinData strings.Builder
	buf := make([]byte, 1024)

	// Structurally address CLI hangs on empty pipes (deadlocks from agent subprocess.PIPE).
	ch := make(chan struct {
		n   int
		err error
	}, 1)

	goroutinelabels.NewGoroutine("stdin_reader", "Read stdin data asynchronously").StartSimple(func() {
		n, err := os.Stdin.Read(buf)
		ch <- struct {
			n   int
			err error
		}{n, err}
	})

	select {
	case res := <-ch:
		if res.n > 0 {
			stdinData.Write(buf[:res.n])
		}
		if res.err != nil {
			// e.g. io.EOF
		} else {
			// Read the rest without timeout since we got the first chunk
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					stdinData.Write(buf[:n])
				}
				if err != nil {
					break
				}
			}
		}
	case <-time.After(2 * time.Second):
		return nil, "", errfmt.Errorf("stdin read timeout: possible deadlock from empty pipe without args (use --file, --data, or close pipe)")
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
