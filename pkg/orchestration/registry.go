package orchestration

import (
	"sync"
)

// OrchestratorRegistry handles the lifecycle and injection of orchestrator services.
type OrchestratorRegistry struct {
	mu      sync.RWMutex
	manager OrchestrationManager
}

var globalRegistry = &OrchestratorRegistry{}

func GetRegistry() *OrchestratorRegistry {
	return globalRegistry
}

func (r *OrchestratorRegistry) RegisterManager(m OrchestrationManager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.manager = m
}

func (r *OrchestratorRegistry) GetManager() OrchestrationManager {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.manager
}
