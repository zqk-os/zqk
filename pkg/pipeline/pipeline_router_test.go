package pipeline_test

import (
	"testing"

	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/pipeline"
)

func TestPipelineRouter(t *testing.T) {
	router := pipeline.NewPipelineRouter()

	plan := map[string]any{
		objects.FieldKeyID: "PLAN-123",
		"workstreams": []any{
			map[string]any{
				objects.FieldKeyID: "WS-1",
				"tasks": []any{
					map[string]any{
						objects.FieldKeyID:   "TSK-1",
						objects.FieldKeyTags: []any{"vocabulary_scheme:frontend"},
					},
					map[string]any{
						objects.FieldKeyID:   "TSK-2",
						objects.FieldKeyTags: []any{"vocabulary_scheme:backend"},
					},
				},
			},
			map[string]any{
				objects.FieldKeyID: "WS-2",
				"tasks": []any{
					map[string]any{
						objects.FieldKeyID:   "TSK-3",
						objects.FieldKeyTags: []any{"other_tag", "vocabulary_scheme:devops"},
					},
					map[string]any{
						objects.FieldKeyID: "TSK-4",
						// missing tags
					},
				},
			},
		},
	}

	res, err := router.Route(plan)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(res.WorkstreamIDs) != 2 {
		t.Errorf("expected 2 workstreams, got %d", len(res.WorkstreamIDs))
	}

	if res.TaskAssignments["TSK-1"] != "persona:frontend" {
		t.Errorf("expected persona:frontend for TSK-1, got %s", res.TaskAssignments["TSK-1"])
	}
	if res.TaskAssignments["TSK-2"] != "persona:backend" {
		t.Errorf("expected persona:backend for TSK-2, got %s", res.TaskAssignments["TSK-2"])
	}
	if res.TaskAssignments["TSK-3"] != "persona:devops" {
		t.Errorf("expected persona:devops for TSK-3, got %s", res.TaskAssignments["TSK-3"])
	}
	if res.TaskAssignments["TSK-4"] != "persona:default" {
		t.Errorf("expected persona:default for TSK-4, got %s", res.TaskAssignments["TSK-4"])
	}
}
