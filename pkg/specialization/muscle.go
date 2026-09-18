package specialization

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/logging"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/storage"
)

// MuscleHandler executes approved strategic plans and ingests data.
type MuscleHandler struct {
	store storage.ObjectStorageProvider
	spine infrastructure.SpinalSpine
}

func (h *MuscleHandler) Initialize(ctx context.Context, store storage.ObjectStorageProvider, spine infrastructure.SpinalSpine) error {
	h.store = store
	h.spine = spine
	return nil
}

func (h *MuscleHandler) Start(ctx context.Context) error {
	// Subscribe to backlog_item proposals
	return h.spine.Subscribe(ctx, objects.KindBacklogItem, h.handleBacklogProposal)
}

func (h *MuscleHandler) handleBacklogProposal(ctx context.Context, event infrastructure.Event) error {
	if event.Op == "propose" {
		title, _ := event.Payload[objects.FieldKeyTitle].(string)
		priority, _ := event.Payload[objects.FieldKeyPriorityTier].(string)

		logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("[MUSCLE] Received proposal: %s (Priority: %s). Validating for execution...\n", title, priority)).Log()

		// In a real scenario, this would check against security policies and resource availability
		// For now, we simulate "acceptance" and "start of work"
		if priority == "P1" {
			logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("[MUSCLE] P1 detected. Fast-tracking execution of %s\n", event.ObjectID)).Log()

			// Update status to in_progress locally (simulated)
			event.Payload[objects.FieldKeyStatus] = "in_progress"

			// Publish execution update
			execEvent := infrastructure.Event{
				ObjectID: event.ObjectID,
				Kind:     objects.KindBacklogItem,
				Op:       "execute",
				Payload:  event.Payload,
			}
			_ = h.spine.Publish(ctx, execEvent)
		}
	}

	return nil
}

func (h *MuscleHandler) Stop() error {
	return nil
}
