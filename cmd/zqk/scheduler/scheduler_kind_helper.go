package scheduler

import (
	"sync"

	"github.com/zqk-os/zqk/pkg/objects"
)

var (
	// Cached scheduler job kind name (loaded from scheduler_job spec)
	schedulerJobKindCache     string
	schedulerJobKindCacheOnce sync.Once
	schedulerJobKindCacheErr  error
)

// getSchedulerJobKind returns the kind name for scheduler jobs
// Dynamically loaded from the scheduler_job spec and cached for performance
// Falls back to schedulerKindJob if spec can't be loaded
func getSchedulerJobKind() string {
	schedulerJobKindCacheOnce.Do(func() {
		// Load scheduler_job spec
		specLoader := objects.GetGlobalSpecLoader()
		spec, err := specLoader.LoadSpecWithInheritance(schedulerFileSchedulerJobYAML)
		if err != nil {
			schedulerJobKindCacheErr = err
			// Fallback to known value if spec can't be loaded
			schedulerJobKindCache = schedulerKindJob
			return
		}

		// Extract ontology (kind name) from spec
		if spec != nil && spec.Ontology != emptyValue {
			schedulerJobKindCache = spec.Ontology
		} else {
			// Fallback to known value if ontology not found
			schedulerJobKindCache = schedulerKindJob
		}
	})

	return schedulerJobKindCache
}
