package cli_builders

import (
	"context"
	"testing"
)

func TestPathOverride(t *testing.T) {
	spec := CLISpec{
		Name:         "test",
		Command:      "echo",
		PathOverride: "echo",
	}

	execCtx := &Execution{
		Spec: spec,
		Args: []string{"hello"},
	}

	err := BaseRunner(context.Background(), execCtx)
	if err != nil {
		t.Fatalf("BaseRunner failed: %v", err)
	}

	if execCtx.Stdout.String() != "hello\n" {
		t.Errorf("Expected 'hello\\n', got %q", execCtx.Stdout.String())
	}
}
