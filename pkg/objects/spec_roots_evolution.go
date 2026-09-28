//go:build !zqk_omit_evolutionpack

package objects

func init() {
	AddModuleSpecRoot("packs/evolution/specs")
	AddModuleLifecycleRoot("packs/evolution/lifecycles")
}
