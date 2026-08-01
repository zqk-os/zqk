package cli

import (
	"bytes"
	"io"
	"os"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/termfd"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

const (
	dataLoaderFlagFile               = "file"
	dataLoaderFlagData               = "data"
	dataLoaderLogFieldFile           = "file"
	dataLoaderErrReadFilePrefix      = "failed to read file"
	dataLoaderErrParseYAMLFilePrefix = "failed to parse YAML"
	dataLoaderLogReadFileFailed      = "Failed to read file"
	dataLoaderLogParseYAMLFileFailed = "Failed to parse YAML from file"
)

// DataLoader provides unified data loading from file, inline data, or stdin
type DataLoader struct {
	logger *logging.EventLogger
}

// NewDataLoader creates a new data loader
func NewDataLoader(logger *logging.EventLogger) *DataLoader {
	return &DataLoader{logger: logger}
}

// LoadData loads data from file, inline data, last-draft pointer, or stdin (in that priority order).
// hint may be nil; when set with Scope and Kind, a matching last-draft pointer (from `zqk new`) is used
// when --file and --data are empty and stdin is empty (TTY) or an empty pipe.
func (dl *DataLoader) LoadData(cmd *cobra.Command, hint *LastDraftHint) (data map[string]any, filePath string, err error) {
	filePath, _ = cmd.Flags().GetString(dataLoaderFlagFile)
	dataStr, _ := cmd.Flags().GetString(dataLoaderFlagData)
	fields, _ := cmd.Flags().GetStringArray("field")

	var fieldUpdates map[string]any
	if len(fields) > 0 {
		fieldUpdates = make(map[string]any)
		for _, fieldStr := range fields {
			parts := strings.SplitN(fieldStr, "=", 2)
			if len(parts) == 2 {
				// Trim spaces to support "key=value"
				key := strings.TrimSpace(parts[0])
				val := strings.TrimSpace(parts[1])
				fieldUpdates[key] = val
			} else {
				// Try colon
				parts = strings.SplitN(fieldStr, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					val := strings.TrimSpace(parts[1])
					fieldUpdates[key] = val
				}
			}
		}
	}

	var loadedData map[string]any
	if filePath != emptyValue {
		loadedData, filePath, err = dl.LoadFromFile(filePath)
	} else if dataStr != emptyValue {
		loadedData, filePath, err = dl.LoadFromString(dataStr)
	} else if len(fields) == 0 {
		// Non-terminal stdin: read entire pipe/redir before considering last-draft pointer.
		if !term.IsTerminal(termfd.Int(os.Stdin)) {
			stdinData, stdinErr := io.ReadAll(os.Stdin)
			if stdinErr != nil {
				return nil, "", errfmt.Newf("read stdin").Wrap(stdinErr)
			}
			if len(bytes.TrimSpace(stdinData)) > 0 {
				loadedData = make(map[string]any)
				if parseErr := yaml.Unmarshal(stdinData, &loadedData); parseErr != nil {
					dl.logger.LogError("Failed to parse YAML from stdin", parseErr)
					return nil, "", errfmt.Newf("failed to parse YAML from stdin").Wrap(parseErr)
				}
			} else {
				// Empty pipe: try last-draft pointer
				p, draftErr := dl.tryLastDraft(hint)
				if draftErr != nil {
					return nil, "", draftErr
				}
				if p != emptyValue {
					loadedData, filePath, err = dl.LoadFromFile(p)
				}
			}
		} else {
			// Terminal: prefer last-draft pointer so `zqk object create <kind>` does not block on stdin.
			p, draftErr := dl.tryLastDraft(hint)
			if draftErr != nil {
				return nil, "", draftErr
			}
			if p != emptyValue {
				loadedData, filePath, err = dl.LoadFromFile(p)
			} else {
				loadedData, filePath, err = dl.LoadFromStdin()
			}
		}
	} else {
		loadedData = make(map[string]any)
	}

	if err != nil {
		return nil, "", err
	}

	if loadedData == nil {
		loadedData = make(map[string]any)
	}

	// Apply field updates
	for k, v := range fieldUpdates {
		loadedData[k] = v
	}

	if len(loadedData) == 0 {
		return nil, "", errfmt.Errorf("no data provided (use --file, --data, --field, or pipe from stdin)")
	}

	return loadedData, filePath, nil
}

func (dl *DataLoader) tryLastDraft(hint *LastDraftHint) (path string, err error) {
	if hint == nil || hint.Scope == emptyValue || hint.Kind == emptyValue {
		return "", nil
	}
	root := ResolveProjectRoot(".")
	if root == emptyValue {
		return "", nil
	}
	p, err := ResolveLastDraftFile(root, hint.Scope, hint.Kind)
	if err != nil {
		return "", err
	}
	if p == emptyValue {
		return "", nil
	}
	logging.FluentEvent(dl.logger).Debug("Using last draft from pointer").
		String("file", p).
		Log()
	return p, nil
}

// LoadFromFile loads data from a file
func (dl *DataLoader) LoadFromFile(filePath string) (data map[string]any, filePathOut string, err error) {
	logging.FluentEvent(dl.logger).Debug("Reading data from file").
		String(dataLoaderLogFieldFile, filePath).
		Log()
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		logging.FluentEvent(dl.logger).Error(dataLoaderLogReadFileFailed, err).
			String(dataLoaderLogFieldFile, filePath).
			Log()
		return nil, filePath, errfmt.Errorf("%s: %w", dataLoaderErrReadFilePrefix, err)
	}
	data = make(map[string]any)
	if err := yaml.Unmarshal(fileData, &data); err != nil {
		logging.FluentEvent(dl.logger).Error(dataLoaderLogParseYAMLFileFailed, err).
			String(dataLoaderLogFieldFile, filePath).
			Log()
		return nil, filePath, errfmt.Errorf("%s: %w", dataLoaderErrParseYAMLFilePrefix, err)
	}
	return data, filePath, nil
}

// LoadFromString loads data from an inline string
func (dl *DataLoader) LoadFromString(dataStr string) (data map[string]any, filePath string, err error) {
	dl.logger.LogDebug("Parsing inline data")
	data = make(map[string]any)
	if err := yaml.Unmarshal([]byte(dataStr), &data); err != nil {
		dl.logger.LogError("Failed to parse inline YAML", err)
		return nil, "", errfmt.Newf("failed to parse YAML").Wrap(err)
	}
	return data, "", nil
}

// LoadFromStdin loads data from stdin
func (dl *DataLoader) LoadFromStdin() (data map[string]any, filePath string, err error) {
	dl.logger.LogDebug("Reading data from stdin")

	// Prevent hang if stdin is a terminal and no data was piped
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) != 0 {
		dl.logger.LogWarning("No data provided (stdin is terminal)")
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
		dl.logger.LogWarning("No data provided")
		return nil, "", errfmt.Errorf("no data provided (use --file, --data, or pipe from stdin)")
	}
	data = make(map[string]any)
	if err := yaml.Unmarshal([]byte(stdinData.String()), &data); err != nil {
		dl.logger.LogError("Failed to parse YAML from stdin", err)
		return nil, "", errfmt.Newf("failed to parse YAML from stdin").Wrap(err)
	}
	return data, "", nil
}
