package qa

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lanceman/zqk/pkg/policyinterrupt"
	"github.com/lanceman/zqk/pkg/validation"
)

// InterruptEmitter handles the emission of policy interrupts for QA disparities.
type InterruptEmitter struct {
	projectRoot string
}

func NewInterruptEmitter(projectRoot string) *InterruptEmitter {
	return &InterruptEmitter{projectRoot: projectRoot}
}

// EmitDisparityInterrupt triggers a high-severity policy interrupt for a structural disparity.
func (e *InterruptEmitter) EmitDisparityInterrupt(ctx context.Context, itemID string, reason string) error {
	record := policyinterrupt.InterruptRecord{
		Ts:          time.Now(),
		Profile:     "policy",
		Severity:    policyinterrupt.SeverityCritical,
		AckRequired: true,
		DedupeKey:   DisparityDedupeKey(itemID),
		Message:     fmt.Sprintf(validation.ConstMagic3022c45f, itemID, reason),
	}
	return policyinterrupt.AppendInterrupt(e.projectRoot, record)
}

// DisparityDedupeKey is the policy-interrupt dedupe key for a QA disparity on itemID.
func DisparityDedupeKey(itemID string) string {
	return fmt.Sprintf(validation.ConstMagicExtracted_18, itemID)
}

// AckDisparityOnPass acknowledges any pending qa-disparity interrupt for itemID after a clean audit.
// Best-effort: never fails the caller's audit success path.
func (e *InterruptEmitter) AckDisparityOnPass(itemID string) {
	if e == nil || strings.TrimSpace(itemID) == "" || strings.TrimSpace(e.projectRoot) == "" {
		return
	}
	_ = policyinterrupt.AppendAck(e.projectRoot, policyinterrupt.AckRecord{
		Profile:        "policy",
		DedupeKey:      DisparityDedupeKey(itemID),
		AckedBy:        "account:system",
		Reason:         "qa_auditor_pass",
		SteeringAction: "auto_ack_on_audit_pass",
	})
}
