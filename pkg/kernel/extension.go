package kernel

import "context"

// Extension defines a pluggable capability that attaches to the Knowledge Kernel.
type Extension interface {
	// Name returns a unique identifier for the extension.
	Name() string

	// Init initializes the extension with the host Knowledge Kernel instance.
	Init(ctx context.Context, k KnowledgeKernel) error

	// Shutdown cleans up any background resources, listeners, or state held by the extension.
	Shutdown(ctx context.Context) error
}
