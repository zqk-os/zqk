//go:build !zqk_omit_decisionpack

package objects

func init() {
	AddModuleSpecRoot("packs/decision/specs")
	AddModuleLifecycleRoot("packs/decision/lifecycles")
}
