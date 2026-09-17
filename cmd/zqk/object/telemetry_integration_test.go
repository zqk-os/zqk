package object

import (
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/kindnames"
	"github.com/lanceman/zqk/pkg/zqkenv"
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

	// 2. AllowCIOverrides was removed (PRI-ENV-SIGNED-LOGIN-001). CI must hard-block --override.
	t.Setenv(zqkenv.EnvCI().Name(), "true")
	updateArgs := []string{"object", "update", compID, "--field", "status=validated", "--override", "--reason-code=Descriptive justification explaining why this bypass is necessary"}
	out, err = runCLIWithTimeout(t, cliBinary, projectRoot, 30*time.Second, updateArgs...)
	if err == nil {
		t.Fatalf("expected CI to block --override after AllowCIOverrides removal; got success: %s", string(out))
	}
	combined := string(out) + err.Error()
	if !strings.Contains(combined, "blocked in CI") && !strings.Contains(combined, "interactive TTY") {
		t.Fatalf("expected CI/TTY block message for --override, got: %s", combined)
	}
}
