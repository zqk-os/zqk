package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const (
	idLoaderFlagIDs  = "ids"
	idLoaderFlagFile = "file"
	idSeparator      = ","
)

// IDLoaderLogger interface for logging in ID loading operations
type IDLoaderLogger interface {
	LogError(msg string, err error, fields ...logging.Field)
}

// ExpandCommaSeparatedIDs expands positional / flag tokens that may contain
// comma-separated object IDs (e.g. "ATK-1,ATK-2" or "ATK-1", "ATK-2,ATK-3").
// Empty segments after trim are dropped. Used by object update/delete/get so
// multi-ID CLI DNA matches.
func ExpandCommaSeparatedIDs(tokens ...string) []string {
	if len(tokens) == 0 {
		return nil
	}
	var ids []string
	for _, token := range tokens {
		if token == emptyValue {
			continue
		}
		if !strings.Contains(token, idSeparator) {
			trimmed := strings.TrimSpace(token)
			if trimmed != emptyValue {
				ids = append(ids, trimmed)
			}
			continue
		}
		for id := range strings.SplitSeq(token, idSeparator) {
			trimmed := strings.TrimSpace(id)
			if trimmed != emptyValue {
				ids = append(ids, trimmed)
			}
		}
	}
	return ids
}

// LoadIDsFromFlags loads IDs from --ids flag (comma-separated) or --file flag (YAML array)
// Returns the IDs and an error if neither flag is provided or if loading fails
func LoadIDsFromFlags(cmd *cobra.Command, logger IDLoaderLogger) ([]string, error) {
	var ids []string
	idsFlag, err := cmd.Flags().GetString(idLoaderFlagIDs)
	if err != nil {
		idsFlag = ""
	}
	filePath, err := cmd.Flags().GetString(idLoaderFlagFile)
	if err != nil {
		filePath = ""
	}

	if idsFlag != emptyValue {
		ids = ExpandCommaSeparatedIDs(idsFlag)
	} else if filePath != emptyValue {
		// Read from file
		data, err := fileutil.ReadFile(filePath)
		if err != nil {
			if logger != nil {
				if r, ok := logging.TryFluentEvent(logger); ok {
					r.Error("Failed to read file", err).File(filePath).Log()
				} else {
					logger.LogError("Failed to read file", err, logging.String("file", filePath))
				}
			}
			return nil, errfmt.Newf("failed to read file").Wrap(err)
		}

		if err := yaml.Unmarshal(data, &ids); err != nil {
			if logger != nil {
				if r, ok := logging.TryFluentEvent(logger); ok {
					r.Error("Failed to parse YAML", err).Log()
				} else {
					logger.LogError("Failed to parse YAML", err)
				}
			}
			return nil, errfmt.Newf("failed to parse YAML").Wrap(err)
		}
	} else {
		return nil, errfmt.Errorf("either --ids or --file must be provided")
	}

	if len(ids) == 0 {
		return nil, errfmt.Errorf("no IDs provided")
	}

	return ids, nil
}
