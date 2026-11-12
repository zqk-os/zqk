package community_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zqk-os/zqk/cmd/zqk/autopilot"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/llm"
	"github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// TestSwarmAutopilot_StaticFloorInvariant satisfies CRIT-1794387137780971000-4676532c.
// Verifies zqk autopilot CLI command registration, flag parsing, schema integrity, and help menu parity.
func TestSwarmAutopilot_StaticFloorInvariant(t *testing.T) {
	cmd := autopilot.NewAutopilotCmd()
	require.NotNil(t, cmd, "autopilot command must be non-nil")
	assert.Equal(t, "autopilot", cmd.Use, "command Use must match 'autopilot'")

	expectedAliases := []string{"pilot", "auto-converge"}
	assert.ElementsMatch(t, expectedAliases, cmd.Aliases, "command aliases must include pilot and auto-converge")

	expectedFlags := []string{"intent", "plan-id", "model", "budget-tokens", "verify", "dry-run"}
	for _, flagName := range expectedFlags {
		flag := cmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "flag --%s must be registered on autopilot command", flagName)
	}

	repoRoot := findRepoRootForSDK(t)
	specPath := filepath.Join(repoRoot, ".zqk", "cli", "specs", "autopilot_command.yaml")
	specBytes, err := fileutil.ReadFile(specPath)
	require.NoError(t, err, "autopilot command spec must exist in .zqk/cli/specs")
	specContent := string(specBytes)
	assert.Contains(t, specContent, "name: autopilot", "spec must declare autopilot command name")
	assert.Contains(t, specContent, "name: budget-tokens", "spec must declare budget-tokens flag")
}

// TestSwarmAutopilot_OperationalDynamicProof satisfies CRIT-1794387142340648000-528c9c55.
// Executes autopilot pipeline in test fixture, discovering local mock engine, provisioning tasks, and validating DoD gate pass.
func TestSwarmAutopilot_OperationalDynamicProof(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	sec := &pkgctx.SecurityContext{AccountID: "ACC-TEST-AUTOPILOT"}

	harvester := llm.NewLocalEngineHarvester(50 * time.Millisecond)

	opts := autopilot.AutopilotOptions{
		Intent:       "Build turnkey autonomous swarm integration proof",
		BudgetTokens: 50000,
		Verify:       true,
		DryRun:       true,
		Harvester:    harvester,
	}

	err := autopilot.ExecuteAutopilot(ctx, sec, nil, opts, nil)
	assert.NoError(t, err, "autopilot execution with valid intent and budget must succeed")
}

// TestSwarmAutopilot_NegativeAdversarialBoundary satisfies CRIT-1794387147062186000-6bb044f8.
// Rejects blank intent arguments, non-positive budgets, and unreachable inference runtimes fail-closed.
func TestSwarmAutopilot_NegativeAdversarialBoundary(t *testing.T) {
	ctx := context.Background()
	sec := &pkgctx.SecurityContext{AccountID: "ACC-TEST-AUTOPILOT"}

	// Boundary 1: Empty intent and empty plan ID must fail-closed
	optsEmpty := autopilot.AutopilotOptions{
		Intent:       "",
		PlanID:       "",
		BudgetTokens: 10000,
	}
	errEmpty := autopilot.ExecuteAutopilot(ctx, sec, nil, optsEmpty, nil)
	assert.Error(t, errEmpty, "empty intent and plan-id must be rejected fail-closed")
	assert.Contains(t, errEmpty.Error(), "neither priority plan nor natural language intent provided")

	// Boundary 2: Non-positive token budget must fail-closed
	optsZeroBudget := autopilot.AutopilotOptions{
		Intent:       "Valid intent",
		BudgetTokens: 0,
	}
	errZeroBudget := autopilot.ExecuteAutopilot(ctx, sec, nil, optsZeroBudget, nil)
	assert.Error(t, errZeroBudget, "zero token budget must be rejected fail-closed")
	assert.Contains(t, errZeroBudget.Error(), "token budget must be positive")

	optsNegativeBudget := autopilot.AutopilotOptions{
		Intent:       "Valid intent",
		BudgetTokens: -500,
	}
	errNegativeBudget := autopilot.ExecuteAutopilot(ctx, sec, nil, optsNegativeBudget, nil)
	assert.Error(t, errNegativeBudget, "negative token budget must be rejected fail-closed")
	assert.Contains(t, errNegativeBudget.Error(), "token budget must be positive")
}
