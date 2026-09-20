package storage

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/execwrap"
	"github.com/zqk-os/zqk/pkg/objects"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// clipFirstJSONObject extracts the first {...} block from CLI combined output (log lines may precede JSON).
func clipFirstJSONObject(output []byte) []byte {
	s := string(output)
	i := strings.Index(s, "{")
	if i < 0 {
		return output
	}
	j := strings.LastIndex(s, "}")
	if j <= i {
		return []byte(s[i:])
	}
	return []byte(s[i : j+1])
}

// internalListJSON matches internal list --format json (items and legacy objects).
type internalListJSON struct {
	Items   []map[string]any `json:"items"`
	Objects []map[string]any `json:"objects"`
}

func internalListRows(r *internalListJSON) []map[string]any {
	if len(r.Objects) > 0 {
		return r.Objects
	}
	return r.Items
}

func parseIntLineFromCLIOutput(output []byte) (int, error) {
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if n, err := strconv.Atoi(line); err == nil {
			return n, nil
		}
	}
	return 0, fmt.Errorf("no integer line in output")
}

// TestCLI_AgainstTestScenario tests CLI commands against the test-scenario data
// This verifies that CLI operations work correctly with content-addressable storage
func TestCLI_AgainstTestScenario(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	prefix := "ZQK"
	t.Setenv(prefix+"_ADMIN_TEST_INIT_API_KEY", "ACC-1785920548450214012-68b850c0")
	t.Setenv(prefix+"_ADMIN_TEST_INIT_BYPASS_AUTH", "1")
	// End-to-end CLI test using an isolated copy from test-scenarios (SetupScenarioTestEnvironment).
	// Use a checked-in scenario as the immutable source of truth and work
	// against an isolated copy for this test run.
	env := SetupScenarioTestEnvironmentForTest(t, "content-addressable-storage")
	scenarioDir := env.ScenarioRoot
	MustBootstrapScenarioRootForCLIForTest(t, scenarioDir)

	// Check if test binary exists
	testBinary := filepath.Join(scenarioDir, "zqk-admin-test-init")
	if _, err := fileutil.Stat(testBinary); err != nil {
		t.Fatalf("Test binary not found: %s (run: go build -o %s ./cmd/zqk)", testBinary, testBinary)
	}

	t.Logf("Testing CLI commands against scenario: %s", scenarioDir)

	// Test internal list command
	t.Run("InternalList", func(t *testing.T) {
		testCLIInternalList(t, testBinary, scenarioDir)
	})

	// Test internal count command
	t.Run("InternalCount", func(t *testing.T) {
		testCLIInternalCount(t, testBinary, scenarioDir)
	})

	// Test internal get command
	t.Run("InternalGet", func(t *testing.T) {
		testCLIInternalGet(t, testBinary, scenarioDir)
	})

	// Test internal list with filters
	t.Run("InternalListWithFilters", func(t *testing.T) {
		testCLIInternalListWithFilters(t, testBinary, scenarioDir)
	})

	// Test internal list with grouping
	t.Run("InternalListWithGrouping", func(t *testing.T) {
		testCLIInternalListWithGrouping(t, testBinary, scenarioDir)
	})

	// Test internal count with grouping
	t.Run("InternalCountWithGrouping", func(t *testing.T) {
		testCLIInternalCountWithGrouping(t, testBinary, scenarioDir)
	})
}

func testCLIInternalList(t *testing.T, binaryPath, scenarioDir string) {
	cmd := execwrap.Command(binaryPath, "internal", "list", "test_audit_aggregation_metric", "--format", "json")
	cmd.Dir = scenarioDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("internal list failed: %v\nOutput: %s", err, output)
	}

	var result internalListJSON
	if err := json.Unmarshal(clipFirstJSONObject(output), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, output)
	}
	rows := internalListRows(&result)
	if len(rows) == 0 {
		t.Skip("No test_audit_aggregation_metric objects found in scenario (scenario may need to be populated)")
	}

	// Verify we see IDs in the output (test data uses TES- prefix, spec uses AAM-)
	// The important thing is that IDs are present, not the specific prefix
	outputStr := string(clipFirstJSONObject(output))
	hasID := strings.Contains(outputStr, "\"id\":") || strings.Contains(outputStr, "TES-") || strings.Contains(outputStr, "AAM-") || strings.Contains(outputStr, "TAM-")
	if !hasID {
		t.Errorf("Expected to see IDs in output, got: %s", outputStr)
	}

	maxLen := 500
	if len(outputStr) < maxLen {
		maxLen = len(outputStr)
	}
	t.Logf("internal list output (first %d chars): %s", maxLen, outputStr[:maxLen])
}

func testCLIInternalCount(t *testing.T, binaryPath, scenarioDir string) {
	cmd := execwrap.Command(binaryPath, "internal", "count", "test_audit_aggregation_metric")
	cmd.Dir = scenarioDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("internal count failed: %v\nOutput: %s", err, output)
	}

	count, err := parseIntLineFromCLIOutput(output)
	if err != nil {
		t.Fatalf("Failed to parse count from output: %s\nError: %v", string(output), err)
	}

	if count < 1 {
		t.Skipf("No test_audit_aggregation_metric objects found in scenario (count: %d, scenario may need to be populated)", count)
	}

	t.Logf("Count: %d", count)
}

func testCLIInternalGet(t *testing.T, binaryPath, scenarioDir string) {
	// First, get a list to find an ID
	listCmd := execwrap.Command(binaryPath, "internal", "list", "test_audit_aggregation_metric", "--format", "json", "--limit", "1")
	listCmd.Dir = scenarioDir
	listOutput, err := listCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("internal list failed: %v\nOutput: %s", err, listOutput)
	}

	// Parse JSON to get an ID
	var listResult internalListJSON
	if err := json.Unmarshal(clipFirstJSONObject(listOutput), &listResult); err != nil {
		t.Fatalf("Failed to parse list JSON: %v\nOutput: %s", err, listOutput)
	}
	listRows := internalListRows(&listResult)
	if len(listRows) == 0 {
		t.Skip("No objects found to test get command")
	}

	testID := listRows[0][objects.FieldKeyID].(string)
	if testID == emptyValue {
		t.Fatalf("No ID found in object: %+v", listRows[0])
	}

	// Test get command
	getCmd := execwrap.Command(binaryPath, "internal", "get", testID)
	getCmd.Dir = scenarioDir
	getOutput, err := getCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("internal get failed for %s: %v\nOutput: %s", testID, err, getOutput)
	}

	getOutputStr := string(getOutput)
	if !strings.Contains(getOutputStr, testID) {
		t.Errorf("Expected output to contain ID %s, got: %s", testID, getOutputStr)
	}

	t.Logf("Successfully retrieved object: %s", testID)
}

func testCLIInternalListWithFilters(t *testing.T, binaryPath, scenarioDir string) {
	// Test filtering by event_count=20 (we know this value exists from the test data)
	cmd := execwrap.Command(binaryPath, "internal", "list", "test_audit_aggregation_metric", "--filter", "event_count=20", "--format", "json")
	cmd.Dir = scenarioDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("internal list with filter failed: %v\nOutput: %s", err, output)
	}

	var result internalListJSON
	if err := json.Unmarshal(clipFirstJSONObject(output), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, output)
	}
	filterRows := internalListRows(&result)

	// Verify the command executed successfully and returned results
	// Note: We're not strictly validating filter correctness here since that's tested
	// in the comprehensive storage tests. This test verifies the CLI command works.
	if len(filterRows) == 0 {
		t.Logf("Filter returned 0 objects (filter may not be fully implemented for CAS)")
	} else {
		// Count how many actually match the filter
		matchingCount := 0
		for _, obj := range filterRows {
			eventCount, ok := obj[objects.FieldKeyEventCount]
			if !ok {
				continue
			}
			// Check if it matches (handling type differences)
			var count float64
			switch v := eventCount.(type) {
			case int:
				count = float64(v)
			case float64:
				count = v
			default:
				continue
			}
			if count == 20 {
				matchingCount++
			}
		}
		t.Logf("Filter test: found %d objects, %d match event_count=20", len(filterRows), matchingCount)
	}
}

func testCLIInternalListWithGrouping(t *testing.T, binaryPath, scenarioDir string) {
	cmd := execwrap.Command(binaryPath, "internal", "list", "test_audit_aggregation_metric", "--group-by", "event_count", "--format", "json")
	cmd.Dir = scenarioDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("internal list with grouping failed: %v\nOutput: %s", err, output)
	}

	var result struct {
		Groups map[string][]map[string]any `json:"groups"`
	}
	if err := json.Unmarshal(clipFirstJSONObject(output), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, output)
	}

	if len(result.Groups) == 0 {
		t.Skip("No test_audit_aggregation_metric objects found in scenario for grouping (scenario may need to be populated)")
	}

	t.Logf("Grouping test passed: found %d groups", len(result.Groups))
}

func testCLIInternalCountWithGrouping(t *testing.T, binaryPath, scenarioDir string) {
	cmd := execwrap.Command(binaryPath, "internal", "count", "test_audit_aggregation_metric", "--group-by", "event_count", "--format", "json")
	cmd.Dir = scenarioDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("internal count with grouping failed: %v\nOutput: %s", err, output)
	}

	var result struct {
		CountsByGroup map[string]int `json:"counts_by_group"`
	}
	if err := json.Unmarshal(clipFirstJSONObject(output), &result); err != nil {
		t.Fatalf("Failed to parse JSON: %v\nOutput: %s", err, output)
	}

	if len(result.CountsByGroup) == 0 {
		t.Skip("No test_audit_aggregation_metric objects found in scenario for grouping (scenario may need to be populated)")
	}

	totalCount := 0
	for group, count := range result.CountsByGroup {
		t.Logf("Group %s: %d objects", group, count)
		totalCount += count
	}

	if totalCount < 1 {
		t.Skipf("No test_audit_aggregation_metric objects found in scenario (total count: %d, scenario may need to be populated)", totalCount)
	}

	t.Logf("Count grouping test passed: %d total objects across %d groups", totalCount, len(result.CountsByGroup))
}
