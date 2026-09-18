package objects

import "strings"

// Whats-next ranking helpers keyed by lifecycle status.role.
// TRACK: [REDACTED-ID]

const (
	whatsNextBonusExecutionLocked = 500_000
	whatsNextBonusShovelReady     = 400_000
	whatsNextBonusHalted          = 200_000
	whatsNextBonusPrioritizing    = 50_000 // grooming role, mid ladder
	whatsNextUnsetOrderPenalty    = 1_000_000
	whatsNextExclusivePersona     = 50_000
	whatsNextSharedPersonaPenalty = 100_000
)

// PlanWhatsNextCandidateStatuses is the status list both CLI whats-next and
// pkg/workflow/whatsnext walk when collecting Gantt columns. Includes grooming
// so next-unlocked (not yet active) plans appear in ambient next_columns.
// TRACK: BLI-1787035087372193000-c022117d
func PlanWhatsNextCandidateStatuses() []string {
	return []string{
		ObjectStatusInProgress,
		ObjectStatusPaused,
		ObjectStatusActive,
		ObjectStatusGrooming,
		ObjectStatusPrioritizing,
	}
}

// PlanStatusEligibleForWhatsNext reports whether a plan status may be a whats-next candidate.
func PlanStatusEligibleForWhatsNext(kind, status string) bool {
	role := statusRoleOrFallback(kind, status)
	switch role {
	case LifecycleRoleExecutionLocked, LifecycleRoleShovelReady, LifecycleRoleHalted, LifecycleRoleGrooming:
		return true
	default:
		return false
	}
}

// PlanHasWrittenIdentity uses inherited title (base_object, required at create)
// or specialized description. Do not require both — description is optional prose
// on top of title. TRACK: BLI-CEF-R26-REMAINING-KINDS-001
func PlanHasWrittenIdentity(obj map[string]any) bool {
	if obj == nil {
		return false
	}
	title, _ := obj[FieldKeyTitle].(string)
	if strings.TrimSpace(title) != "" {
		return true
	}
	desc, _ := obj[FieldKeyDescription].(string)
	return strings.TrimSpace(desc) != ""
}

// PlanHasWorkstreamLane is the spec-backed Gantt bind: workstream_refs (plural)
// or the leftover singular workstream_ref alias. Empty slices fail.
func PlanHasWorkstreamLane(obj map[string]any) bool {
	return len(WorkstreamLaneIDs(obj)) > 0
}

// PlanStatusExecutionFacing is true for shovel-ready / execution-locked / halted plans
// (paused counts as halted — still an execution lane, not a grooming mega).
func PlanStatusExecutionFacing(kind, status string) bool {
	role := statusRoleOrFallback(kind, status)
	switch role {
	case LifecycleRoleExecutionLocked, LifecycleRoleShovelReady, LifecycleRoleHalted:
		return true
	default:
		return false
	}
}

// PlanWhatsNextStatusBonus ranks execution_locked ≫ shovel_ready ≫ halted ≫ grooming.
func PlanWhatsNextStatusBonus(kind, status string) int {
	role := statusRoleOrFallback(kind, status)
	switch role {
	case LifecycleRoleExecutionLocked:
		return whatsNextBonusExecutionLocked
	case LifecycleRoleShovelReady:
		return whatsNextBonusShovelReady
	case LifecycleRoleHalted:
		return whatsNextBonusHalted
	case LifecycleRoleGrooming:
		if strings.EqualFold(strings.TrimSpace(status), "prioritizing") {
			return whatsNextBonusPrioritizing
		}
		return 0
	default:
		return 0
	}
}

// PlanActiveOrderPenalty: lower active_order ranks higher. Execution-locked plans
// (in_progress) always penalty 0 — ≡ active @ order 0 even when shockwave cleared the field.
func PlanActiveOrderPenalty(kind string, obj map[string]any) int {
	if obj == nil {
		return whatsNextUnsetOrderPenalty
	}
	st, _ := obj[FieldKeyStatus].(string)
	if statusRoleOrFallback(kind, st) == LifecycleRoleExecutionLocked {
		return 0
	}
	v, ok := obj[FieldKeyActiveOrder]
	if !ok || v == nil {
		return whatsNextUnsetOrderPenalty
	}
	switch n := v.(type) {
	case int:
		if n < 0 {
			return whatsNextUnsetOrderPenalty
		}
		return n
	case int64:
		if n < 0 {
			return whatsNextUnsetOrderPenalty
		}
		return int(n)
	case float64:
		if n < 0 {
			return whatsNextUnsetOrderPenalty
		}
		return int(n)
	default:
		return whatsNextUnsetOrderPenalty
	}
}

// BacklogCountsAsOpenWork is true when a linked BLI should count toward plan "has work".
// Terminal and halted (error) do not; realign/shovel_ready/execution_locked do.
func BacklogCountsAsOpenWork(status string) bool {
	role := statusRoleOrFallback(KindBacklogItem, status)
	switch role {
	case LifecycleRoleTerminal, LifecycleRoleHalted:
		return false
	default:
		if role != "" {
			return true
		}
		// Fallback when role unavailable (tests without lifecycle tree).
		switch strings.ToLower(strings.TrimSpace(status)) {
		case ObjectStatusComplete, ObjectStatusArchived, ObjectStatusCancelled,
			ObjectStatusImplemented, ObjectStatusRejected, ObjectStatusError:
			return false
		default:
			return true
		}
	}
}

// BacklogCountsAsExecutionFuel is shovel-ready / in-flight / blocked only.
// Parked realign (deferred, roadmap, validated, exploring) must not win a seated
// whats-next column over planned work. TRACK: BLI-1787035087372193000-c022117d
func BacklogCountsAsExecutionFuel(status string) bool {
	role := statusRoleOrFallback(KindBacklogItem, status)
	switch role {
	case LifecycleRoleShovelReady, LifecycleRoleExecutionLocked:
		return true
	case LifecycleRoleHalted:
		// blocked stays visible so a primary can unblock; paused parking does not.
		st := strings.ToLower(strings.TrimSpace(status))
		return st == ObjectStatusBlocked || st == "blocked"
	default:
		return false
	}
}

// PlanSeatedPersonaBonus ranks a persona-exclusive column above a shared umbrella
// (e.g. hop-gate ALPHA-only vs R27 ALPHA+BETA) when whats-next is seat-scoped.
func PlanSeatedPersonaBonus(personaIDs []string, obj map[string]any) int {
	if len(personaIDs) == 0 || obj == nil {
		return 0
	}
	n := personaRefCount(obj)
	switch {
	case n <= 1:
		return whatsNextExclusivePersona
	default:
		return -whatsNextSharedPersonaPenalty
	}
}

func personaRefCount(obj map[string]any) int {
	refsAny := obj[FieldKeyPersonaRefs]
	switch refs := refsAny.(type) {
	case []any:
		n := 0
		for _, r := range refs {
			if s, ok := r.(string); ok && strings.TrimSpace(s) != "" {
				n++
			}
		}
		return n
	case []string:
		n := 0
		for _, s := range refs {
			if strings.TrimSpace(s) != "" {
				n++
			}
		}
		return n
	default:
		return 0
	}
}

func statusRoleOrFallback(kind, status string) string {
	role := GetGlobalStatusChecker().Role(kind, status)
	if role != "" {
		return role
	}
	// Minimal string fallback for kinds without role YAML / offline tests.
	st := strings.ToLower(strings.TrimSpace(status))
	switch st {
	case ObjectStatusInProgress:
		return LifecycleRoleExecutionLocked
	case ObjectStatusActive, ObjectStatusPlanned:
		return LifecycleRoleShovelReady
	case ObjectStatusPaused, ObjectStatusBlocked, ObjectStatusError:
		return LifecycleRoleHalted
	case "grooming", "planning", "prioritizing":
		return LifecycleRoleGrooming
	case ObjectStatusComplete, ObjectStatusArchived, ObjectStatusCancelled, ObjectStatusRejected, ObjectStatusImplemented:
		return LifecycleRoleTerminal
	default:
		return ""
	}
}
