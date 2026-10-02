package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Logger interface for CRUD helper operations
type CRUDLogger interface {
	LogInfo(msg string, fields ...logging.Field)
	LogWarning(msg string, fields ...logging.Field)
	LogError(msg string, err error, fields ...logging.Field)
}

// EnsureKindMatches ensures the object's kind matches the command's kind
func EnsureKindMatches(objData map[string]any, kind string, logger CRUDLogger) error {
	if objData[objects.FieldKeyKind] == nil {
		objData[objects.FieldKeyKind] = kind
	} else if objData[objects.FieldKeyKind] != kind {
		if r, ok := logging.TryFluentEvent(logger); ok {
			r.Warn("Kind mismatch detected").
				String("object_kind", fmt.Sprintf("%v", objData[objects.FieldKeyKind])).
				String("command_kind", kind).
				Log()
		} else {
			logger.LogWarning("Kind mismatch detected",
				logging.String("object_kind", fmt.Sprintf("%v", objData[objects.FieldKeyKind])),
				logging.String("command_kind", kind))
		}
		return errfmt.Errorf("kind mismatch: object has kind %v, but command specified %s", objData[objects.FieldKeyKind], kind)
	}
	return nil
}

// isTempFilePath returns true if the path is considered a temp file for create-from-file cleanup.
// Temp paths: base name starts with "tmp-" or "tmp_" (e.g. tmp-req.yaml, tmp_policy_data_retention.yaml).
func isTempFilePath(filePath string) bool {
	if filePath == emptyValue {
		return false
	}
	base := filepath.Base(filePath)
	return strings.HasPrefix(base, "tmp-") || strings.HasPrefix(base, "tmp_")
}

func CleanupSourceFile(cmd *cobra.Command, filePath string, logger CRUDLogger) {
	if filePath == emptyValue {
		return
	}

	var flags FlagBag
	keepFile := flags.Bool(cmd, "keep-file")
	if flags.Err() != nil || keepFile {
		return
	}

	if !isTempFilePath(filePath) {
		return
	}

	if err := fileutil.Remove(filePath); err != nil {
		if r, ok := logging.TryFluentEvent(logger); ok {
			r.Warn("Failed to remove source file").File(filePath).WithError(err).Log()
		} else {
			logger.LogWarning("Failed to remove source file", logging.Error(err), logging.FileField(filePath))
		}
		// Don't fail the command if cleanup fails
	} else {
		if r, ok := logging.TryFluentEvent(logger); ok {
			r.Info("Source file removed").File(filePath).Log()
		} else {
			logger.LogInfo("Source file removed", logging.FileField(filePath))
		}
	}
}

func resolveObjectTypeLabel(objectType, defaultLabel string) string {
	if objectType != emptyValue {
		return objectType
	}
	return defaultLabel
}

func logAndFormatSuccess(id string, logger CRUDLogger, logMsg, cliMsg string, extraFields ...logging.Field) string {
	if r, ok := logging.TryFluentEvent(logger); ok {
		event := r.Info(logMsg)
		if id != emptyValue {
			event = event.ObjectID(id)
		}
		for _, f := range extraFields {
			if f.Key == "kind" {
				if k, ok := f.Value.(string); ok {
					event = event.Kind(k)
				}
			}
		}
		event.Log()
	} else if id != emptyValue {
		fields := append([]logging.Field{logging.IDField(id)}, extraFields...)
		logger.LogInfo(logMsg, fields...)
	} else {
		logger.LogInfo(logMsg, extraFields...)
	}

	successIcon := color.New(color.FgGreen).Sprint("✓")
	return fmt.Sprintf("%s %s\n", successIcon, cliMsg)
}

// FormatCreateSuccessMessage formats the success message for object creation
func FormatCreateSuccessMessage(objData map[string]any, kind string, objectType string, logger CRUDLogger) string {
	id, _ := objData[objects.FieldKeyID].(string)
	label := resolveObjectTypeLabel(objectType, "Object")
	if id == emptyValue {
		logMsg := fmt.Sprintf("%s created successfully", label)
		cliMsg := fmt.Sprintf("%s created successfully", label)
		return logAndFormatSuccess("", logger, logMsg, cliMsg, logging.KindField(kind))
	}

	highlightID := color.New(color.FgCyan).Sprint(id)
	logMsg := fmt.Sprintf("%s created successfully", label)
	cliMsg := fmt.Sprintf("%s %s created successfully", label, highlightID)
	return logAndFormatSuccess(id, logger, logMsg, cliMsg, logging.KindField(kind))
}

func formatActionSuccessMessage(id string, isBuiltIn bool, objectType string, actionVerb string, logger CRUDLogger) string {
	label := resolveObjectTypeLabel(objectType, "Object")
	highlightID := color.New(color.FgCyan).Sprint(id)
	prefix := ""
	if isBuiltIn {
		prefix = "Built-in "
	}
	logMsg := fmt.Sprintf("%s%s %s", prefix, label, actionVerb)
	cliMsg := fmt.Sprintf("%s%s %s %s", prefix, label, highlightID, actionVerb)
	return logAndFormatSuccess(id, logger, logMsg, cliMsg)
}

// FormatDeleteSuccessMessage formats the success message for object deletion
func FormatDeleteSuccessMessage(id string, isBuiltIn bool, cascade bool, objectType string, logger CRUDLogger) string {
	if cascade {
		label := resolveObjectTypeLabel(objectType, "Object")
		highlightID := color.New(color.FgCyan).Sprint(id)
		prefix := ""
		if isBuiltIn {
			prefix = "Built-in "
		}
		logMsg := fmt.Sprintf("%s%s and dependents deleted successfully", prefix, label)
		cliMsg := fmt.Sprintf("%s%s %s and its dependents deleted successfully", prefix, label, highlightID)
		return logAndFormatSuccess(id, logger, logMsg, cliMsg)
	}
	return formatActionSuccessMessage(id, isBuiltIn, objectType, "deleted successfully", logger)
}

// FormatUpdateSuccessMessage formats the success message for object update
func FormatUpdateSuccessMessage(id string, isBuiltIn bool, objectType string, logger CRUDLogger) string {
	return formatActionSuccessMessage(id, isBuiltIn, objectType, "updated successfully", logger)
}
