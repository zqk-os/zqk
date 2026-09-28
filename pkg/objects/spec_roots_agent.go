//go:build !zqk_omit_agentpack

package objects

func init() {
	AddModuleSpecRoot("packs/agent/specs")
	AddModuleLifecycleRoot("packs/agent/lifecycles")
}
