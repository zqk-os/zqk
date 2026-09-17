package workflow

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lanceman/zqk/pkg/agentfeed"
	"github.com/lanceman/zqk/pkg/interactionpolicy"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/workflow/whatsnext"
	"github.com/lanceman/zqk/pkg/zqkenv"
)

func TestCompileWhatsNextDrive_emptyColumnIsStratplan(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{BacklogCountsByStatus: map[string]int{"planned": 0}}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil {
		t.Fatal("expected hunger when planned=0")
	}
	if out.GuidingStep.Event != interactionpolicy.EventStratplanAhead {
		t.Fatalf("event=%q", out.GuidingStep.Event)
	}
	if out.GuidingStep.PolicyID != interactionpolicy.PolicyTPMGroomAhead {
		t.Fatalf("policy=%q", out.GuidingStep.PolicyID)
	}
	if strings.Contains(out.GuidingStep.GuidingStep, "Groom BLIs on the seated PRI") {
		t.Fatalf("must not tell TPM to restuff: %q", out.GuidingStep.GuidingStep)
	}
	if out.GuidingStep.CommandHint != interactionpolicy.HintAlignRefresh {
		t.Fatalf("missing align cache must hint persist, got %q", out.GuidingStep.CommandHint)
	}
}

func TestCompileWhatsNextDrive_executingColumnIsStratplanWithAmbient(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{
		PriorityPlan:          &whatsNextPriorityPlan{ID: "PRI-LEAD", Title: "Ghost RCA", Status: objects.ObjectStatusInProgress},
		BacklogCountsByStatus: map[string]int{"planned": 0, "in_progress": 1},
		ActivePlans: []whatsNextPriorityPlan{
			{ID: "PRI-LEAD", Status: objects.ObjectStatusInProgress},
			{ID: "PRI-NEXT", Status: objects.ObjectStatusGrooming},
		},
		KernelAmbience: &whatsnext.KernelAmbience{
			DraftPlaneTotal: 4,
			StrategicAlignment: &whatsnext.StrategicAlignmentSnapshot{
				Available:         true,
				AlignmentScore:    92.9,
				ItemsWithoutGoals: 62,
				MeasuredAt:        time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil || out.GuidingStep.Event != interactionpolicy.EventStratplanAhead {
		t.Fatalf("step=%+v", out.GuidingStep)
	}
	gs := out.GuidingStep.GuidingStep
	for _, part := range []string{"Do not break the seal", "Ambient:", "lead_priority_plan PRI-LEAD", "goal_gaps=62", "next_priority_plans", "PRI-NEXT", "fresh", "draft_plane=4"} {
		if !strings.Contains(gs, part) {
			t.Fatalf("missing %q in %q", part, gs)
		}
	}
	if out.GuidingStep.CommandHint != interactionpolicy.HintObjectGet("PRI-NEXT") {
		t.Fatalf("fresh align must not remint align, hint=%q", out.GuidingStep.CommandHint)
	}
}

func TestCompileWhatsNextDrive_shapedNextHourglassesLead(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{
		PriorityPlan:          &whatsNextPriorityPlan{ID: "PRI-LEAD", Title: "S18", Status: objects.ObjectStatusInProgress},
		BacklogCountsByStatus: map[string]int{"planned": 0, "in_progress": 1},
		ActivePlans: []whatsNextPriorityPlan{
			{ID: "PRI-LEAD", Status: objects.ObjectStatusInProgress},
			{ID: "PRI-NEXT", Status: objects.ObjectStatusGrooming, Shaped: true},
		},
		KernelAmbience: &whatsnext.KernelAmbience{
			DraftPlaneTotal: 2,
			StrategicAlignment: &whatsnext.StrategicAlignmentSnapshot{
				Available:      true,
				AlignmentScore: 96,
				MeasuredAt:     time.Now().UTC().Format(time.RFC3339),
			},
		},
	}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil {
		t.Fatal("expected step")
	}
	if !strings.Contains(out.GuidingStep.GuidingStep, "grooming,shaped") {
		t.Fatalf("ambient should mark shaped: %q", out.GuidingStep.GuidingStep)
	}
	if out.GuidingStep.CommandHint != interactionpolicy.HintHourglass {
		t.Fatalf("shaped next + executing lead: got hint %q", out.GuidingStep.CommandHint)
	}
}

func TestCompileWhatsNextDrive_unsealedExploringIsGroom(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{
		PriorityPlan:          &whatsNextPriorityPlan{ID: "PRI-INTAKE", Status: objects.ObjectStatusActive},
		BacklogCountsByStatus: map[string]int{"planned": 0, "exploring": 2},
	}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil || out.GuidingStep.Event != interactionpolicy.EventShovelReadyEmpty {
		t.Fatalf("step=%+v", out.GuidingStep)
	}
}

func TestCompileWhatsNextDrive_idleFullColumnIsNotGroomAhead(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{BacklogCountsByStatus: map[string]int{"planned": 3}}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil {
		t.Fatal("expected idle drive when planned>0 and no hourglass outbox")
	}
	if out.GuidingStep.Event != interactionpolicy.EventIdle {
		t.Fatalf("event=%q", out.GuidingStep.Event)
	}
	if out.GuidingStep.PolicyID != interactionpolicy.PolicyTPMProcessAdmin {
		t.Fatalf("policy=%q (idle must not overlay GROOM-AHEAD empty-column text)", out.GuidingStep.PolicyID)
	}
	if strings.Contains(out.GuidingStep.GuidingStep, "shovel-ready is empty") {
		t.Fatalf("false empty-column hunger: %q", out.GuidingStep.GuidingStep)
	}
	if out.GuidingStep.CommandHint != "" {
		t.Fatalf("idle is seat-worker duty, hint must stay empty, got %q", out.GuidingStep.CommandHint)
	}
	if out.FillItem != nil {
		t.Fatalf("no ambience: fill must be nil, got %+v", out.FillItem)
	}
}

func TestCompileWhatsNextDrive_idleFillSetsHint(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{
		BacklogCountsByStatus: map[string]int{"planned": 3},
		KernelAmbience:        &whatsnext.KernelAmbience{GhostRefCount: 2, Available: true},
	}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil || out.GuidingStep.Event != interactionpolicy.EventIdle {
		t.Fatalf("step=%+v", out.GuidingStep)
	}
	if out.FillItem == nil || out.FillItem.Kind != whatsnext.FillKindGhostRef {
		t.Fatalf("fill=%+v", out.FillItem)
	}
	if out.GuidingStep.CommandHint != whatsnext.FillCmdAutofixDangling {
		t.Fatalf("hint=%q", out.GuidingStep.CommandHint)
	}
	if !strings.Contains(out.GuidingStep.GuidingStep, "GhostRefs") {
		t.Fatalf("fill reason missing: %q", out.GuidingStep.GuidingStep)
	}
}

func TestCompileWhatsNextDrive_pushAheadBeatsIdle(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{
		BacklogCountsByStatus: map[string]int{"planned": 3},
		KernelAmbience:        &whatsnext.KernelAmbience{BranchAhead: 2},
	}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil || out.GuidingStep.Event != interactionpolicy.EventPushAhead {
		t.Fatalf("step=%+v", out.GuidingStep)
	}
	if out.GuidingStep.CommandHint != interactionpolicy.HintPushAhead {
		t.Fatalf("hint=%q", out.GuidingStep.CommandHint)
	}
}

func TestCompileWhatsNextDrive_hourglassWaitingIsIdleNotSilent(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{
		BacklogCountsByStatus: map[string]int{"planned": 3},
		Correspondence: &whatsNextCorrespondence{
			OutboxAwaitingPeerAck: []agentfeed.CorrespondenceItem{{EventID: "AFE-WAIT"}},
		},
	}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil || out.GuidingStep.Event != interactionpolicy.EventIdle {
		t.Fatalf("hourglass must not silence hunger: %+v", out.GuidingStep)
	}
	if !strings.Contains(out.GuidingStep.GuidingStep, "do not remint") {
		t.Fatalf("keep-working cue missing: %q", out.GuidingStep.GuidingStep)
	}
	if out.GuidingStep.CommandHint != "" {
		t.Fatalf("hourglass must not remint steer, hint=%q", out.GuidingStep.CommandHint)
	}
}

func TestCompileWhatsNextDrive_hourglassFillKeepsWorking(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{
		BacklogCountsByStatus: map[string]int{"planned": 3},
		Correspondence: &whatsNextCorrespondence{
			OutboxAwaitingPeerAck: []agentfeed.CorrespondenceItem{{EventID: "AFE-WAIT"}},
		},
		KernelAmbience: &whatsnext.KernelAmbience{GhostRefCount: 1, Available: true},
	}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil || out.GuidingStep.Event != interactionpolicy.EventIdle {
		t.Fatalf("hourglass must not silence hunger: %+v", out.GuidingStep)
	}
	if out.GuidingStep.CommandHint != whatsnext.FillCmdAutofixDangling {
		t.Fatalf("hourglass fill hint=%q", out.GuidingStep.CommandHint)
	}
}

func TestCompileWhatsNextDrive_inboxBeatsPushAhead(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{
		BacklogCountsByStatus: map[string]int{"planned": 0},
		KernelAmbience:        &whatsnext.KernelAmbience{BranchAhead: 3},
		Correspondence: &whatsNextCorrespondence{
			InboxUnacked: []agentfeed.CorrespondenceItem{{EventID: "AFE-1"}},
		},
	}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil || out.GuidingStep.Event != interactionpolicy.EventInboxUnacked {
		t.Fatalf("inbox must preempt push-ahead: %+v", out.GuidingStep)
	}
}

func TestCompileWhatsNextDrive_inboxIsSwarmHunger(t *testing.T) {
	t.Setenv(zqkenv.Persona().Name(), "")
	out := whatsNextOut{
		BacklogCountsByStatus: map[string]int{"planned": 0},
		Correspondence: &whatsNextCorrespondence{
			InboxUnacked: []agentfeed.CorrespondenceItem{{EventID: "AFE-1"}},
		},
	}
	compileWhatsNextDrive(&out, context.Background(), nil, "", nil)
	if out.GuidingStep == nil {
		t.Fatal("inbox unacked must compile hunger so Cursor stop-hook followup fires")
	}
	if out.GuidingStep.Event != interactionpolicy.EventInboxUnacked {
		t.Fatalf("event=%q", out.GuidingStep.Event)
	}
	if out.GuidingStep.PolicyID != interactionpolicy.PolicyTPMProcessAdmin {
		t.Fatalf("policy=%q", out.GuidingStep.PolicyID)
	}
}

func TestPriorityPlanLooksShaped_UsesInheritedTitleAndWorkstream(t *testing.T) {
	t.Parallel()
	if priorityPlanLooksShaped(map[string]any{
		objects.FieldKeyDescription: "notes",
		objects.FieldKeyPersonaRefs: []string{"PER-ORCH-ALPHA"},
	}) {
		t.Fatal("personas without a workstream lane must not count as shaped")
	}
	if !priorityPlanLooksShaped(map[string]any{
		objects.FieldKeyTitle:          "CEF Round 23 — package directory names",
		objects.FieldKeyWorkstreamRefs: []string{"WS-CEF-ARCHITECTURE"},
		objects.FieldKeyPersonaRefs:    []string{"PER-ORCH-ALPHA"},
	}) {
		t.Fatal("inherited title plus workstream plus personas must be shaped")
	}
}
