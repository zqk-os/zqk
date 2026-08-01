package object

import (
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/kindnames"
)

func TestSnapRemedyTelemetryCycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	tmpDir, cliBinary := setupCLITestEnvironmentForComprehensive(t)
	projectRoot := tmpDir

	// 1. Create a baseline object (e.g., a component)
	compID := "COMP-telemetry-test-01"
	createArgs := []string{"object", "create", kindnames.Component, "--data", "{\"id\": \"" + compID + "\", \"title\": \"Test Component\", \"status\": \"created\", \"component_type\": \"service\", \"spec_context_broker\": \"none\", \"spec_interpreter\": \"none\", \"domain\": \"api\"}"}

	out, err := runCLIWithTimeout(t, cliBinary, projectRoot, 30*time.Second, createArgs...)
	if err != nil {
		t.Fatalf("Failed to create component: %v\nOutput: %s", err, string(out))
	}

	// 2. Perform a snap-remedy (manual override of status)
	updateArgs := []string{"object", "update", compID, "--field", "status=validated", "--override", "--reason-code=Descriptive justification explaining why this bypass is necessary"}
	out, err = runCLIWithTimeout(t, cliBinary, projectRoot, 30*time.Second, updateArgs...)
	if err != nil {
		t.Fatalf("Failed to override component status: %v\nOutput: %s", err, string(out))
	}

	// Ensure the output contains the anti-pattern warning
	if !strings.Contains(string(out), "WARNING: Manual status override detected") {
		t.Fatalf("Expected anti-pattern warning in output, got: %s", string(out))
	}

	// 3. Verify the telemetry (technical_debt) was generated in the background.
	// Wait briefly for the fire-and-forget goroutine to persist it.
	time.Sleep(2 * time.Second)

	listArgs := []string{"object", "list", kindnames.TechnicalDebt, "--format", "json"}
	out, err = runCLIWithTimeout(t, cliBinary, projectRoot, 30*time.Second, listArgs...)
	if err != nil {
		t.Fatalf("Failed to list technical_debt: %v\nOutput: %s", err, string(out))
	}

	if !strings.Contains(string(out), compID) {
		t.Fatalf("Expected to find a technical_debt object referencing the overridden component %s, but didn't. Output: %s", compID, string(out))
	}
}
