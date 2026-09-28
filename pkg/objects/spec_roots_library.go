//go:build !zqk_omit_librarypack

package objects

func init() {
	AddModuleSpecRoot("packs/library/specs")
	AddModuleLifecycleRoot("packs/library/lifecycles")
}
