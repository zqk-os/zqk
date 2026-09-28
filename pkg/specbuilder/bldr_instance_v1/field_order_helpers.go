package bldr_instance_v1

import "github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"

// buildFieldOrderFromSpec keeps generated builders on the spec-derived field order.
func buildFieldOrderFromSpec(ontology string) []string {
	return instance_builders.FieldOrderFromSpec(ontology)
}
