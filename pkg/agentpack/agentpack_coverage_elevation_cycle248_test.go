package agentpack_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/zqk-os/zqk/pkg/agentpack"
)

// TestCycle248_AgentPackConstantsAndKinds tests package constants, kind enumeration, and uniqueness.
func TestCycle248_AgentPackConstantsAndKinds(t *testing.T) {
	t.Run("ConstantsValidation", func(t *testing.T) {
		assert.Equal(t, "agent", agentpack.Name)
		assert.Equal(t, "packs/agent/specs", agentpack.SpecDir)
		assert.Equal(t, "packs/agent/lifecycles", agentpack.LifecycleDir)
		assert.Equal(t, "zqk_omit_agentpack", agentpack.OmitBuildTag)
	})

	t.Run("KindsEnumerationAndIntegrity", func(t *testing.T) {
		kinds := agentpack.Kinds()
		require.NotEmpty(t, kinds)
		assert.Equal(t, 13, len(kinds))

		expectedKinds := []string{
			"agent_architecture",
			"agent_feed",
			"agent_instruction",
			"agent_onboarding_preparation",
			"agent_skill",
			"agent_task",
			"context_refresh_schedule",
			"mcp_built_in_tool",
			"mcp_session",
			"mcp_spec",
			"persona",
			"prompt_template",
			"provider_profile",
		}

		assert.Equal(t, expectedKinds, kinds)

		seen := make(map[string]bool)
		for _, k := range kinds {
			assert.NotEmpty(t, k)
			assert.False(t, seen[k], "duplicate kind found: "+k)
			seen[k] = true
		}
	})
}
