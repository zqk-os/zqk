package mesh

import (
	"context"
	"errors"
	"sync"
)

var ErrKernelNotFound = errors.New("remote kernel not found")

// RemoteKernel represents a trusted peer in the Sovereign Mesh.
type RemoteKernel struct {
	ID        string
	PublicKey string
	Endpoint  string
}

// Registry manages the identity of trusted remote kernels.
type Registry interface {
	Verify(ctx context.Context, kernelID string) (*RemoteKernel, error)
	Register(ctx context.Context, kernel RemoteKernel) error
}

type InMemoryRegistry struct {
	mu      sync.RWMutex
	kernels map[string]*RemoteKernel
}

func NewInMemoryRegistry() *InMemoryRegistry {
	return &InMemoryRegistry{
		kernels: make(map[string]*RemoteKernel),
	}
}

func (r *InMemoryRegistry) Verify(ctx context.Context, kernelID string) (*RemoteKernel, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	k, ok := r.kernels[kernelID]
	if !ok {
		return nil, ErrKernelNotFound
	}
	return k, nil
}

func (r *InMemoryRegistry) Register(ctx context.Context, kernel RemoteKernel) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	k := kernel
	r.kernels[kernel.ID] = &k
	return nil
}
