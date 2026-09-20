package swarminit

import (
	"strings"
	"testing"

	"github.com/zqk-os/zqk/pkg/objects"
)

func TestParseRecipe_missingExecutorFailClosed(t *testing.T) {
	t.Parallel()
	_, err := ParseRecipe(map[string]any{
		objects.FieldKeyID: "PIP-TEST",
		objects.FieldKeyStages: []any{
			map[string]any{objects.FieldKeyID: "s1"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "executor is required") {
		t.Fatalf("want executor required, got %v", err)
	}
}

func TestParseRecipe_unknownOnFailFailClosed(t *testing.T) {
	t.Parallel()
	_, err := ParseRecipe(map[string]any{
		objects.FieldKeyID: "PIP-TEST",
		objects.FieldKeyStages: []any{
			map[string]any{
				objects.FieldKeyID: "s1",
				stageKeyExecutor:   ExecutorBindSeats,
				stageKeyOnFail:     "explode",
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "on_fail") {
		t.Fatalf("want on_fail error, got %v", err)
	}
}

func TestParseRecipe_ok(t *testing.T) {
	t.Parallel()
	r, err := ParseRecipe(testPipeline("PIP-OK", []map[string]any{
		{objects.FieldKeyID: "bind_seats", stageKeyExecutor: ExecutorBindSeats, stageKeyOnFail: OnFailStop},
	}))
	if err != nil {
		t.Fatal(err)
	}
	if r.ID != "PIP-OK" || len(r.Stages) != 1 || r.Stages[0].Executor != ExecutorBindSeats {
		t.Fatalf("recipe=%+v", r)
	}
}

func TestPipelineRefFromWorkflow(t *testing.T) {
	t.Parallel()
	ref, err := PipelineRefFromWorkflow(map[string]any{
		objects.FieldKeyID: "WFL-X",
		objects.FieldKeyMetadata: map[string]any{
			SwarmInitPipelineRefKey: "PIP-SWARM-INIT-MMORCH-001",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ref != "PIP-SWARM-INIT-MMORCH-001" {
		t.Fatalf("ref=%q", ref)
	}
	if _, err := PipelineRefFromWorkflow(map[string]any{objects.FieldKeyID: "WFL-X"}); err == nil {
		t.Fatal("empty metadata must fail")
	}
}

func testPipeline(id string, stages []map[string]any) map[string]any {
	list := make([]any, len(stages))
	for i := range stages {
		list[i] = stages[i]
	}
	return map[string]any{
		objects.FieldKeyID:     id,
		objects.FieldKeyTitle:  "test recipe",
		objects.FieldKeyStages: list,
	}
}
