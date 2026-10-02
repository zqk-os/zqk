package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/storage"
)

// OutputBulkResult outputs bulk operation results using consistent CLI formatting.
func OutputBulkResult(cmd *cobra.Command, result *storage.BulkResult, format, operation string) {
	if result == nil {
		return
	}
	// Convert storage.BulkResult errors to clipkg.BulkErrorInfo
	errors := make([]clipkg.BulkErrorInfo, len(result.Errors))
	for i, err := range result.Errors {
		errors[i] = clipkg.BulkErrorInfo{
			ID:      err.ID,
			Index:   err.Index,
			Message: err.Message,
		}
	}

	// Build structured data for format handlers
	outputData := clipkg.BuildBulkResultData(
		result.TotalCount,
		result.SuccessCount,
		result.FailureCount,
		result.Results,
		errors,
		operation,
	)

	// Use FormatOutput for consistent formatting (respects --format flag)
	if err := FormatOutput(cmd, outputData); err != nil {
		// Fallback to legacy output if FormatOutput fails
		output, outputErr := clipkg.OutputBulkResult(
			result.TotalCount,
			result.SuccessCount,
			result.FailureCount,
			result.Results,
			errors,
			operation,
			format,
		)
		if outputErr != nil {
			// Last resort fallback
			fallback := fmt.Sprintf("Bulk %s operation completed with errors (failed to format output: %v)\n", operation, outputErr)
			if writeErr := WriteOutput(cmd, []byte(fallback)); writeErr != nil {
				return
			}
			return
		}
		if writeErr := WriteOutput(cmd, output); writeErr != nil {
			return
		}
	}
}
