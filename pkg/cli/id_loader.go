package cli

import (
	"os"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
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
		// Parse comma-separated IDs
		for id := range strings.SplitSeq(idsFlag, idSeparator) {
			ids = append(ids, strings.TrimSpace(id))
		}
	} else if filePath != emptyValue {
		// Read from file
		data, err := os.ReadFile(filePath)
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
