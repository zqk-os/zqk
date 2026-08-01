package scheduler

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/logging"

	"github.com/lanceman/zqk/pkg/coordination"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_enum_v1/audit_event"
)

// EmitConvergenceEvaluationEvent emits a telemetry event representing a convergence evaluation.
// This is typically called by the coordinator when a Phase Router evaluation completes.
func EmitConvergenceEvaluationEvent(
	ctx context.Context,
	projectRoot string,
	sessionID string,
	flowVariant string,
	result *PhaseRouterResult,
	disparity map[string]any,
) {
	if projectRoot == emptyValue {
		return
	}

	metadata := map[string]any{
		objects.FieldKeySessionID:   sessionID,
		objects.FieldKeyFlowVariant: flowVariant,
		"routing_profile":           result.RoutingProfileID,
		"implied_phase":             string(result.MeasurementImpliedPhase),
		"suggested_phase":           string(result.SuggestedCurrentPhase),
		"phase_alignment":           result.PhaseAlignment,
		"disparity_active":          disparity["active_tombstone"],
	}

	eventData := &coordination.EventData{
		AuditMetadata: metadata,
		LoggingFields: []coordination.LoggingField{
			{Key: "message", Value: fmt.Sprintf("Convergence Evaluation: Session %s (Profile: %s)", sessionID, result.RoutingProfileID)},
			{Key: "target_kind", Value: objects.KindConvergenceSession},
			{Key: "target_id", Value: sessionID},
		},
	}

	eventCtx := coordination.NewEventContext(fmt.Sprintf("conv_eval_%s_%d", sessionID, time.Now().Unix()), string(audit_event.EventTypeCommandExecution), "complete").
		WithEventData(eventData).
		WithChannels(true, true, true, true) // Enable all routing channels for this event

	coordinator := coordination.GetCoordinator()
	if coordinator != nil {
		if err := coordinator.Emit(ctx, eventCtx); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to emit convergence evaluation event").WithError(err).Log()
		}
	}
}
