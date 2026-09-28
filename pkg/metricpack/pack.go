package metricpack

import (
	_ "github.com/zqk-os/zqk/packs/metric/bldr_instance_v1"
	_ "github.com/zqk-os/zqk/packs/metric/bldr_lifecycle_v1"
	_ "github.com/zqk-os/zqk/packs/metric/bldr_v2"
)

const (
	// Name is the pack id the composition root links.
	Name = "metric"
	// SpecDir is the object spec directory this pack owns.
	SpecDir = "packs/metric/specs"
	// LifecycleDir is the lifecycle directory this pack owns.
	LifecycleDir = "packs/metric/lifecycles"
	// OmitBuildTag drops this pack from the zqk composition root.
	OmitBuildTag = "zqk_omit_metricpack"
)

// Kinds are the object kinds this pack owns.
func Kinds() []string {
	return []string{
		"audit_aggregation_metric",
		"base_sampler",
		"command_metric",
		"file_lock_metric",
		"kind_mapping_metric",
		"list_metric_sampler",
		"metadata_package",
		"ordered_list_metric_sampler",
		"sampler_profile",
		"scalar_metric_sampler",
		"scheduler_health_metric",
		"status_history_metric_sampler",
	}
}
