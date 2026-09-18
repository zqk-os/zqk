package interactionpolicy

import (
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/pkg/objects"
)

// StratplanAmbient is the kernel facts that make a planned==0 hunger
// prompt coherent (lead priority_plan, counts, alignment, next PRIs).
type StratplanAmbient struct {
	LeadID          string
	LeadTitle       string
	LeadStatus      string
	Counts          map[string]int
	AlignOK         bool
	AlignScore      float64
	GoalGaps        int
	AlignMeasuredAt string
	AlignAge        string
	AlignFresh      bool
	DraftPlaneTotal int
	NextPlans       []string
}

const MaxAmbientPlans = 3

// AlignFreshMax is how long TPM may trust align-latest.json before the gland
// asks for a refresh. Fresh cache must not re-issue the align CommandHint.
// TRACK: BLI-1787035087372193000-c022117d
const AlignFreshMax = 30 * time.Minute

const (
	HintAlignRefresh  = "zqk system align --format json -o .zqk/state/ambient/align-latest.json"
	HintDraftClassify = "zqk object draft sweep --dry-run --all"
	HintHourglass     = "zqk feed steer --to-agent-id <seat> --await-peer-ack"
	HintPushAhead     = "git status -sb && git push"
	// HintTracePipeline is the TPM objectify path. Hand-minting 1:1 REQ→CRIT
	// skips test cases and the multi-criteria base structure.
	// TRACK: POL-AGENT-TPM-TRACE-PIPELINE-001
	HintTracePipeline = "zqk new object requirement --title \"...\" && zqk workflow gen-trace-pipeline <REQ-id>"
)

// HintOrchestratePlan is the machine first line. Empty seat stays a placeholder
// so stop-hunger can still name the protocol when persona→seat is unknown.
func HintOrchestratePlan(seat, planID string) string {
	planID = strings.TrimSpace(planID)
	if planID == "" {
		return HintHourglass
	}
	if strings.TrimSpace(seat) == "" {
		seat = "<seat>"
	}
	return fmt.Sprintf("zqk feed steer --to-agent-id %s --await-peer-ack --message %q", seat, "ORCHESTRATE_PLAN "+planID)
}

// HintObjectGet is the situational next when a grooming/unlocked column is named.
func HintObjectGet(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	return "zqk object get " + id
}

// FirstAmbientPlanID parses "PRI-X (grooming)" labels from NextPlans.
func FirstAmbientPlanID(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	fields := strings.Fields(strings.TrimSpace(labels[0]))
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// FinalizeAlignFreshness stamps AlignFresh/AlignAge from MeasuredAt.
// Missing timestamp with AlignOK is treated as fresh so unit fixtures without
// mtime still compile a next-column hint instead of reminting align.
func FinalizeAlignFreshness(a *StratplanAmbient, now time.Time) {
	if a == nil {
		return
	}
	if !a.AlignOK {
		a.AlignFresh = false
		a.AlignAge = ""
		return
	}
	if strings.TrimSpace(a.AlignMeasuredAt) == "" {
		a.AlignFresh = true
		a.AlignAge = "unknown"
		return
	}
	t, err := time.Parse(time.RFC3339, a.AlignMeasuredAt)
	if err != nil {
		a.AlignFresh = false
		a.AlignAge = "unparsed"
		return
	}
	age := now.Sub(t)
	if age < 0 {
		age = 0
	}
	a.AlignAge = formatAlignAge(age)
	a.AlignFresh = age <= AlignFreshMax
}

func formatAlignAge(d time.Duration) string {
	if d < time.Minute {
		return "0m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

// CompileStratplanCommandHint is the stop-hook Next: line. Catalog default
// (always align) is what made GROOM-AHEAD feel ignored — TPM already had a
// fresh cache and a grooming next column, but hunger kept saying "run align".
func CompileStratplanCommandHint(a StratplanAmbient) string {
	if !a.AlignOK || !a.AlignFresh {
		return HintAlignRefresh
	}
	if id := NextUnshapedPlanID(a.NextPlans); id != "" {
		return HintObjectGet(id)
	}
	// Next grooming PRI already shaped: do not re-get it. If the lead is
	// executing, hourglass that work. Draft classify only when nothing is in flight.
	if leadExecuting(a) {
		return HintHourglass
	}
	if a.DraftPlaneTotal > 0 {
		return HintDraftClassify
	}
	return HintHourglass
}

func leadExecuting(a StratplanAmbient) bool {
	if strings.EqualFold(strings.TrimSpace(a.LeadStatus), objects.ObjectStatusInProgress) {
		return true
	}
	if a.Counts != nil && a.Counts["in_progress"] > 0 {
		return true
	}
	return false
}

// NextUnshapedPlanID skips labels marked (grooming,shaped) so hunger does not
// remint object get after TPM already filled description + persona_refs.
func NextUnshapedPlanID(labels []string) string {
	for _, lab := range labels {
		if strings.Contains(lab, ",shaped") {
			continue
		}
		if id := FirstAmbientPlanID([]string{lab}); id != "" {
			return id
		}
	}
	return ""
}

// AmbientPlanRef is a sibling priority_plan (Gantt column) for next_priority_plans ranking.
type AmbientPlanRef struct {
	ID     string
	Status string
	Shaped bool
}

// RankNextPlanLabels puts grooming/prioritizing (next unlocked) ahead of
// other sibling priority_plans so ambient names the shape target, not another lock.
func RankNextPlanLabels(leadID string, plans []AmbientPlanRef) []string {
	var grooming, others []string
	lead := strings.TrimSpace(leadID)
	for _, p := range plans {
		if p.ID == "" || p.ID == lead {
			continue
		}
		label := p.ID + " (" + p.Status
		if p.Shaped {
			switch strings.ToLower(strings.TrimSpace(p.Status)) {
			case "grooming", "prioritizing", "planning":
				label += ",shaped"
			}
		}
		label += ")"
		switch strings.ToLower(strings.TrimSpace(p.Status)) {
		case "grooming", "prioritizing", "planning":
			grooming = append(grooming, label)
		default:
			others = append(others, label)
		}
	}
	out := append(grooming, others...)
	if len(out) > MaxAmbientPlans {
		out = out[:MaxAmbientPlans]
	}
	return out
}

// AppendStratplanAmbient suffixes compiled hunger with ambient facts so the
// TPM prompt is forward planning, not an empty "don't do X" reflex.
func AppendStratplanAmbient(drive string, a StratplanAmbient) string {
	drive = strings.TrimSpace(drive)
	sig := formatStratplanAmbient(a)
	if sig == "" {
		return drive
	}
	if drive == "" {
		return sig
	}
	return drive + " Ambient: " + sig
}

func formatStratplanAmbient(a StratplanAmbient) string {
	var b strings.Builder
	if a.LeadID != "" {
		st := a.LeadStatus
		if st == "" {
			st = "?"
		}
		fmt.Fprintf(&b, "lead_priority_plan %s [%s]", a.LeadID, st)
		if t := strings.TrimSpace(a.LeadTitle); t != "" {
			fmt.Fprintf(&b, " %s", clipRunes(t, 48))
		}
	}
	if a.Counts != nil {
		fmt.Fprintf(&b, "; counts planned=%d in_progress=%d exploring=%d validated=%d complete=%d",
			a.Counts["planned"], a.Counts["in_progress"], a.Counts["exploring"], a.Counts["validated"], a.Counts["complete"])
	}
	if a.AlignOK {
		fresh := "fresh"
		if !a.AlignFresh {
			fresh = "STALE"
		}
		age := a.AlignAge
		if age == "" {
			age = "?"
		}
		fmt.Fprintf(&b, "; align score=%.1f %s age=%s goal_gaps=%d", a.AlignScore, fresh, age, a.GoalGaps)
	} else {
		b.WriteString("; align cache missing — persist " + HintAlignRefresh)
	}
	if a.DraftPlaneTotal > 0 {
		fmt.Fprintf(&b, "; draft_plane=%d", a.DraftPlaneTotal)
	}
	if len(a.NextPlans) > 0 {
		n := a.NextPlans
		if len(n) > MaxAmbientPlans {
			n = n[:MaxAmbientPlans]
		}
		fmt.Fprintf(&b, "; next_priority_plans %s", strings.Join(n, ", "))
	} else if a.LeadID != "" {
		b.WriteString("; next_priority_plans none")
	}
	return strings.TrimSpace(b.String())
}
