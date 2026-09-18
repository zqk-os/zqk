package object

import (
	"fmt"

	"github.com/zqk-os/zqk/cmd/zqk/system"
	"github.com/zqk-os/zqk/internal/cli"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// applyHybridProjectionToObjectMaps applies hybrid top-level projection to each map (mutates slice elements in place).
func applyHybridProjectionToObjectMaps(objs []map[string]any, fields []string) {
	if len(fields) == 0 || len(objs) == 0 {
		return
	}
	for i := range objs {
		if objs[i] == nil {
			continue
		}
		kind, _ := objs[i][objects.FieldKeyKind].(string)
		mask := objects.HybridMaskForList(kind, fields, "")
		objs[i] = objects.ProjectMapHybrid(objs[i], mask)
	}
}

// applyHybridProjectionToBulkResult applies projection to successful result maps on a BulkResult.
func applyHybridProjectionToBulkResult(result *storagepkg.BulkResult, fields []string) {
	if result == nil || len(fields) == 0 {
		return
	}
	applyHybridProjectionToObjectMaps(result.Results, fields)
}

// setCacheCheckerForBatchCreation sets up the cache checker for batch creation mode
// This enables non-blocking reference validation during batch operations
func setCacheCheckerForBatchCreation(_ *cli.Processor) {
	storagepkg.SetCacheChecker(func(objectID string) (string, bool) {
		cache := system.GetGlobalObjectIDCache()
		entry, exists := cache.Get(objectID)
		if exists && entry != nil && entry.FilePath != emptyValue {
			return entry.FilePath, true
		}
		return "", false
	})
}

// outputBulkResult outputs bulk operation results using shared utility
func outputBulkResult(cmd *cobra.Command, result *storagepkg.BulkResult, format, operation string) {
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
	if err := cli.FormatOutput(cmd, outputData); err != nil {
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
			//nolint:errcheck // Output errors are non-critical
			_ = cli.WriteOutput(cmd, []byte(fallback))
			return
		}
		//nolint:errcheck // Output errors are non-critical
		_ = cli.WriteOutput(cmd, output)
	}
}
