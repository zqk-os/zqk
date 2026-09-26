package docman_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// Satisfies CRIT-PHASE11-OPERABILITY-RUNBOOKS:
// Automated test verifying operational incident runbook coverage, structure, and integrity.
func TestRunbooksIntegrity(t *testing.T) {
	runbooksDir := filepath.Join("..", "..", "docs", "runbooks")
	if _, err := fileutil.Stat(runbooksDir); fileutil.IsNotExist(err) {
		runbooksDir = filepath.Join("docs", "runbooks")
	}

	requiredRunbooks := []string{
		"README.md",
		"RB-CAS-001-CAS-CORRUPTION-RECOVERY.md",
		"RB-WAL-001-WAL-COMPACTION-FAILURES.md",
		"RB-LCK-001-LOCK-CONTENTION-DEADLOCKS.md",
		"RB-SCH-001-SCHEDULER-DAEMON-TRIAGE.md",
	}

	for _, filename := range requiredRunbooks {
		path := filepath.Join(runbooksDir, filename)
		data, err := fileutil.ReadFile(path)
		require.NoError(t, err, "runbook file should exist: %s", path)
		content := string(data)
		require.NotEmpty(t, content, "runbook content should not be empty")

		if filename != "README.md" {
			require.Contains(t, content, "## Metadata", "runbook should contain Metadata section: %s", filename)
			require.Contains(t, content, "## Symptoms & Alerts", "runbook should contain Symptoms section: %s", filename)
			require.Contains(t, content, "## Root Cause Analysis", "runbook should contain Root Cause section: %s", filename)
			require.Contains(t, content, "## Step-by-Step Remediation Procedure", "runbook should contain Step-by-Step Remediation: %s", filename)
			require.Contains(t, content, "Verification Gate", "runbook should contain Verification Gate: %s", filename)
		} else {
			require.Contains(t, content, "# Operational Incident Runbooks", "index should contain title")
			for _, item := range requiredRunbooks[1:] {
				require.True(t, strings.Contains(content, item), "index must reference runbook %s", item)
			}
		}
	}
}
