package object

import (
	"testing"
	"time"

	"github.com/zqk-os/zqk/pkg/kindnames"
)

func TestTechnicalDebtIDValidationRegression(t *testing.T) {
	tmpDir, cliBinary := setupCLITestEnvironmentForComprehensive(t)
	projectRoot := tmpDir

	tdeID := "TDE-1234567890-test"
	createArgs := []string{"object", "create", kindnames.TechnicalDebt, "--data", "{\"id\": \"" + tdeID + "\", \"title\": \"Test Debt\", \"status\": \"identified\", \"debt_type\": \"tooling\", \"description\": \"A test debt\", \"target_resolution_date\": \"2026-12-31\"}"}

	out, err := runCLIWithTimeout(t, cliBinary, projectRoot, 30*time.Second, createArgs...)
	if err != nil {
		t.Fatalf("Failed to create technical_debt: %v\nOutput: %s", err, string(out))
	}

	// Lifecycle: promote (not --override). AllowCIOverrides was removed (PRI-ENV-SIGNED-LOGIN-001).
	updateArgs := []string{
		"object", "update", tdeID,
		"--field", "impact_assessment=medium",
	}
	out, err = runCLIWithTimeout(t, cliBinary, projectRoot, 30*time.Second, updateArgs...)
	if err != nil {
		t.Fatalf("Failed to update technical_debt fields: %v\nOutput: %s", err, string(out))
	}
	promoteArgs := []string{"object", "promote", tdeID}
	out, err = runCLIWithTimeout(t, cliBinary, projectRoot, 30*time.Second, promoteArgs...)
	if err != nil {
		t.Fatalf("Failed to promote technical_debt (ID validation regression): %v\nOutput: %s", err, string(out))
	}

	getArgs := []string{"object", "get", tdeID, "--format", "json"}
	out, err = runCLIWithTimeout(t, cliBinary, projectRoot, 30*time.Second, getArgs...)
	if err != nil {
		t.Fatalf("Failed to get technical_debt: %v\nOutput: %s", err, string(out))
	}

	listArgs := []string{"object", "list", kindnames.TechnicalDebt, "--format", "json"}
	out, err = runCLIWithTimeout(t, cliBinary, projectRoot, 30*time.Second, listArgs...)
	if err != nil {
		t.Fatalf("Failed to list technical_debt: %v\nOutput: %s", err, string(out))
	}
}
