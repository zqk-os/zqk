package internal

import (
	"bytes"
	"fmt"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/objects"

	"github.com/lanceman/zqk/internal/cli"
	clipkg "github.com/lanceman/zqk/pkg/cli"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/processing"
	"github.com/spf13/cobra"
)

// NewInternalProcessCmd creates a new process command for internal objects
func NewInternalProcessCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Process a reference file with operations or data (admin only)",
		"Process a reference file that contains operations or data to be automatically executed.",
		"",
		"Reference files support three formats:",
		"  1. operations: List of operations (create, update, delete, get, list)",
		"  2. data: List of objects to create",
		"  3. template: Template with variables to expand and create",
	).
		AddExample("Process operations from a reference file", "%s internal process --file operations.yaml").
		AddExample("Process data from a reference file", "%s internal process --file data.yaml").
		AddExample("Process template with variables", "%s internal process --file template.yaml").
		AddExample("Dry-run to see what would be executed", "%s internal process --file operations.yaml --dry-run").
		ExcludeCommonFlags()

	cmd := &cobra.Command{
		Use:  "process --file <reference-file>",
		Args: cobra.NoArgs,
		RunE: runInternalProcess,
	}

	// Apply help builder to command
	helpBuilder.ApplyToCommand(cmd)

	cli.AddCommonFlags(cmd)
	cmd.Flags().String("file", "", "Path to reference file (required)")
	cmd.Flags().Bool("dry-run", false, "Show what would be executed without actually executing")

	return cmd
}

func runInternalProcess(cmd *cobra.Command, args []string) error {
	// Create processor (handles context, storage, security, logging)
	proc, err := cli.NewProcessor(cmd)
	if err != nil {
		return errfmt.Newf("failed to create processor").Wrap(err)
	}

	filePath, err := cmd.Flags().GetString("file")
	if err != nil || filePath == emptyValue {
		return errfmt.Errorf("--file is required")
	}

	dryRun, err := cmd.Flags().GetBool("dry-run")
	if err != nil {
		return errfmt.Newf("failed to get dry-run flag").Wrap(err)
	}

	// Load reference file
	refFile, err := processing.LoadReferenceFile(filePath)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to load reference file", err).
			File(filePath).
			Log()
		return errfmt.Newf("failed to load reference file").Wrap(err)
	}

	logging.FluentEvent(proc.Logger()).Info("Processing reference file").
		File(filePath).
		String("format", refFile.Format).
		Bool("dry_run", dryRun).
		Log()

	// Get profile from context
	profile := proc.Context().Profile
	if profile == emptyValue {
		profile = string(pkgctx.ProfileHuman) // Default
	}

	// Create processing processor
	processor := processing.NewProcessor(proc.Storage(), profile)

	// Process the reference file
	result, err := processor.ProcessReferenceFile(proc.OperationContext(), refFile, dryRun)
	if err != nil {
		logging.FluentEvent(proc.Logger()).Error("Failed to process reference file", err).Log()
		return errfmt.Newf("failed to process reference file").Wrap(err)
	}

	format := cli.GetFormat(cmd)
	switch format {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		data := map[string]any{
			objects.FieldKeySuccessCount: result.SuccessCount,
			objects.FieldKeyFailureCount: result.FailureCount,
			"total_operations":           len(result.Operations),
			"operations":                 result.Operations,
		}
		return cli.FormatOutput(cmd, data)
	default:
		return outputTable(cmd, result)
	}
}

func outputTable(cmd *cobra.Command, result *processing.ProcessingResult) error {
	var buf bytes.Buffer
	buf.WriteString("\nProcessing Results:\n")
	fmt.Fprintf(&buf, "  Success: %d\n", result.SuccessCount)
	fmt.Fprintf(&buf, "  Failure: %d\n", result.FailureCount)
	fmt.Fprintf(&buf, "  Total:   %d\n\n", len(result.Operations))

	if len(result.Operations) > 0 {
		buf.WriteString("Operations:\n")
		for _, op := range result.Operations {
			status := op.Status
			if op.Error != emptyValue {
				status = fmt.Sprintf("%s (%s)", status, op.Error)
			}
			fmt.Fprintf(&buf, "  [%d] %s: %s", op.Index, op.Type, status)
			if op.ObjectID != emptyValue {
				fmt.Fprintf(&buf, " (ID: %s)", op.ObjectID)
			}
			if op.Description != emptyValue {
				fmt.Fprintf(&buf, " - %s", op.Description)
			}
			buf.WriteString("\n")
		}
	}
	return cli.WriteOutput(cmd, buf.Bytes())
}
