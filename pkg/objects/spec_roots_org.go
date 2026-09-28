//go:build !zqk_omit_orgpack

package objects

func init() {
	AddModuleSpecRoot("packs/org/specs")
	AddModuleLifecycleRoot("packs/org/lifecycles")
}
