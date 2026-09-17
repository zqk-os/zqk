package primaryorch

import (
	"context"
	"fmt"
	"strings"

	"github.com/lanceman/zqk/pkg/objects"
)

// PersonaTPM is the role token used in wake payloads for technical program manager
// attentiveness (not a vendor agent_id).
const PersonaTPM = "tpm"

// MaybeWakeOnPlanBacklogError wakes the primary orchestrator when a backlog_item
// on a priority plan transitions into status=error.
// Best-effort: never fails the caller's status transition.
func MaybeWakeOnPlanBacklogError(ctx context.Context, projectRoot string, kind, fromStatus, toStatus string, obj map[string]any) {
	if kind != objects.KindBacklogItem {
		return
	}
	if toStatus != objects.ObjectStatusError || fromStatus == objects.ObjectStatusError {
		return
	}
	planID, _ := obj[objects.FieldKeyPriorityPlanRef].(string)
	if strings.TrimSpace(planID) == "" {
		return
	}
	id, _ := obj[objects.FieldKeyID].(string)
	title, _ := obj[objects.FieldKeyTitle].(string)
	msg := fmt.Sprintf("Plan BLI entered error — investigate (promote-first). bli=%s title=%s", id, title)
	_, _ = WakePrimary(ctx, projectRoot, WakeRequest{
		TaskID:  id,
		Persona: PersonaTPM,
		PlanID:  planID,
		Message: msg,
		TopBLIs: []BacklogBrief{{ID: id, Title: title, Tier: objects.StringField(obj, objects.FieldKeyPriorityTier)}},
	})
}
