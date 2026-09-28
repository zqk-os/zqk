package agentpack

import (
	_ "github.com/zqk-os/zqk/packs/agent/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/agent/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/agent/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "agent"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/agent/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/agent/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_agentpack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"agent_architecture",
		"agent_feed",
		"agent_instruction",
		"agent_onboarding_preparation",
		"agent_skill",
		"agent_task",
		"mcp_built_in_tool",
		"mcp_session",
		"mcp_spec",
		"persona",
		"prompt_template",
		"provider_profile",
	}
}
