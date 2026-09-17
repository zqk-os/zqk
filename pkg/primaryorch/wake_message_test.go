package primaryorch

import (
	"strings"
	"testing"
)

func TestAppendAttentivenessContext_IncludesPRIAndTopBLIs(t *testing.T) {
	t.Parallel()
	got := AppendAttentivenessContext(
		"CAP wake: Task ATK-1 assigned to tpm",
		"PRI-123",
		[]BacklogBrief{
			{ID: "BLI-a", Title: "First", Tier: "P0"},
			{ID: "BLI-b", Title: "Second", Tier: "P1"},
			{ID: "BLI-c", Title: "Third", Tier: "P1"},
			{ID: "BLI-d", Title: "Skipped", Tier: "P2"},
		},
	)
	if !strings.Contains(got, "PRI=PRI-123") {
		t.Fatalf("missing PRI: %q", got)
	}
	if !strings.Contains(got, "BLI[1] P0 BLI-a") || !strings.Contains(got, "BLI[3] P1 BLI-c") {
		t.Fatalf("missing BLI lines: %q", got)
	}
	if strings.Contains(got, "BLI-d") {
		t.Fatalf("expected max 3 BLIs: %q", got)
	}
}

func TestComposeWakeMessage_UsesRequestFields(t *testing.T) {
	t.Parallel()
	got := ComposeWakeMessage(WakeRequest{
		TaskID:  "ATK-9",
		Persona: PersonaTPM,
		Message: "investigate",
		PlanID:  "PRI-9",
		TopBLIs: []BacklogBrief{{ID: "BLI-1", Title: "Heal check", Tier: "P1"}},
	})
	if !strings.Contains(got, "investigate") || !strings.Contains(got, "PRI=PRI-9") || !strings.Contains(got, "BLI-1") {
		t.Fatalf("got %q", got)
	}
}
