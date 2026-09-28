//go:build !zqk_omit_vocabularypack

package objects

func init() {
	AddModuleSpecRoot("packs/vocabulary/specs")
	AddModuleLifecycleRoot("packs/vocabulary/lifecycles")
}
