package ticker

import (
	"testing"
)

func TestActivityTicker(t *testing.T) {
	ticker := NewActivityTicker()

	ticker.Tick("coder_agent", "Ontology Layer Registry")
	ticker.Tick("reviewer_agent", "Ontology Layer Registry")

	status := ticker.GetStatus()
	if len(status) != 2 {
		t.Errorf("expected 2 active agents, got %d", len(status))
	}
	foundCoder := false
	for _, a := range status {
		if a.Persona == "coder_agent" {
			foundCoder = true
			break
		}
	}
	if !foundCoder {
		t.Errorf("expected status entry for coder_agent")
	}
}
