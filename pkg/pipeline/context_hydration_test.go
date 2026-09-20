package pipeline

import (
	"context"
	"testing"
)

func TestContextHydrationPlugin(t *testing.T) {
	plugin := &ContextHydrationPlugin{}
	if plugin.Name() != "ContextHydration" {
		t.Errorf("expected ContextHydration, got %s", plugin.Name())
	}

	payload, err := plugin.Execute(context.Background(), nil)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if payload["hydrated"] != true {
		t.Errorf("expected payload to be hydrated")
	}

	err = plugin.Validate(context.Background(), payload)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
