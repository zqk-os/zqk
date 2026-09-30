package bldr_cli_cmd_v1

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestTaxonomyDescriptions_NonTautological(t *testing.T) {
	builders := []struct {
		name string
		cmd  *cobra.Command
	}{
		{"pre-commit", NewPreCommitCommandBuilder()},
		{"scheduler", NewSchedulerCommandBuilder()},
		{"system-federate-handshake", NewSystemFederateHandshakeCommandBuilder()},
		{"mesh-market", NewMeshMarketCommandBuilder()},
		{"system-evolve", NewSystemEvolveCommandBuilder()},
		{"system-federate", NewSystemFederateCommandBuilder()},
		{"system-metrics-scheduler-health", NewSystemMetricsSchedulerHealthCommandBuilder()},
		{"mesh-advertise", NewMeshAdvertiseCommandBuilder()},
		{"mesh-lease", NewMeshLeaseCommandBuilder()},
		{"workflow", NewWorkflowCommandBuilder()},
		{"system-dashboard", NewSystemDashboardCommandBuilder()},
		{"mesh", NewMeshCommandBuilder()},
		{"auth", NewAuthCommandBuilder()},
		{"auth-login", NewAuthLoginCommandBuilder()},
		{"version", NewVersionCommandBuilder()},
		{"sync-id-prefixes-from-specs", NewSystemSyncIdPrefixesFromSpecsCommandBuilder()},
		{"completion", NewCompletionCommandBuilder()},
		{"use", NewUseCommandBuilder()},
		{"file-lock", NewSystemMetricsFileLockCommandBuilder()},
		{"join", NewJoinCommandBuilder()},
		{"infer", NewSemanticInferCommandBuilder()},
		{"disable", NewHealthchkBulkDisableCommandBuilder()},
		{"logout", NewAuthLogoutCommandBuilder()},
		{"initiate", NewSystemFederateInitiateCommandBuilder()},
	}

	for _, tc := range builders {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cmd == nil {
				t.Fatalf("expected command %s to not be nil", tc.name)
			}
			short := tc.cmd.Short
			if strings.HasSuffix(strings.ToLower(short), " command") {
				t.Errorf("command %s has tautological short description: %q", tc.name, short)
			}
			if len(short) < 10 {
				t.Errorf("command %s has excessively terse short description: %q", tc.name, short)
			}
		})
	}
}
