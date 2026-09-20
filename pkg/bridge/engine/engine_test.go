package engine

import (
	"context"
	"testing"
)

func TestTranslationEngine(t *testing.T) {
	engine := NewTranslationEngine()

	t.Run("Rejects unregistered format", func(t *testing.T) {
		_, err := engine.Translate(context.Background(), "unknown", []byte("{}"))
		if err == nil {
			t.Errorf("expected error for unknown format")
		}
	})
}
