package scheduler

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

// getSchedulerJobKind returns the kind name for scheduler jobs.
// SpecLoader already memos the YAML; do not stack a second Once.
func getSchedulerJobKind() string {
	spec, err := objects.GetGlobalSpecLoader().LoadSpecWithInheritance(schedulerFileSchedulerJobYAML)
	if err != nil || spec == nil || spec.Ontology == emptyValue {
		return schedulerKindJob
	}
	return spec.Ontology
}
