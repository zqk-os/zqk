package ambient

import (
	"encoding/json"
	"testing"
)

func TestCSnapBuilder_BuildEnvelope(t *testing.T) {
	t.Run("build successful envelope", func(t *testing.T) {
		builder := NewCSnapBuilder()

		filePath := "main.go"
		content := []byte("package main")

		payload, err := builder.BuildEnvelope(filePath, content)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		var env CSnapEnvelope
		err = json.Unmarshal(payload, &env)
		if err != nil {
			t.Fatalf("failed to unmarshal generated payload: %v", err)
		}

		if env.FilePath != filePath {
			t.Errorf("expected FilePath %s, got %s", filePath, env.FilePath)
		}

		if len(env.DependencyGraph) == 0 {
			t.Errorf("expected dependencies in the envelope, got none")
		}
	})
}
