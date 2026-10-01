package qa

import (
	"strings"
	"testing"
)

func TestGuidanceEngine_Recommend(t *testing.T) {
	engine := NewGuidanceEngine()

	t.Run("Coverage failure", func(t *testing.T) {
		g := engine.Recommend("coverage", "pkg/storage")
		if !strings.Contains(g.Summary, "Coverage") {
			t.Errorf("unexpected summary: %s", g.Summary)
		}
		if len(g.Steps) == 0 {
			t.Error("expected remediation steps")
		}
	})

	t.Run("AST violation", func(t *testing.T) {
		g := engine.Recommend("concurrency_violation", "direct instantiation of NewManager")
		if !strings.Contains(g.Steps[0], "Specific Violation") {
			t.Errorf("expected specific violation in steps: %s", g.Steps[0])
		}
	})

	t.Run("Smoke and mirrors", func(t *testing.T) {
		g := engine.Recommend("smoke_and_mirrors", "BLI-123")
		if !strings.Contains(g.Summary, "Disparity") {
			t.Errorf("unexpected summary: %s", g.Summary)
		}
	})
}
