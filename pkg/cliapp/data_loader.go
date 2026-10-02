package cli

import (
	"bytes"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"

	"github.com/zqk-os/zqk/pkg/utils/fileutil"
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
	var flagsBag clipkg.FlagBag
	filePath = flagsBag.String(cmd, dataLoaderFlagFile)
	dataStr := flagsBag.String(cmd, dataLoaderFlagData)
	fields := flagsBag.StringArray(cmd, "field")
	if err := flagsBag.Err(); err != nil {
		return nil, "", err
	}

	var fieldUpdates map[string]any
	if len(fields) > 0 {
		fp := NewFieldParser(dl.logger)
		var parseErr error
		fieldUpdates, parseErr = fp.ParseFieldFlags(fields)
		if parseErr != nil {
			return nil, "", parseErr
		}
	}

	loadedData, filePath, err := dl.resolveInitialData(filePath, dataStr, len(fields) > 0, hint)
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

func (dl *DataLoader) resolveInitialData(filePath, dataStr string, hasFields bool, hint *LastDraftHint) (map[string]any, string, error) {
	if filePath != emptyValue {
		return dl.LoadFromFile(filePath)
	}
	if dataStr != emptyValue {
		return dl.LoadFromString(dataStr)
	}
	if !hasFields {
		return dl.loadFromStdinOrDraft(hint)
	}
	return make(map[string]any), "", nil
}

func (dl *DataLoader) loadFromStdinOrDraft(hint *LastDraftHint) (map[string]any, string, error) {
	if !term.IsTerminal(fileutil.TermFdInt(os.Stdin)) {
		return dl.loadFromPipedStdin(hint)
	}
	return dl.loadFromTerminalOrDraft(hint)
}

func (dl *DataLoader) loadFromDraftIfAvailable(hint *LastDraftHint) (map[string]any, string, error, bool) {
	p, draftErr := dl.tryLastDraft(hint)
	if draftErr != nil {
		return nil, "", draftErr, true
	}
	if p != emptyValue {
		data, path, err := dl.LoadFromFile(p)
		return data, path, err, true
	}
	return nil, "", nil, false
}

func (dl *DataLoader) loadFromTerminalOrDraft(hint *LastDraftHint) (map[string]any, string, error) {
	if data, path, err, handled := dl.loadFromDraftIfAvailable(hint); handled {
		return data, path, err
	}
	return dl.LoadFromStdin()
}

func (dl *DataLoader) loadFromPipedStdin(hint *LastDraftHint) (map[string]any, string, error) {
	ch := make(chan []byte, 1)
	errCh := make(chan error, 1)
	goroutinelabels.NewGoroutine("stdin_reader", "Read stdin data asynchronously").StartSimple(func() {
		data, readErr := io.ReadAll(os.Stdin)
		if readErr != nil {
			errCh <- readErr
		} else {
			ch <- data
		}
	})

	var stdinData []byte
	var stdinErr error
	select {
	case stdinData = <-ch:
	case stdinErr = <-errCh:
	case <-time.After(2 * time.Second):
		return nil, "", errfmt.Errorf("stdin read timeout: possible deadlock from empty pipe without args (use --file, --data, or close pipe)")
	}

	if stdinErr != nil {
		return nil, "", errfmt.Newf("read stdin").Wrap(stdinErr)
	}
	if len(bytes.TrimSpace(stdinData)) > 0 {
		loadedData := make(map[string]any)
		if parseErr := yaml.Unmarshal(stdinData, &loadedData); parseErr != nil {
			dl.logger.LogError("Failed to parse YAML from stdin", parseErr)
			return nil, "", errfmt.Newf("failed to parse YAML from stdin").Wrap(parseErr)
		}
		return loadedData, "", nil
	}

	if data, path, err, handled := dl.loadFromDraftIfAvailable(hint); handled {
		return data, path, err
	}
	return make(map[string]any), "", nil
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
	fileData, err := fileutil.ReadFile(filePath)
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
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&fileutil.ModeCharDevice) != 0 {
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
