package ontology

import (
	"context"
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
)

func TestContextualizer_Contextualize(t *testing.T) {
	// Mock manager not needed for basic contextualization logic,
	// but good practice to include in constructor.
	c := NewContextualizer(nil)

	t.Run("ValidData", func(t *testing.T) {
		data := map[string]any{objects.FieldKeyKind: "service", objects.FieldKeyID: "svc-1"}
		res, err := c.Contextualize(context.Background(), data)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if res["mapped_kind"] != "service" || res["mapped_id"] != "svc-1" {
			t.Errorf("Mapping failed, got %v", res)
		}
	})

	t.Run("MissingFields", func(t *testing.T) {
		data := map[string]any{objects.FieldKeyKind: "service"} // missing id
		_, err := c.Contextualize(context.Background(), data)
		if err == nil {
			t.Error("Expected error, got nil")
		}
	})
}
