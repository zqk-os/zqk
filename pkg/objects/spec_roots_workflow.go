//go:build !zqk_omit_workflowpack

package objects

func init() {
	AddModuleSpecRoot("packs/workflow/specs")
	AddModuleLifecycleRoot("packs/workflow/lifecycles")
}
