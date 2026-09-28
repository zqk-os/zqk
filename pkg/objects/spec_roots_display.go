//go:build !zqk_omit_displaypack

package objects

func init() {
	AddModuleSpecRoot("packs/display/specs")
	AddModuleLifecycleRoot("packs/display/lifecycles")
}
