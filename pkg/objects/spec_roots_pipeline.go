//go:build !zqk_omit_pipelinepack

package objects

func init() {
	AddModuleSpecRoot("packs/pipeline/specs")
	AddModuleLifecycleRoot("packs/pipeline/lifecycles")
}
