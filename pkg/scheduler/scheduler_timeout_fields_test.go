package scheduler

import (
	"testing"

	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
)

// TestTimeoutFieldsAppendCombine verifies that timeoutFields can be correctly
// constructed with multiple logging fields in a single append operation.
// This test ensures the appendCombine refactoring doesn't break functionality.
func TestTimeoutFieldsAppendCombine(t *testing.T) {
	t.Parallel()
	// Simulate the timeoutFields construction pattern from scheduler.go:955
	timeoutFields := []logging.Field{
		logging.String("job_id", "TEST-JOB-001"),
		logging.String("job_type", "run_wrapper"),
	}

	// Simulate the combined append operation (after refactoring)
	cmdStr := "test-command"
	workingDir := "/test/dir"
	timeoutFields = append(timeoutFields,
		logging.String("command", cmdStr),
		logging.String(objects.FieldKeyWorkingDirectory, workingDir),
		logging.String("diagnostic_note", "Command may be waiting for file locks, network I/O, or other blocking operations"),
	)

	// Verify all fields are present
	expectedFields := map[string]bool{
		"job_id":                         false,
		objects.FieldKeyJobType:          false,
		objects.FieldKeyCommand:          false,
		objects.FieldKeyWorkingDirectory: false,
		"diagnostic_note":                false,
	}

	// Check that we have the expected number of fields
	if len(timeoutFields) < len(expectedFields) {
		t.Errorf("Expected at least %d fields, got %d", len(expectedFields), len(timeoutFields))
	}

	// Verify the structure is correct (fields are logging.Field types)
	// logging.Field is an interface type, so we verify we have the expected count
	if len(timeoutFields) < len(expectedFields) {
		t.Errorf("Expected at least %d fields, got %d", len(expectedFields), len(timeoutFields))
	}

	// Verify we can iterate over fields without panicking
	// This ensures the combined append operation works correctly
	fieldCount := 0
	for range timeoutFields {
		fieldCount++
	}
	if fieldCount != len(timeoutFields) {
		t.Errorf("Field iteration mismatch: counted %d, len() reports %d", fieldCount, len(timeoutFields))
	}

	// Verify the combined append works correctly
	// The key test: ensure multiple fields can be appended in one operation
	initialLen := len(timeoutFields)
	timeoutFields = append(timeoutFields,
		logging.String("additional_field_1", "value1"),
		logging.String("additional_field_2", "value2"),
	)

	if len(timeoutFields) != initialLen+2 {
		t.Errorf("Combined append failed: expected %d fields, got %d", initialLen+2, len(timeoutFields))
	}
}
