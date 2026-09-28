//go:build !zqk_omit_qapack

package objects

func init() {
	AddModuleSpecRoot("packs/qa/specs")
	AddModuleLifecycleRoot("packs/qa/lifecycles")
}
