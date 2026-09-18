package specialization

import (
	"context"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/storage"
)

// ShieldedHandler wraps a Handler to ensure it operates within a 'Rubber Room'.
// It initializes the inner handler with a ShadowSpine, shielding the main
// infrastructure from any side-effects of the maturation process.
type ShieldedHandler struct {
	inner Handler
	spine *infrastructure.ShadowSpine
}

// NewShieldedHandler creates a new ShieldedHandler wrapping the provided handler.
func NewShieldedHandler(inner Handler) *ShieldedHandler {
	return &ShieldedHandler{
		inner: inner,
	}
}

// Initialize prepares the shielded handler. It wraps the provided spine
// in a ShadowSpine before passing it to the inner handler.
func (h *ShieldedHandler) Initialize(ctx context.Context, store storage.ObjectStorageProvider, spine infrastructure.SpinalSpine) error {
	h.spine = infrastructure.NewShadowSpine(spine)
	return h.inner.Initialize(ctx, store, h.spine)
}

// Start begins the inner handler's subscription loop.
func (h *ShieldedHandler) Start(ctx context.Context) error {
	return h.inner.Start(ctx)
}

// Stop gracefully shuts down the inner handler.
func (h *ShieldedHandler) Stop() error {
	return h.inner.Stop()
}

// GetShadowEvents returns events that were diverted to the shadow spine.
func (h *ShieldedHandler) GetShadowEvents() []infrastructure.Event {
	if h.spine == nil {
		return nil
	}
	return h.spine.GetShadowEvents()
}
