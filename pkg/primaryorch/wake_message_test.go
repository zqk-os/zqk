package primaryorch

import (
	"strings"
	"testing"
)

func TestAppendAttentivenessContext_IncludesPRIAndTopBLIs(t *testing.T) {
	t.Parallel()
	got := AppendAttentivenessContext(
		"CAP wake: Task ATK-1 assigned to tpm",
		"PLAN-123",
		[]BacklogBrief{
			{ID: "ITEM-a", Title: "First", Tier: "P0"},
			{ID: "ITEM-b", Title: "Second", Tier: "P1"},
			{ID: "ITEM-c", Title: "Third", Tier: "P1"},
			{ID: "ITEM-d", Title: "Skipped", Tier: "P2"},
		},
	)
	if !strings.Contains(got, "PRI=PLAN-123") {
		t.Fatalf("missing PRI: %q", got)
	}
	if !strings.Contains(got, "BLI[1] P0 ITEM-a") || !strings.Contains(got, "BLI[3] P1 ITEM-c") {
		t.Fatalf("missing BLI lines: %q", got)
	}
	if strings.Contains(got, "ITEM-d") {
		t.Fatalf("expected max 3 BLIs: %q", got)
	}
}

func TestComposeWakeMessage_UsesRequestFields(t *testing.T) {
	t.Parallel()
	got := ComposeWakeMessage(WakeRequest{
		TaskID:  "ATK-9",
		Persona: PersonaTPM,
		Message: "investigate",
		PlanID:  "PLAN-9",
		TopBLIs: []BacklogBrief{{ID: "ITEM-1", Title: "Heal check", Tier: "P1"}},
	})
	if !strings.Contains(got, "investigate") || !strings.Contains(got, "PRI=PLAN-9") || !strings.Contains(got, "ITEM-1") {
		t.Fatalf("got %q", got)
	}
}
