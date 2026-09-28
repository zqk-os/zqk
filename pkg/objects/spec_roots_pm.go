//go:build !zqk_omit_pmpack

package objects

func init() {
	AddModuleSpecRoot("packs/pm/specs")
	AddModuleLifecycleRoot("packs/pm/lifecycles")
}
