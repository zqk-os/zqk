//go:build !zqk_omit_interfacepack

package objects

func init() {
	AddModuleSpecRoot("packs/interface/specs")
	AddModuleLifecycleRoot("packs/interface/lifecycles")
}
