//go:build !zqk_omit_releasepack

package objects

func init() {
	AddModuleSpecRoot("packs/release/specs")
	AddModuleLifecycleRoot("packs/release/lifecycles")
}
