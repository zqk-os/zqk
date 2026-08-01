package object

import (
	"github.com/lanceman/zqk/internal/cli"
	"github.com/lanceman/zqk/pkg/nildecode"
	"github.com/lanceman/zqk/pkg/objects"
)

func applyMilestoneCriteriaOverlay(proc *cli.Processor, obj map[string]any) {
	kind, _ := obj[objects.FieldKeyKind].(string)
	if kind != objects.KindMilestone {
		return
	}

	// Prefer using `resolved_criteria_refs` emitted by the generic reference resolver overlay.
	// That ensures we do not parse `[x]` markers and we keep resolution consistent across views.
	resolvedKey := "resolved_" + objects.FieldKeyCriteriaRefs
	resolvedAny, hasResolved := obj[resolvedKey]

	var statuses []string
	switch v := resolvedAny.(type) {
	case []map[string]any:
		statuses = extractStatusesFromResolved(v)
	case []any:
		statuses = extractStatusesFromResolvedAny(v)
	default:
		_ = hasResolved // keep fallback path reachable if type mismatches
	}

	// Fallback: if the generic overlay wasn't applied, compute completion from raw `criteria_refs`.
	// This is best-effort and should remain bounded.
	if len(statuses) == 0 {
		critRefsAny, ok := obj[objects.FieldKeyCriteriaRefs]
		if !ok {
			return
		}
		critRefsAny, ok = nildecode.DecodeNonNilPayload[any](critRefsAny)
		if !ok {
			return
		}
		critRefs := extractStatusesFromRawRefs(proc, obj, critRefsAny)
		statuses = critRefs
	}

	n := len(statuses)
	if n == 0 {
		return
	}

	completeCount := 0
	for _, s := range statuses {
		if milestoneCriteriaStatusIsComplete(s) {
			completeCount++
		}
	}

	percent := (float64(completeCount) / float64(n)) * 100
	obj["milestone_complete_via_criteria_refs"] = (completeCount == n)
	obj["milestone_percent_complete_via_criteria_refs"] = percent
}

func milestoneCriteriaStatusIsComplete(status string) bool {
	switch status {
	case objects.ObjectStatusCompleted, objects.ObjectStatusValidated:
		return true
	// criteria, milestone, goal, backlog_item, and several other kinds use lifecycle
	// terminal "complete" (see docs/process/_internal/lifecycles/*), distinct from
	// ObjectStatusCompleted ("completed") used by metrics and similar kinds.
	case "complete":
		return true
	default:
		return false
	}
}

func extractStatusesFromResolved(resolved []map[string]any) []string {
	out := make([]string, 0, len(resolved))
	for _, entry := range resolved {
		status, _ := entry[objects.FieldKeyStatus].(string)
		out = append(out, status)
	}
	return out
}

func extractStatusesFromResolvedAny(resolved []any) []string {
	out := make([]string, 0, len(resolved))
	for _, item := range resolved {
		if m, ok := item.(map[string]any); ok {
			status, _ := m[objects.FieldKeyStatus].(string)
			out = append(out, status)
		}
	}
	return out
}

// extractStatusesFromRawRefs is a bounded fallback for when the generic overlay didn't run.
// It reads each referenced criteria object and extracts its durable `status`.
func extractStatusesFromRawRefs(proc *cli.Processor, obj map[string]any, critRefsAny any) []string {
	critRefs := extractReferenceIDs(critRefsAny)
	if len(critRefs) == 0 {
		return nil
	}

	// Best-effort bounded fallback (avoid N+1 fan-out).
	const maxReads = 50
	if len(critRefs) > maxReads {
		critRefs = critRefs[:maxReads]
	}

	ctx := proc.OperationContext()
	secCtx := proc.SecurityContext()
	storage := proc.Storage()

	out := make([]string, 0, len(critRefs))

	res, err := storage.BulkGet(ctx, secCtx, critRefs)
	if err != nil || res == nil {
		for range critRefs {
			out = append(out, "")
		}
		return out
	}

	objMap := make(map[string]map[string]any, len(res.Results))
	for _, critObj := range res.Results {
		if id, ok := critObj[objects.FieldKeyID].(string); ok {
			objMap[id] = critObj
		}
	}

	for _, critID := range critRefs {
		var status string
		if critObj, ok := objMap[critID]; ok {
			status, _ = critObj[objects.FieldKeyStatus].(string)
		}
		out = append(out, status)
	}

	return out
}
