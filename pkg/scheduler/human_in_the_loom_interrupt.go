package scheduler

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/policyinterrupt"
)

// EmitHumanInTheLoomInterrupt pauses autonomous execution and raises a policy interrupt
// for a human to review the convergence state.
func EmitHumanInTheLoomInterrupt(
	ctx context.Context,
	projectRoot string,
	sessionID string,
	reason string,
	policyID string,
	confidence float64,
	isStreaming bool,
	decisionBranch string,
) error {
	if projectRoot == emptyValue {
		return errfmt.Errorf("project root is required to emit interrupt")
	}

	dedupeKey := fmt.Sprintf("hitl-%s-%s", sessionID, policyID)

	// Emit the interrupt to the WAL
	rec := policyinterrupt.InterruptRecord{
		Profile:          objects.KindPolicy,
		Severity:         "critical",
		AckRequired:      true,
		DedupeKey:        dedupeKey,
		PolicyID:         policyID,
		Message:          fmt.Sprintf("Human-in-the-Loom approval required for session %s: %s", sessionID, reason),
		SuggestedAction:  "zqk system policy-interrupts ack --dedupe-key " + dedupeKey,
		ExpiresAtRFC3339: time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339),
		OriginOperation:  "convergence_router",
		OriginActorID:    "system", // Typically the orchestrator/agent
		Confidence:       confidence,
		IsStreaming:      isStreaming,
		DecisionBranch:   decisionBranch,
	}

	if err := policyinterrupt.AppendInterrupt(projectRoot, rec); err != nil {
		return errfmt.Newf("failed to append HITL interrupt").Wrap(err)
	}

	// Telemetry broadcast
	eventData := &coordination.EventData{
		AuditMetadata: map[string]any{
			objects.FieldKeySessionID: sessionID,
			"policy_id":               policyID,
			"dedupe_key":              dedupeKey,
			objects.FieldKeyReason:    reason,
		},
		LoggingFields: []coordination.LoggingField{
			{Key: "message", Value: fmt.Sprintf("HITL Interrupt Emitted: %s", reason)},
			{Key: "target_kind", Value: objects.KindConvergenceSession},
			{Key: "target_id", Value: sessionID},
		},
	}

	eventCtx := coordination.NewEventContext(dedupeKey, "policy_interrupt", "start").
		WithEventData(eventData).
		WithChannels(true, true, true, true)

	coordinator := coordination.GetCoordinator()
	if coordinator != nil {
		if err := coordinator.Emit(ctx, eventCtx); err != nil {
			logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
			SLog(logger).Debug("Failed to emit HITL policy interrupt").WithError(err).Log()
		}
	}

	return nil
}
