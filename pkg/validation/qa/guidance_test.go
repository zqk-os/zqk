package qa

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/validation"
)

func TestGuidanceEngine_Recommend(t *testing.T) {
	engine := NewGuidanceEngine()

	t.Run(validation.ConstMagicba16c010, func(t *testing.T) {
		g := engine.Recommend("coverage", "pkg/storage")
		if !strings.Contains(g.Summary, "Coverage") {
			t.Errorf(validation.ConstMagicfbf6b3aa, g.Summary)
		}
		if len(g.Steps) == 0 {
			t.Error(validation.ConstMagic4d2dee75)
		}
	})

	t.Run("AST violation", func(t *testing.T) {
		g := engine.Recommend(validation.ConstMagic9ae33b8e, validation.ConstMagic6faf563e)
		if !strings.Contains(g.Steps[0], validation.ConstMagic73400192) {
			t.Errorf(validation.ConstMagic204ed768, g.Steps[0])
		}
	})

	t.Run(validation.ConstMagic1f33b26f, func(t *testing.T) {
		g := engine.Recommend(validation.ConstMagic94fd8777, "BLI-123")
		if !strings.Contains(g.Summary, "Disparity") {
			t.Errorf(validation.ConstMagicfbf6b3aa, g.Summary)
		}
	})
}
