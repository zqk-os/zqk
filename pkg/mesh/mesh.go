package mesh

import (
	"context"
	"sync"
)

// CapacityAdvertisement represents available compute/intelligence availability.
type CapacityAdvertisement struct {
	KernelID string
	Compute  float64
}

// ToolPodRegistration represents a dynamically registered tool pod.
type ToolPodRegistration struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`     // e.g., "ffmpeg", "python-executor"
	Endpoint    string            `json:"endpoint"` // e.g., "http://localhost:8081"
	Description string            `json:"description"`
	Metadata    map[string]string `json:"metadata"`
}

// Mesh defines the communication interface for federated agents.
type Mesh interface {
	BroadcastCapacity(ctx context.Context, cap CapacityAdvertisement) error
	RegisterToolPod(ctx context.Context, tp ToolPodRegistration) error
	DiscoverToolPods(ctx context.Context) ([]ToolPodRegistration, error)
}

type InMemoryMesh struct {
	mu         sync.RWMutex
	capacities []CapacityAdvertisement
	toolPods   map[string]ToolPodRegistration
}

func NewInMemoryMesh() *InMemoryMesh {
	return &InMemoryMesh{
		capacities: make([]CapacityAdvertisement, 0),
		toolPods:   make(map[string]ToolPodRegistration),
	}
}

func (m *InMemoryMesh) BroadcastCapacity(ctx context.Context, cap CapacityAdvertisement) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capacities = append(m.capacities, cap)
	return nil
}

func (m *InMemoryMesh) GetCapacities() []CapacityAdvertisement {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]CapacityAdvertisement, len(m.capacities))
	copy(res, m.capacities)
	return res
}

func (m *InMemoryMesh) RegisterToolPod(ctx context.Context, tp ToolPodRegistration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.toolPods[tp.ID] = tp
	return nil
}

func (m *InMemoryMesh) DiscoverToolPods(ctx context.Context) ([]ToolPodRegistration, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]ToolPodRegistration, 0, len(m.toolPods))
	for _, tp := range m.toolPods {
		res = append(res, tp)
	}
	return res, nil
}
