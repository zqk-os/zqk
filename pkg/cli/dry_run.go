package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/zqk-os/zqk/pkg/logging"
)

// Logger interface for dry-run operations
type DryRunLogger interface {
	LogInfo(msg string, fields ...logging.Field)
	LogError(msg string, err error, fields ...logging.Field)
}

// DryRunResult represents the structured output for dry-run operations
// This allows the output to flow through format handlers for consistent formatting
type DryRunResult struct {
	Message string         `json:"message" yaml:"message"`
	Data    map[string]any `json:"data" yaml:"data"`
	Kind    string         `json:"kind" yaml:"kind"`
}

func isDryRunActive(cmd *cobra.Command, objectType string) (bool, string) {
	var flags FlagBag
	dryRun := flags.Bool(cmd, "dry-run")
	if flags.Err() != nil || !dryRun {
		return false, ""
	}
	if objectType != emptyValue {
		return true, objectType
	}
	return true, "object"
}

func logDryRunAction(logger DryRunLogger, action string, id string, kind string) {
	if r, ok := logging.TryFluentEvent(logger); ok {
		event := r.Info("Dry-run mode: showing what would be " + action)
		if id != emptyValue {
			event = event.ObjectID(id)
		}
		if kind != emptyValue {
			event = event.Kind(kind)
		}
		event.Log()
	} else if id != emptyValue {
		logger.LogInfo("Dry-run mode: showing what would be "+action, logging.IDField(id))
	} else if kind != emptyValue {
		logger.LogInfo("Dry-run mode: showing what would be "+action, logging.KindField(kind))
	} else {
		logger.LogInfo("Dry-run mode: showing what would be " + action)
	}
}

func checkAndLogDryRun(cmd *cobra.Command, objectType, action, id, kind string, logger DryRunLogger) (bool, string) {
	active, label := isDryRunActive(cmd, objectType)
	if !active {
		return false, ""
	}
	logDryRunAction(logger, action, id, kind)
	return true, label
}

// HandleDryRun handles dry-run mode for create operations
// Returns (handled, result, error) - handled=true means dry-run was executed and command should exit
// Caller should use internal/cli.FormatOutput to format the result, which respects --format flag
func HandleDryRun(cmd *cobra.Command, objData map[string]any, kind string, logger DryRunLogger, objectType string) (bool, *DryRunResult, error) {
	active, label := checkAndLogDryRun(cmd, objectType, "created", emptyValue, kind, logger)
	if !active {
		return false, nil, nil
	}

	result := &DryRunResult{
		Message: fmt.Sprintf("Would create %s", label),
		Data:    objData,
		Kind:    kind,
	}

	return true, result, nil
}

// DeleteDryRunResult represents the structured output for delete dry-run operations
type DeleteDryRunResult struct {
	Message string         `json:"message" yaml:"message"`
	ID      string         `json:"id" yaml:"id"`
	Object  map[string]any `json:"object" yaml:"object"`
	Cascade bool           `json:"cascade" yaml:"cascade"`
}

// HandleDeleteDryRun handles dry-run mode for delete operations
// Returns (handled, result, error) - handled=true means dry-run was executed and command should exit
// Caller should use internal/cli.FormatOutput to format the result, which respects --format flag
func HandleDeleteDryRun(cmd *cobra.Command, id string, obj map[string]any, cascade bool, logger DryRunLogger, objectType string) (bool, *DeleteDryRunResult, error) {
	active, label := checkAndLogDryRun(cmd, objectType, "deleted", id, emptyValue, logger)
	if !active {
		return false, nil, nil
	}

	result := &DeleteDryRunResult{
		Message: fmt.Sprintf("Would delete %s", label),
		ID:      id,
		Object:  obj,
		Cascade: cascade,
	}

	return true, result, nil
}

// UpdateDryRunResult represents the structured output for update dry-run operations
type UpdateDryRunResult struct {
	Message   string            `json:"message" yaml:"message"`
	ID        string            `json:"id" yaml:"id"`
	Current   map[string]any    `json:"current" yaml:"current"`
	Updates   map[string]any    `json:"updates" yaml:"updates"`
	Changed   map[string]Change `json:"changed" yaml:"changed"`
	IsBuiltIn bool              `json:"is_built_in,omitempty" yaml:"is_built_in,omitempty"`
}

// Change represents a field change (old -> new value)
type Change struct {
	Old any `json:"old" yaml:"old"`
	New any `json:"new" yaml:"new"`
}

// HandleUpdateDryRun handles dry-run mode for update operations
// Returns (handled, result, error) - handled=true means dry-run was executed and command should exit
// Caller should use internal/cli.FormatOutput to format the result, which respects --format flag
func HandleUpdateDryRun(cmd *cobra.Command, id string, current map[string]any, updates map[string]any, isBuiltIn bool, logger DryRunLogger, objectType string) (bool, *UpdateDryRunResult, error) {
	active, label := checkAndLogDryRun(cmd, objectType, "updated", id, emptyValue, logger)
	if !active {
		return false, nil, nil
	}

	// Build changed fields map
	changed := make(map[string]Change)
	for k, newVal := range updates {
		if k == "expected_updated_at" {
			continue // Skip optimistic locking field
		}
		if oldVal, exists := current[k]; exists {
			changed[k] = Change{Old: oldVal, New: newVal}
		} else {
			changed[k] = Change{Old: nil, New: newVal}
		}
	}

	result := &UpdateDryRunResult{
		Message:   fmt.Sprintf("Would update %s", label),
		ID:        id,
		Current:   current,
		Updates:   updates,
		Changed:   changed,
		IsBuiltIn: isBuiltIn,
	}

	return true, result, nil
}
