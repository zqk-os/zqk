//go:build !zqk_omit_surfacepack

package objects

func init() {
	AddModuleSpecRoot("packs/surface/specs")
	AddModuleLifecycleRoot("packs/surface/lifecycles")
}
