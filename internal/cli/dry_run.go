package cli

import (
	"bytes"
	"fmt"

	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// DryRunHandler provides unified dry-run handling
type DryRunHandler struct {
	logger *logging.EventLogger
}

// NewDryRunHandler creates a new dry-run handler
func NewDryRunHandler(logger *logging.EventLogger) *DryRunHandler {
	return &DryRunHandler{logger: logger}
}

// IsDryRun checks if dry-run mode is enabled
func (dr *DryRunHandler) IsDryRun(cmd *cobra.Command) bool {
	var flagsBag clipkg.FlagBag
	dryRun := flagsBag.Bool(cmd, "dry-run")
	if flagsBag.Err() != nil {
		return false
	}
	return dryRun
}

// HandleCreateDryRun handles dry-run for create operations
func (dr *DryRunHandler) HandleCreateDryRun(cmd *cobra.Command, objData map[string]any, kind string) (bool, error) {
	if !dr.IsDryRun(cmd) {
		return false, nil
	}

	logging.FluentEvent(dr.logger).Info("Dry-run mode: showing what would be created").
		String("kind", kind).
		Log()
	output, err := yaml.Marshal(objData)
	if err != nil {
		logging.FluentEvent(dr.logger).Error("Failed to marshal object for dry-run", err).Log()
		return true, errfmt.Newf("failed to marshal object").Wrap(err)
	}
	var buf bytes.Buffer
	buf.WriteString("Would create object:\n")
	buf.Write(output)
	return true, WriteOutput(cmd, buf.Bytes())
}

// HandleUpdateDryRun handles dry-run for update operations
func (dr *DryRunHandler) HandleUpdateDryRun(cmd *cobra.Command, id string, current map[string]any, updates map[string]any) (bool, error) {
	if !dr.IsDryRun(cmd) {
		return false, nil
	}

	logging.FluentEvent(dr.logger).Info("Dry-run mode: showing what would be updated").
		String("id", id).
		Log()
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Would update object %s:\n", id)
	buf.WriteString("Current values:\n")
	for k, v := range current {
		if updates[k] != nil {
			fmt.Fprintf(&buf, "  %s: %v -> %v\n", k, v, updates[k])
		}
	}
	buf.WriteString("New values:\n")
	for k, v := range updates {
		if k != "expected_updated_at" {
			fmt.Fprintf(&buf, "  %s: %v\n", k, v)
		}
	}
	return true, WriteOutput(cmd, buf.Bytes())
}

// CreateDryRunResult is the formattable result for create dry-run (use with FormatOutput).
type CreateDryRunResult struct {
	Message string         `json:"message" yaml:"message"`
	Data    map[string]any `json:"data" yaml:"data"`
	Kind    string         `json:"kind" yaml:"kind"`
}

// HandleCreateDryRunResult returns a formattable result for create dry-run. Caller should use FormatOutput(cmd, result).
func (dr *DryRunHandler) HandleCreateDryRunResult(cmd *cobra.Command, objData map[string]any, kind string, objectType string) (bool, *CreateDryRunResult, error) {
	if !dr.IsDryRun(cmd) {
		return false, nil, nil
	}
	label := "object"
	if objectType != emptyValue {
		label = objectType
	}
	logging.FluentEvent(dr.logger).Info("Dry-run mode: showing what would be created").
		String("kind", kind).
		Log()
	return true, &CreateDryRunResult{
		Message: fmt.Sprintf("Would create %s", label),
		Data:    objData,
		Kind:    kind,
	}, nil
}

// UpdateDryRunResult is the formattable result for update dry-run (use with FormatOutput).
type UpdateDryRunResult struct {
	Message string            `json:"message" yaml:"message"`
	ID      string            `json:"id" yaml:"id"`
	Current map[string]any    `json:"current" yaml:"current"`
	Updates map[string]any    `json:"updates" yaml:"updates"`
	Changed map[string]Change `json:"changed" yaml:"changed"`
}

// Change represents a field change (old -> new value).
type Change struct {
	Old any `json:"old" yaml:"old"`
	New any `json:"new" yaml:"new"`
}

// HandleUpdateDryRunResult returns a formattable result for update dry-run. Caller should use FormatOutput(cmd, result).
func (dr *DryRunHandler) HandleUpdateDryRunResult(cmd *cobra.Command, id string, current map[string]any, updates map[string]any) (bool, *UpdateDryRunResult, error) {
	if !dr.IsDryRun(cmd) {
		return false, nil, nil
	}
	logging.FluentEvent(dr.logger).Info("Dry-run mode: showing what would be updated").
		String("id", id).
		Log()
	changed := make(map[string]Change)
	for k, newVal := range updates {
		if k == "expected_updated_at" {
			continue
		}
		if oldVal, has := current[k]; has {
			changed[k] = Change{Old: oldVal, New: newVal}
		}
	}
	return true, &UpdateDryRunResult{
		Message: "Would update object",
		ID:      id,
		Current: current,
		Updates: updates,
		Changed: changed,
	}, nil
}
