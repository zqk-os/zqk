package agentprompt

import (
	"context"
	"strings"
	"testing"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/objects"
)

func TestFormatTaskEnvelope_refsNotBodies(t *testing.T) {
	opts := TaskPromptOptions{
		PlanTitle:   "Plan A",
		PlanID:      "PRI-1",
		TaskTitle:   "Do the thing",
		TargetAgent: "PER-worker",
		PersonaID:   "PER-worker",
		Capability:  "code",
	}
	out := FormatTaskEnvelope(opts, []string{"POL-ONBOARD-001", "POL-EXTRA"}, []string{"ASK-ORCH"})
	if !IsTaskEnvelope(out) {
		t.Fatal("expected envelope marker")
	}
	if !strings.Contains(out, "PRI-1") || !strings.Contains(out, "Do the thing") {
		t.Fatalf("missing dynamic facts:\n%s", out)
	}
	if !strings.Contains(out, "ASK-ORCH") || !strings.Contains(out, "POL-EXTRA") {
		t.Fatalf("missing refs:\n%s", out)
	}
	for _, banned := range []string{
		"Top 30 Highest Complexity",
		"#### Mandates",
		"You must write the test case FIRST",
		"You must NEVER pause or idle",
	} {
		if strings.Contains(out, banned) {
			t.Fatalf("persist envelope must not contain %q:\n%s", banned, out)
		}
	}
}

func TestBuildTaskEnvelope_policyAndSkillRefs(t *testing.T) {
	sp := &testMockSP{
		objects: map[string]map[string]any{
			"ASK-BOOT": {
				objects.FieldKeyKind:                objects.KindAgentSkill,
				objects.FieldKeyID:                  "ASK-BOOT",
				objects.FieldKeyTitle:               "Boot skill",
				objects.FieldKeyInstructionsSummary: "[orchestration-boot] required",
				objects.FieldKeyInstructions:        strings.Repeat("NEVER copy this body into the ATK. ", 40),
			},
			"PER-123": {
				objects.FieldKeyKind:                  objects.KindPersona,
				objects.FieldKeyID:                    "PER-123",
				objects.FieldKeyInteractionPolicyRefs: []any{"POL-AGENT-001"},
				objects.FieldKeyRelatedObjectRefs:     []any{"ASK-BOOT"},
			},
			"POL-AGENT-001": {
				objects.FieldKeyKind:        objects.KindPolicy,
				objects.FieldKeyID:          "POL-AGENT-001",
				objects.FieldKeyTitle:       "Agent policy",
				objects.FieldKeyDescription: strings.Repeat("policy body that must not persist. ", 20),
			},
		},
	}
	opts := TaskPromptOptions{
		PlanTitle:   "P",
		PlanID:      "PRI-x",
		TaskTitle:   "T",
		TargetAgent: "PER-123",
		PersonaID:   "PER-123",
	}
	env, err := BuildTaskEnvelope(context.Background(), sp, pkgctx.NewSystemSecurityContext(), "", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !IsTaskEnvelope(env.Description) {
		t.Fatal("expected envelope marker")
	}
	if !containsID(env.SkillRefs, "ASK-BOOT") {
		t.Fatalf("expected ASK-BOOT in skill refs, got %#v", env.SkillRefs)
	}
	if !containsID(env.PolicyRefs, "POL-AGENT-001") {
		t.Fatalf("expected persona policy in policy refs, got %#v", env.PolicyRefs)
	}
	if strings.Contains(env.Description, "NEVER copy this body") {
		t.Fatal("skill body leaked into persist envelope")
	}
	if strings.Contains(env.Description, "policy body that must not persist") {
		t.Fatal("policy body leaked into persist envelope")
	}
}

func TestBuildTaskPrompt_refsNotBodies(t *testing.T) {
	sp := &testMockSP{
		objects: map[string]map[string]any{
			"ASK-BOOT": {
				objects.FieldKeyKind:                objects.KindAgentSkill,
				objects.FieldKeyID:                  "ASK-BOOT",
				objects.FieldKeyTitle:               "Boot skill",
				objects.FieldKeyInstructionsSummary: "[orchestration-boot] required",
				objects.FieldKeyInstructions:        "NEVER copy this body into the prompt.",
			},
		},
	}
	out, err := BuildTaskPrompt(context.Background(), sp, pkgctx.NewSystemSecurityContext(), "", TaskPromptOptions{
		PlanTitle: "Test Plan",
		TaskTitle: "Work",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Test Plan") {
		t.Fatalf("expected plan title:\n%s", out)
	}
	if strings.Contains(out, "NEVER copy this body") {
		t.Fatal("skill body inlined into execute prompt")
	}
	if strings.Contains(out, "Top 30 Highest Complexity") {
		t.Fatal("AST census inlined into execute prompt")
	}
	if !strings.Contains(out, StandingPolicyTDD) && !strings.Contains(out, "zqk object get") {
		t.Fatal("expected standing refs in execute prompt")
	}
}

func TestBuildTaskPrompt_persistLayerIsEnvelope(t *testing.T) {
	sp := &testMockSP{objects: map[string]map[string]any{}}
	opts := TaskPromptOptions{
		PlanTitle: "Plan A",
		PlanID:    "PRI-1",
		TaskTitle: "Do the thing",
		Layer:     PromptLayerPersist,
	}
	out, err := BuildTaskPrompt(context.Background(), sp, pkgctx.NewSystemSecurityContext(), "", opts)
	if err != nil {
		t.Fatal(err)
	}
	if !IsTaskEnvelope(out) {
		t.Fatalf("persist layer must emit envelope marker:\n%s", out)
	}
	if strings.Contains(out, "Top 30 Highest Complexity") || strings.Contains(out, "#### Mandates") {
		t.Fatalf("persist layer leaked static dump:\n%s", out)
	}
	if len(out) > 4000 {
		t.Fatalf("persist envelope too large (%d bytes):\n%s", len(out), out)
	}
}

func TestFormatTaskEnvelope_shorterThanPolicyBodies(t *testing.T) {
	p := &PolicyEnforcement{ActivePolicies: []map[string]any{
		{
			objects.FieldKeyID:          "POL-ONBOARD-001",
			objects.FieldKeyTitle:       "Process data",
			objects.FieldKeyDescription: strings.Repeat("body ", 800),
		},
	}}
	refs := p.GeneratePromptSectionRefs()
	bodies := p.GeneratePromptSectionBodies()
	if len(refs) >= len(bodies) {
		t.Fatalf("refs (%d) should be shorter than bodies (%d)", len(refs), len(bodies))
	}
	env := FormatTaskEnvelope(TaskPromptOptions{TaskTitle: "T", PlanID: "PRI-1"}, []string{"POL-ONBOARD-001"}, nil)
	if len(env) >= len(bodies) {
		t.Fatalf("envelope (%d) should be shorter than inlined policy body (%d)", len(env), len(bodies))
	}
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
