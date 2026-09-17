package orchestration

import (
	"sync/atomic"
)

// OrchestratorRegistry handles the lifecycle and injection of orchestrator services.
type OrchestratorRegistry struct {
	manager atomic.Pointer[OrchestrationManager]
}

var globalRegistry = &OrchestratorRegistry{}

func GetRegistry() *OrchestratorRegistry {
	return globalRegistry
}

func (r *OrchestratorRegistry) RegisterManager(m OrchestrationManager) {
	r.manager.Store(&m)
}

func (r *OrchestratorRegistry) GetManager() OrchestrationManager {
	ptr := r.manager.Load()
	if ptr == nil {
		return nil
	}
	return *ptr
}
