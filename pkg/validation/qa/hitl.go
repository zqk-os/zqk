package qa

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/policyinterrupt"
)

// InterruptEmitter handles the emission of policy interrupts for QA disparities.
type InterruptEmitter struct {
	projectRoot string
}

func NewInterruptEmitter(projectRoot string) *InterruptEmitter {
	return &InterruptEmitter{projectRoot: projectRoot}
}

// ProjectRoot returns the configured project root directory.
func (e *InterruptEmitter) ProjectRoot() string {
	if e == nil {
		return ""
	}
	return e.projectRoot
}

// EmitDisparityInterrupt triggers a high-severity policy interrupt for a structural disparity.
func (e *InterruptEmitter) EmitDisparityInterrupt(ctx context.Context, itemID string, reason string) error {
	record := policyinterrupt.InterruptRecord{
		Ts:          time.Now(),
		Profile:     "policy",
		Severity:    policyinterrupt.SeverityCritical,
		AckRequired: true,
		DedupeKey:   DisparityDedupeKey(itemID),
		Message:     fmt.Sprintf("QA Disparity Detected for %s: %s", itemID, reason),
	}
	return policyinterrupt.AppendInterrupt(e.projectRoot, record)
}

// DisparityDedupeKey is the policy-interrupt dedupe key for a QA disparity on itemID.
func DisparityDedupeKey(itemID string) string {
	return fmt.Sprintf("qa-disparity-%s", itemID)
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
		AckedBy:        objects.DefaultSystemAccountID,
		Reason:         "qa_auditor_pass",
		SteeringAction: "auto_ack_on_audit_pass",
	})
}
