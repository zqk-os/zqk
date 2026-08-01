package object

import (
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/kindnames"
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

	updateArgs := []string{"object", "update", tdeID, "--field", "status=resolved", "--override", "--reason-code", "testing the regression validation of this bypass because we need to override the warning checks"}
	out, err = runCLIWithTimeout(t, cliBinary, projectRoot, 30*time.Second, updateArgs...)
	if err != nil {
		t.Fatalf("Failed to update technical_debt. ID validation regression: %v\nOutput: %s", err, string(out))
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
