//go:build !zqk_omit_metricpack

package objects

func init() {
	AddModuleSpecRoot("packs/metric/specs")
	AddModuleLifecycleRoot("packs/metric/lifecycles")
}
