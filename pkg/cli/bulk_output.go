package cli

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/lanceman/zqk/pkg/outputtypes"
	"gopkg.in/yaml.v3"
)

// BulkErrorInfo represents error information for a single item in a bulk operation
// This avoids importing pkg/storage directly
type BulkErrorInfo struct {
	ID      string
	Index   int
	Message string
}

const (
	bulkOutputKeyOperation    = "operation"
	bulkOutputKeyTotal        = "total"
	bulkOutputKeySuccess      = "success"
	bulkOutputKeyFailure      = "failure"
	bulkOutputKeyResultsCount = "results_count"
	bulkOutputKeyErrorsCount  = "errors_count"
	bulkOutputKeyResults      = "results"
	bulkOutputKeyErrors       = "errors"
	bulkOutputKeyID           = "id"
	bulkOutputKeyIndex        = "index"
	bulkOutputKeyMessage      = "message"
)

// BuildBulkResultData builds the structured data for bulk result output
// Returns data that can be passed to FormatOutput
// This function takes individual fields to avoid importing pkg/storage
func BuildBulkResultData(totalCount, successCount, failureCount int, results []map[string]any, errors []BulkErrorInfo, operation string) map[string]any {
	outputData := map[string]any{
		bulkOutputKeyOperation:    operation,
		bulkOutputKeyTotal:        totalCount,
		bulkOutputKeySuccess:      successCount,
		bulkOutputKeyFailure:      failureCount,
		bulkOutputKeyResultsCount: len(results),
		bulkOutputKeyErrorsCount:  len(errors),
	}

	// Include results if available
	if len(results) > 0 {
		outputData[bulkOutputKeyResults] = results
	}

	// Include errors if available
	if len(errors) > 0 {
		errorMaps := make([]map[string]any, len(errors))
		for i, err := range errors {
			errorMaps[i] = map[string]any{
				bulkOutputKeyID:      err.ID,
				bulkOutputKeyIndex:   err.Index,
				bulkOutputKeyMessage: err.Message,
			}
		}
		outputData[bulkOutputKeyErrors] = errorMaps
	}

	return outputData
}

// OutputBulkResult formats bulk operation results in the requested format and returns the formatted bytes
// Callers should use internal/cli.FormatOutput or internal/cli.WriteOutput to write the result
// This is a shared utility for bulk operations across object, internal, and other commands
func OutputBulkResult(totalCount, successCount, failureCount int, results []map[string]any, errors []BulkErrorInfo, operation, format string) ([]byte, error) {
	outputData := BuildBulkResultData(totalCount, successCount, failureCount, results, errors, operation)

	switch format {
	case string(outputtypes.IDJSON):
		return OutputBulkResultJSON(outputData, operation, totalCount, successCount, failureCount)
	case string(outputtypes.IDYAML):
		return OutputBulkResultYAML(outputData, operation, totalCount, successCount, failureCount)
	case string(outputtypes.IDTable):
		return OutputBulkResultTable(totalCount, successCount, failureCount, results, errors, operation)
	default:
		// Default to table for unknown formats
		return OutputBulkResultTable(totalCount, successCount, failureCount, results, errors, operation)
	}
}

// OutputBulkResultJSON formats bulk results in JSON format
func OutputBulkResultJSON(outputData map[string]any, operation string, totalCount, successCount, failureCount int) ([]byte, error) {
	jsonData, err := json.MarshalIndent(outputData, "", "  ")
	if err != nil {
		// Fallback to simple output if marshaling fails
		fallback := fmt.Sprintf(`{"operation":%q,"total":%d,"success":%d,"failure":%d,"error":"failed to marshal result: %v"}`, operation, totalCount, successCount, failureCount, err)
		return []byte(fallback), nil
	}
	return jsonData, nil
}

// OutputBulkResultYAML formats bulk results in YAML format
func OutputBulkResultYAML(outputData map[string]any, operation string, totalCount, successCount, failureCount int) ([]byte, error) {
	yamlData, err := yaml.Marshal(outputData)
	if err != nil {
		// Fallback to simple output if marshaling fails
		fallback := fmt.Sprintf("operation: %s\ntotal: %d\nsuccess: %d\nfailure: %d\nerror: failed to marshal result: %v\n", operation, totalCount, successCount, failureCount, err)
		return []byte(fallback), nil
	}
	return yamlData, nil
}

// OutputBulkResultTable formats bulk results in table format
func OutputBulkResultTable(totalCount, successCount, failureCount int, results []map[string]any, errors []BulkErrorInfo, operation string) ([]byte, error) {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "Bulk %s operation completed:\n", operation)
	fmt.Fprintf(&buf, "  Total:    %d\n", totalCount)
	fmt.Fprintf(&buf, "  Success:  %d\n", successCount)
	fmt.Fprintf(&buf, "  Failure:  %d\n", failureCount)

	if len(errors) > 0 {
		buf.WriteString("\nErrors:\n")
		for _, err := range errors {
			if err.ID != emptyValue {
				fmt.Fprintf(&buf, "  - %s (index %d): %s\n", err.ID, err.Index, err.Message)
			} else {
				fmt.Fprintf(&buf, "  - Index %d: %s\n", err.Index, err.Message)
			}
		}
	}

	if len(results) > 0 && (operation == "get" || operation == "create") {
		fmt.Fprintf(&buf, "\nRetrieved %d object(s)\n", len(results))
	}

	return buf.Bytes(), nil
}
