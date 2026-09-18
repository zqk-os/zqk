package specialization

import (
	"context"
	"fmt"

	"github.com/zqk-os/zqk/pkg/logging"

	"github.com/zqk-os/zqk/pkg/infrastructure"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/semantic"
	"github.com/zqk-os/zqk/pkg/storage"
)

// NeuronHandler senses operational drift and generates strategic inferences.
type NeuronHandler struct {
	store      storage.ObjectStorageProvider
	spine      infrastructure.SpinalSpine
	reconciler *semantic.SemanticReconciler
	engine     *semantic.InferenceEngine
}

func (h *NeuronHandler) Initialize(ctx context.Context, store storage.ObjectStorageProvider, spine infrastructure.SpinalSpine) error {
	h.store = store
	h.spine = spine
	h.reconciler = semantic.NewSemanticReconciler(store)
	h.engine = semantic.NewInferenceEngine(store)
	return nil
}

func (h *NeuronHandler) Start(ctx context.Context) error {
	// Subscribe to convergence session updates to detect drift in real-time
	return h.spine.Subscribe(ctx, objects.KindConvergenceSession, h.handleConvergenceUpdate)
}

func (h *NeuronHandler) handleConvergenceUpdate(ctx context.Context, event infrastructure.Event) error {
	// 1. Extract session state
	status, _ := event.Payload[objects.FieldKeyStatus].(string)
	delta, _ := event.Payload[objects.FieldKeyDeltaAssessment].(string)

	if status == "active" && delta == "trending_away" {
		// 2. Perform inference
		logging.FluentEvent(logging.GetLogger()).Info(fmt.Sprintf("[NEURON] Drift detected in session %s. Running inference...\n", event.ObjectID)).Log()

		res := semantic.ConvergenceResult{
			SessionID:       event.ObjectID,
			Title:           fmt.Sprintf("%v", event.Payload[objects.FieldKeyTitle]),
			DeltaAssessment: delta,
			TrendingAway:    true,
		}

		inferences := h.engine.InferFromConvergence([]semantic.ConvergenceResult{res})

		// 3. Publish proposed backlog items back to the mesh
		for _, inf := range inferences {
			infEvent := infrastructure.Event{
				ObjectID: fmt.Sprintf("%v", inf[objects.FieldKeyID]),
				Kind:     objects.KindBacklogItem,
				Op:       "propose",
				Payload:  inf,
			}
			_ = h.spine.Publish(ctx, infEvent)
		}
	}

	return nil
}

func (h *NeuronHandler) Stop() error {
	return nil
}
